package nix

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NixManager = &nixManager{}

type nixManager struct {
	config dogeboxd.ServerConfig
	pups   dogeboxd.PupManager
}

func NewNixManager(config dogeboxd.ServerConfig, pups dogeboxd.PupManager) dogeboxd.NixManager {
	return nixManager{
		config: config,
		pups:   pups,
	}
}

func (nm nixManager) InitSystem(patch dogeboxd.NixPatch, dbxState dogeboxd.DogeboxState) {
	nm.UpdateIncludesFile(patch, nm.pups)

	patch.UpdateSystem(dogeboxd.NixSystemTemplateValues{
		SSH_ENABLED:     dbxState.SSH.Enabled,
		SSH_KEYS:        dbxState.SSH.Keys,
		SYSTEM_HOSTNAME: dbxState.Hostname,
		KEYMAP:          dbxState.KeyMap,
		TIMEZONE:        dbxState.Timezone,
	})

	nm.UpdateFirewallRules(patch, dbxState)
	nm.UpdateSystemContainerConfiguration(patch)
}

func (nm nixManager) UpdateIncludesFile(patch dogeboxd.NixPatch, pups dogeboxd.PupManager) {
	installed := pups.GetStateMap()
	var pupIDs []string
	for id, state := range installed {
		if state.Installation == dogeboxd.STATE_INSTALLING || state.Installation == dogeboxd.STATE_READY || state.Installation == dogeboxd.STATE_RUNNING {
			pupIDs = append(pupIDs, id)
		}
	}

	values := dogeboxd.NixIncludesFileTemplateValues{
		PUP_IDS: pupIDs,
		NIX_DIR: nm.config.NixDir,
	}

	patch.UpdateIncludesFile(values)
}

func (nm nixManager) WritePupFile(
	nixPatch dogeboxd.NixPatch,
	state dogeboxd.PupState,
	dbxState dogeboxd.DogeboxState,
) {
	services := []dogeboxd.NixPupContainerServiceValues{}

	for _, service := range state.Manifest.Container.Services {
		cwd := filepath.Join(fmt.Sprintf("${pkgs.pup.%s}", service.Name), service.Command.CWD)

		services = append(services, dogeboxd.NixPupContainerServiceValues{
			NAME: service.Name,
			EXEC: service.Command.Exec,
			CWD:  cwd,
			ENV:  toEnv(service.Command.ENV),
		})
	}

	pupSpecificEnv := nm.pups.GetPupSpecificEnvironmentVariablesForContainer(state.ID)
	globalEnv := dogeboxd.GetSystemEnvironmentVariablesForContainer()

	sourceDirectory := filepath.Join(nm.config.DataDir, "pups", state.ID)
	nixFile := filepath.Join(sourceDirectory, state.Manifest.Container.Build.NixFile)

	if state.IsDevModeEnabled {
		sourceDirectory = state.Source.Location
		nixFile = filepath.Join(sourceDirectory, state.Manifest.Container.Build.NixFile)
	}

	values := dogeboxd.NixPupContainerTemplateValues{
		DATA_DIR:          nm.config.DataDir,
		CONTAINER_LOG_DIR: nm.config.ContainerLogDir,
		PUP_ID:            state.ID,
		PUP_ENABLED:       state.Enabled,
		INTERNAL_IP:       state.IP,
		PUP_PORTS: []struct {
			PORT   int
			PUBLIC bool
		}{},
		STORAGE_PATH: filepath.Join(nm.config.DataDir, "pups/storage", state.ID),
		PUP_PATH:     sourceDirectory,
		NIX_FILE:     nixFile,
		SERVICES:     services,
		PUP_ENV:      toEnv(pupSpecificEnv),
		GLOBAL_ENV:   toEnv(globalEnv),

		IS_DEV_MODE:       state.IsDevModeEnabled,
		DEV_MODE_SERVICES: state.DevModeServices,
	}

	rebuildFW := false

	for _, ex := range state.Manifest.Container.Exposes {
		values.PUP_PORTS = append(values.PUP_PORTS, struct {
			PORT   int
			PUBLIC bool
		}{
			PORT:   ex.Port,
			PUBLIC: ex.ListenOnHost,
		})

		if ex.ListenOnHost || ex.WebUI {
			rebuildFW = true
		}
	}

	// If we have any public host ports, we need to
	// update the host firewall to open those ports.
	if rebuildFW {
		nm.UpdateFirewallRules(nixPatch, dbxState)
	}

	// If we need access to the internet, update the system container config.
	if state.Manifest.Container.RequiresInternet {
		nm.UpdateSystemContainerConfiguration(nixPatch)
	}

	nixPatch.WritePupFile(state.ID, values)
}

func (nm nixManager) RemovePupFile(nixPatch dogeboxd.NixPatch, pupId string) {
	nixPatch.RemovePupFile(pupId)
}

func (nm nixManager) UpdateSystem(nixPatch dogeboxd.NixPatch, values dogeboxd.NixSystemTemplateValues) {
	if values.KEYMAP == "" {
		values.KEYMAP = "us"
	}

	if values.TIMEZONE == "" {
		values.TIMEZONE = "UTC"
	}

	nixPatch.UpdateSystem(values)
}

func (nm nixManager) UpdateSystemContainerConfiguration(nixPatch dogeboxd.NixPatch) {
	// TODO: Move away from hardcoding these values. Should be pulled from pupmanager?
	hostIp := "10.69.0.1"
	containerCidr := "10.69.0.0/8"

	pupState := nm.pups.GetStateMap()
	var pupsRequiringInternet []dogeboxd.NixSystemContainerConfigTemplatePupRequiresInternet
	for _, state := range pupState {
		if state.Manifest.Container.RequiresInternet {
			pupsRequiringInternet = append(pupsRequiringInternet, dogeboxd.NixSystemContainerConfigTemplatePupRequiresInternet{
				PUP_ID: state.ID,
				PUP_IP: state.IP,
			})
		}
	}

	var pupsTcpConnections []dogeboxd.NixSystemContainerConfigTemplatePupTcpConnection

	pupsById := map[string]dogeboxd.PupState{}
	for _, state := range pupState {
		pupsById[state.ID] = state
	}

	for _, state := range pupState {
		// For each pup, we build up a list of _other_ pups that it needs to
		// talk TCP to. This could be zero, it could be many, or all of its
		// dependencies could actually point to the same remote pup.
		otherPupsById := map[string]dogeboxd.NixSystemContainerConfigTemplatePupTcpConnectionOtherPup{}

		for _, dependency := range state.Manifest.Dependencies {
			provider := state.Providers[dependency.InterfaceName]

			if provider == "" {
				// Do nothing here.
				continue
			}

			providerPup, ok := pupsById[provider]
			if !ok {
				// Probably log an error here?
				continue
			}

			// Find our interface in the provider's manifest
			var providerExposes *dogeboxd.PupManifestExposeConfig
			for _, providerExpose := range providerPup.Manifest.Container.Exposes {
				if providerExpose.Type != "tcp" {
					// Ignore anything not TCP, as those are supported elsewhere.
					continue
				}

				for _, providerExposeInterface := range providerExpose.Interfaces {
					if providerExposeInterface == dependency.InterfaceName {
						providerExposes = &providerExpose
						break
					}
				}
			}

			if providerExposes == nil {
				// No provider configured for this interface, ignore.
				continue
			}

			if _, ok := otherPupsById[providerPup.ID]; !ok {
				otherPupsById[providerPup.ID] = dogeboxd.NixSystemContainerConfigTemplatePupTcpConnectionOtherPup{
					NAME: providerPup.Manifest.Meta.Name,
					ID:   providerPup.ID,
					IP:   providerPup.IP,
					PORTS: []struct {
						PORT int
					}{},
				}
			}

			existing := otherPupsById[providerPup.ID]
			existing.PORTS = append(existing.PORTS, struct{ PORT int }{PORT: providerExposes.Port})
			otherPupsById[providerPup.ID] = existing
		}

		otherPups := []dogeboxd.NixSystemContainerConfigTemplatePupTcpConnectionOtherPup{}

		for _, otherPup := range otherPupsById {
			otherPups = append(otherPups, otherPup)
		}

		pupsTcpConnections = append(pupsTcpConnections, dogeboxd.NixSystemContainerConfigTemplatePupTcpConnection{
			NAME:       state.Manifest.Meta.Name,
			ID:         state.ID,
			IP:         state.IP,
			OTHER_PUPS: otherPups,
		})
	}

	values := dogeboxd.NixSystemContainerConfigTemplateValues{
		DOGEBOX_HOST_IP:         hostIp,
		DOGEBOX_CONTAINER_CIDR:  containerCidr,
		PUPS_REQUIRING_INTERNET: pupsRequiringInternet,
		PUPS_TCP_CONNECTIONS:    pupsTcpConnections,
	}

	nixPatch.UpdateSystemContainerConfiguration(values)
}

func (nm nixManager) UpdateFirewallRules(nixPatch dogeboxd.NixPatch, dbxState dogeboxd.DogeboxState) {
	installed := nm.pups.GetStateMap()
	var pupPorts []struct {
		PORT   int
		PUBLIC bool
		PUP_ID string
	}
	for pupID, state := range installed {
		// open all ports Exposed by the manifest
		for _, port := range state.Manifest.Container.Exposes {
			pupPorts = append(pupPorts, struct {
				PORT   int
				PUBLIC bool
				PUP_ID string
			}{
				PORT:   port.Port,
				PUBLIC: port.ListenOnHost,
				PUP_ID: pupID,
			})
		}
		// open all ports for webuis
		for _, webui := range state.WebUIs {
			pupPorts = append(pupPorts, struct {
				PORT   int
				PUBLIC bool
				PUP_ID string
			}{
				PORT:   webui.Port,
				PUBLIC: true,
				PUP_ID: pupID,
			})
		}
	}

	nixPatch.UpdateFirewall(dogeboxd.NixFirewallTemplateValues{
		SSH_ENABLED: dbxState.SSH.Enabled,
		PUP_PORTS:   pupPorts,
	})
}

func (nm nixManager) UpdateNetwork(nixPatch dogeboxd.NixPatch, values dogeboxd.NixNetworkTemplateValues) {
	// TODO: Move this out of here once network/nix.go is gone.
	nixPatch.UpdateNetwork(values)
}

func (nm nixManager) UpdateStorageOverlay(nixPatch dogeboxd.NixPatch, partitionName string) {
	currentUID := os.Getuid()
	uidStr := strconv.Itoa(currentUID)

	values := dogeboxd.NixStorageOverlayTemplateValues{
		STORAGE_DEVICE: partitionName,
		DATA_DIR:       nm.config.DataDir,
		DBX_UID:        uidStr,
	}

	nixPatch.UpdateStorageOverlay(values)
}

func (nm nixManager) RebuildBoot(log dogeboxd.SubLogger) error {
	cmdArgs := []string{"_dbxroot", "nix", "rb"}
	rebuildTarget, ok := nm.remoteRebuildTarget()
	if ok {
		log.Logf("Using remote rebuild target: %s", rebuildTarget)
		cmdArgs = append([]string{"env", fmt.Sprintf("REMOTE_REBUILD_DOGEBOX_DIRECTORY=%s", rebuildTarget)}, cmdArgs...)
	}

	md := exec.Command("sudo", cmdArgs...)
	log.LogCmd(md)
	err := md.Run()
	if err != nil {
		log.Errf("Error executing nix rebuild boot: %v\n", err)
		return err
	}
	return nil
}

func (nm nixManager) Rebuild(log dogeboxd.SubLogger) error {
	cmdArgs := []string{"_dbxroot", "nix", "rs"}
	rebuildTarget, ok := nm.remoteRebuildTarget()
	if ok {
		log.Logf("Using remote rebuild target: %s", rebuildTarget)
		cmdArgs = append([]string{"env", fmt.Sprintf("REMOTE_REBUILD_DOGEBOX_DIRECTORY=%s", rebuildTarget)}, cmdArgs...)
	}

	cmd := exec.Command("sudo", cmdArgs...)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Error executing nix rebuild: %v\n", err)
		return err
	}

	return nil
}

func (nm nixManager) remoteRebuildTarget() (string, bool) {
	if nm.config.NixDir == "" {
		return "", false
	}

	return nm.config.NixDir, true
}

func (nm nixManager) NewPatch(log dogeboxd.SubLogger) dogeboxd.NixPatch {
	return NewNixPatch(nm, log)
}

func toEnv(entries map[string]string) []dogeboxd.EnvEntry {
	envSlice := make([]dogeboxd.EnvEntry, 0, len(entries))
	for key, value := range entries {
		strValue := fmt.Sprintf("%v", value)
		envSlice = append(envSlice, dogeboxd.EnvEntry{KEY: key, VAL: strValue})
	}
	return envSlice
}

func GetRunningFlakePath() (string, error) {
	// Get system architecture
	cmd := exec.Command("uname", "-m")
	archOutput, err := cmd.Output()
	if err != nil {
		//log.Errf("error getting arch via uname: %v", err)
		return "", err
	}

	// TODO : check out strconv
	architecture := strings.TrimSpace(string(archOutput))

	// Get build type
	buildTypeBytes, err := os.ReadFile("/opt/build-type")
	if err != nil {
		//log.Printf("Failed to read build type: %v", err)
		os.Exit(1)
	}
	buildType := strings.TrimSpace(string(buildTypeBytes))

	flakeName := fmt.Sprintf("dogeboxos-%s-%s", buildType, architecture)
	flakePath := fmt.Sprintf("/etc/nixos#nixosConfigurations.%s", flakeName)

	return flakePath, nil
}

func GetConfigValue(configItem string) (string, error) {
	flakePath, err := GetRunningFlakePath()
	if err != nil {
		//log.Errf("error getting flake path: %v", err)
		return "", err
	}
	cmdArgs := []string{"nix", "eval", "--raw", flakePath + ".config." + configItem, "--impure"}
	cmd := exec.Command("sudo", cmdArgs...)
	stdout, err := cmd.Output()
	if err != nil {
		//log.Errf("error getting config value: %v", err)
		return "", err
	}
	return string(stdout), nil
}
