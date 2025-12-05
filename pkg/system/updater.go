package system

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

/*
SystemUpdater implements dogeboxd.SystemUpdater

SystemUpdater is responsible for handling longer running jobs for
dogeboxd.Dogeboxd, especially as they relate to the operating system.

*/

func NewSystemUpdater(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager, nixManager dogeboxd.NixManager, sourceManager dogeboxd.SourceManager, pupManager dogeboxd.PupManager, stateManager dogeboxd.StateManager, dkm dogeboxd.DKMManager, snapshotManager SnapshotManager) SystemUpdater {
	return SystemUpdater{
		config:     config,
		jobs:       make(chan dogeboxd.Job),
		done:       make(chan dogeboxd.Job),
		network:    networkManager,
		nix:        nixManager,
		sources:    sourceManager,
		pupManager: pupManager,
		sm:         stateManager,
		dkm:        dkm,
		snapshots:  snapshotManager,
	}
}

// SnapshotManager interface for pup version snapshots
type SnapshotManager interface {
	CreateSnapshot(pupState dogeboxd.PupState) error
	GetSnapshot(pupID string) (*dogeboxd.PupVersionSnapshot, error)
	HasSnapshot(pupID string) bool
	DeleteSnapshot(pupID string) error
}

type SystemUpdater struct {
	config     dogeboxd.ServerConfig
	jobs       chan dogeboxd.Job
	done       chan dogeboxd.Job
	network    dogeboxd.NetworkManager
	nix        dogeboxd.NixManager
	sources    dogeboxd.SourceManager
	pupManager dogeboxd.PupManager
	sm         dogeboxd.StateManager
	dkm        dogeboxd.DKMManager
	snapshots  SnapshotManager
}

func (t SystemUpdater) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {
				case <-stop:
					break mainloop
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					switch a := j.A.(type) {
					case dogeboxd.InstallPup:
						err := t.installPup(a, j)
						if err != nil {
							j.Err = "Failed to install pup"
						}
						t.done <- j
					case dogeboxd.UninstallPup:
						err := t.uninstallPup(j)
						if err != nil {
							j.Err = "Failed to uninstall pup"
						}
						t.done <- j
					case dogeboxd.PurgePup:
						err := t.purgePup(j)
						if err != nil {
							j.Err = "Failed to purge pup"
						}
						t.done <- j
					case dogeboxd.EnablePup:
						err := t.enablePup(j)
						if err != nil {
							j.Err = "Failed to enable pup"
						}
						t.done <- j
					case dogeboxd.DisablePup:
						err := t.disablePup(j)
						if err != nil {
							j.Err = "Failed to disable pup"
						}
						t.done <- j
					case dogeboxd.UpgradePup:
						err := t.upgradePup(a, j)
						if err != nil {
							j.Err = "Failed to upgrade pup"
						}
						t.done <- j
					case dogeboxd.RollbackPupUpgrade:
						err := t.rollbackPupUpgrade(j)
						if err != nil {
							j.Err = "Failed to rollback pup"
						}
						t.done <- j
					case dogeboxd.ImportBlockchainData:
						err := t.importBlockchainData(j)
						if err != nil {
							j.Err = "Failed to import blockchain data"
						}
						t.done <- j
					case dogeboxd.UpdatePendingSystemNetwork:
						err := t.network.SetPendingNetwork(a.Network, j)
						if err != nil {
							j.Err = "Failed to set system network"
						}
						t.done <- j

					case dogeboxd.EnableSSH:
						err := t.EnableSSH(j.Logger.Step("enable SSH"))
						if err != nil {
							j.Err = "Failed to enable SSH"
						}
						t.done <- j
					case dogeboxd.DisableSSH:
						err := t.DisableSSH(j.Logger.Step("disable SSH"))
						if err != nil {
							j.Err = "Failed to disable SSH"
						}
						t.done <- j

					case dogeboxd.AddSSHKey:
						err := t.AddSSHKey(a.Key, j.Logger.Step("add SSH key"))
						if err != nil {
							j.Err = "Failed to add SSH key"
						}
						t.done <- j

					case dogeboxd.RemoveSSHKey:
						err := t.RemoveSSHKey(a.ID, j.Logger.Step("remove SSH key"))
						if err != nil {
							j.Err = "Failed to remove SSH key"
						}
						t.done <- j

					case dogeboxd.SaveCustomNix:
						err := t.SaveCustomNix(a.Content, j.Logger.Step("save custom nix"))
						if err != nil {
							j.Err = "Failed to save custom configuration"
						}
						t.done <- j

					case dogeboxd.AddBinaryCache:
						err := t.AddBinaryCache(a, j.Logger.Step("Add binary cache"))
						if err != nil {
							j.Err = "Failed to add binary cache"
						}
						t.done <- j

					case dogeboxd.RemoveBinaryCache:
						err := t.removeBinaryCache(a)
						if err != nil {
							j.Err = "Failed to remove binary cache"
						}
						t.done <- j

					case dogeboxd.SystemUpdate:
						logger := j.Logger.Step("system update")
						logger.Progress(5).Logf("Starting system update to %s", a.Version)
						if err := DoSystemUpdate(a.Package, a.Version); err != nil {
							logger.Errf("System update failed: %v", err)
							j.Err = err.Error()
						} else {
							logger.Progress(100).Logf("System update to %s completed", a.Version)
						}
						t.done <- j

					default:
						fmt.Printf("Unknown action type: %v\n", a)
					}
				}
			}
		}()
		started <- true
		<-stop
		// do shutdown things
		stopped <- true
	}()
	return nil
}

func (t SystemUpdater) AddJob(j dogeboxd.Job) {
	t.jobs <- j
}

func (t SystemUpdater) GetUpdateChannel() chan dogeboxd.Job {
	return t.done
}

// NOTE: updatePup and rollbackPup are not yet implemented.
// Current scope focuses on update detection and notification only.
// These will be added in a future phase when we implement actual update execution.
//
// func (t SystemUpdater) updatePup(a dogeboxd.UpdatePup, j dogeboxd.Job) error {
// 	s := *j.State
// 	log := j.Logger.Step("update")
// 	log.Logf("Updating pup %s (%s) to version %s", s.Manifest.Meta.Name, s.ID, a.TargetVersion)
// 	// TODO: This needs to use the PupUpdater service
// 	return nil
// }
//
// func (t SystemUpdater) rollbackPup(a dogeboxd.RollbackPupUpdate, j dogeboxd.Job) error {
// 	s := *j.State
// 	log := j.Logger.Step("rollback")
// 	log.Logf("Rolling back pup %s (%s)", s.Manifest.Meta.Name, s.ID)
// 	// TODO: This needs to use the PupUpdater service
// 	return nil
// }

func (t SystemUpdater) markPupBroken(s dogeboxd.PupState, reason string, upstreamError error) error {
	_, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupBrokenReason(reason), dogeboxd.SetPupInstallation(dogeboxd.STATE_BROKEN))
	if err != nil {
		log.Printf("Failed to even mark pup as broken after issue: %v", err)
		return err
	}

	log.Printf("Marked pup %s as broken because: %s", s.ID, reason)

	return upstreamError
}

/* InstallPup takes a PupManifest and ensures a nix config
 * is written and any packages installed so that the Pup can
 * be started.
 */
func (t SystemUpdater) installPup(pupSelection dogeboxd.InstallPup, j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("install")
	nixPatch := t.nix.NewPatch(log)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_INSTALLING)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	log.Logf("Installing pup from %s: %s @ %s", pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)

	log.Logf("Downloading pup to %s", pupPath)
	err := t.sources.DownloadPup(pupPath, pupSelection.SourceId, pupSelection.PupName, pupSelection.PupVersion)
	if err != nil {
		log.Errf("Failed to download pup: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// Ensure the nix file configured in the manifest matches the hash specified.
	// Read pupPath s.Manifest.Container.Build.NixFile and hash it with sha256
	nixFile, err := os.ReadFile(filepath.Join(pupPath, s.Manifest.Container.Build.NixFile))
	if err != nil {
		log.Errf("Failed to read specified nix file: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_FILE_MISSING, err)
	}
	nixFileSha256 := sha256.Sum256(nixFile)

	// Compare the sha256 hash of the nix file to the hash specified in the manifest
	if fmt.Sprintf("%x", nixFileSha256) != s.Manifest.Container.Build.NixFileSha256 {
		log.Errf("Nix file hash mismatch, should be %s but is %s", fmt.Sprintf("%x", nixFileSha256), s.Manifest.Container.Build.NixFileSha256)

		// Log, but only actually return an error if we're not in dev mode.
		if !s.IsDevModeEnabled {
			return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_HASH_MISMATCH, err)
		}
	}

	// create the storage dir
	cmd := exec.Command("sudo", "_dbxroot", "pup", "create-storage", "--data-dir", t.config.DataDir, "--pupId", s.ID)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO : Do we need command output here?
		log.Errf("Failed to create pup storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// write delegate key to storage dir
	keyData, err := t.dkm.MakeDelegate(s.ID, pupSelection.SessionToken)
	if err != nil {
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_CREATION_FAILED, err)
	}

	cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", s.ID, "--key-file", "delegated.key", "--data", keyData.Priv)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO : Do we need command output here?
		log.Errf("Failed to create delegate key in storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", s.ID, "--key-file", "delegated.extended.key", "--data", keyData.Wif)
	log.LogCmd(cmd)
	err = cmd.Run()
	if err != nil {
		// TODO : Do we need command output here?
		log.Errf("Failed to create extended delegate key in storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED, err)
	}

	// Write initial config to secure storage (includes defaults from manifest)
	// This ensures config.env exists before the container starts
	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, s.Config, log); err != nil {
		log.Errf("Failed to write initial config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// Now that we're mostly installed, enable it.
	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_ENABLE_FAILED, err)
	}

	dbxState := t.sm.Get().Dogebox

	t.nix.WritePupFile(nixPatch, newState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	// Do a nix rebuild before we mark the pup as installed, this way
	// the frontend will get a much longer "Installing.." state, as opposed
	// to a much longer "Starting.." state, which might confuse the user.
	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	return nil
}

func (t SystemUpdater) uninstallPup(j dogeboxd.Job) error {
	// TODO: uninstall deps if they're not needed by another pup.
	s := *j.State
	log := j.Logger.Step("uninstall")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Uninstalling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLING)); err != nil {
		log.Errf("Failed to update pup uninstalling state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	t.nix.RemovePupFile(nixPatch, s.ID)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UNINSTALLED)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	return nil
}

func (t SystemUpdater) purgePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("purge")
	// Check if we're in a purgable state before we do anything.
	if s.Installation != dogeboxd.STATE_UNINSTALLED {
		log.Errf("Cannot purge pup %s in state %s", s.ID, s.Installation)
		return fmt.Errorf("cannot purge pup %s in state %s", s.ID, s.Installation)
	}

	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_PURGING)); err != nil {
		log.Errf("Failed to update pup purging state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	pupDir := filepath.Join(t.config.DataDir, "pups")

	log.Logf("Purging pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	// Delete pup state from disk
	if err := os.Remove(filepath.Join(pupDir, fmt.Sprintf("pup_%s.gob", s.ID))); err != nil {
		log.Errf("Failed to remove pup state %v", err)
		// Keep going if we fail.
	}

	// Delete downloaded pup source
	if err := os.RemoveAll(filepath.Join(pupDir, s.ID)); err != nil {
		log.Errf("Failed to remove pup source %v", err)
		// Keep going if we fail.
	}

	// Delete pup storage directory
	cmd := exec.Command("sudo", "_dbxroot", "pup", "delete-storage", "--pupId", s.ID, "--data-dir", t.config.DataDir)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Failed to remove pup storage: %v", err)
		// Keep going if we fail.
	}

	if err := t.pupManager.PurgePup(s.ID); err != nil {
		log.Errf("Failed to purge pup %s: %v", s.ID, err)
		// Keep going if we fail.
	}

	return nil
}

func (t SystemUpdater) enablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("enable")
	log.Logf("Enabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true))
	if err != nil {
		log.Errf("Failed to update pup enabled state: %v", err)
		return err
	}
	log.Log("set pup state to enabled")

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) disablePup(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("disable")
	log.Logf("Disabling pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	newState, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(false))
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	log.LogCmd(cmd)

	if err := cmd.Run(); err != nil {
		log.Errf("Error executing _dbxroot pup stop: %v", err)
		return err
	}

	dbxState := t.sm.Get().Dogebox

	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) importBlockchainData(j dogeboxd.Job) error {
	log := j.Logger.Step("import-blockchain-data")
	log.Log("Starting blockchain data import")

	// Find the Dogecoin Core pup
	var dogecoinPup *dogeboxd.PupState
	pupStateMap := t.pupManager.GetStateMap()
	for _, pup := range pupStateMap {
		if pup.Manifest.Meta.Name == "Dogecoin Core" {
			dogecoinPup = &pup
			break
		}
	}

	var wasEnabled bool
	if dogecoinPup != nil {
		log.Logf("Found Dogecoin Core pup: %s (ID: %s)", dogecoinPup.Manifest.Meta.Name, dogecoinPup.ID)
		wasEnabled = dogecoinPup.Enabled

		// If the pup is enabled, disable it to prevent auto-restart during import
		if wasEnabled {
			log.Log("Dogecoin Core pup is enabled, temporarily disabling during import...")
			_, err := t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(false))
			if err != nil {
				log.Errf("Failed to disable pup: %v", err)
				return err
			}

			// Stop the pup if it's running
			stopCmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", dogecoinPup.ID)
			log.LogCmd(stopCmd)
			if err := stopCmd.Run(); err != nil {
				log.Errf("Error stopping pup: %v", err)
				// Re-enable the pup if we failed to stop it
				t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(true))
				return err
			}
		}
	}

	// Run the blockchain data import command
	cmd := exec.Command("sudo", "_dbxroot", "import-blockchain-data", "--data-dir", t.config.DataDir)
	log.LogCmd(cmd)

	err := cmd.Run()
	if err != nil {
		log.Errf("Failed to import blockchain data: %v", err)
	}

	// Re-enable the pup if it was originally enabled
	if dogecoinPup != nil && wasEnabled {
		log.Log("Re-enabling Dogecoin Core pup...")
		_, enableErr := t.pupManager.UpdatePup(dogecoinPup.ID, dogeboxd.PupEnabled(true))
		if enableErr != nil {
			log.Errf("Failed to re-enable pup: %v", enableErr)
			if err == nil {
				err = enableErr
			}
		} else {
			// Apply nix patch to ensure the pup configuration is updated
			dbxState := t.sm.Get().Dogebox
			nixPatch := t.nix.NewPatch(log)
			pupState, _, pupErr := t.pupManager.GetPup(dogecoinPup.ID)
			if pupErr == nil {
				t.nix.WritePupFile(nixPatch, pupState, dbxState)
				if applyErr := nixPatch.Apply(); applyErr != nil {
					log.Errf("Failed to apply nix patch: %v", applyErr)
				}
			}
		}
	}

	if err != nil {
		return err
	}

	log.Log("Blockchain data import completed")
	return nil
}

func (t SystemUpdater) AddBinaryCache(j dogeboxd.AddBinaryCache, log dogeboxd.SubLogger) error {
	dbxState := t.sm.Get().Dogebox

	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return fmt.Errorf("failed to generate random ID for binary cache: %v", err)
	}

	dbxState.BinaryCaches = append(dbxState.BinaryCaches, dogeboxd.DogeboxStateBinaryCache{
		ID:   string(id),
		Host: j.Host,
		Key:  j.Key,
	})

	if err := t.sm.SetDogebox(dbxState); err != nil {
		return err
	}

	nixPatch := t.nix.NewPatch(log)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(nixPatch, values)

	return nixPatch.Apply()
}

func (t SystemUpdater) removeBinaryCache(j dogeboxd.RemoveBinaryCache) error {
	dbxState := t.sm.Get().Dogebox

	keyFound := false
	for i, cache := range dbxState.BinaryCaches {
		if cache.ID == j.ID {
			dbxState.BinaryCaches = append(dbxState.BinaryCaches[:i], dbxState.BinaryCaches[i+1:]...)
			keyFound = true
		}
	}

	if !keyFound {
		return fmt.Errorf("binary cache with ID %s not found", j.ID)
	}

	return t.sm.SetDogebox(dbxState)
}

// upgradePup handles upgrading a pup to a new version while preserving config and data
func (t SystemUpdater) upgradePup(upgrade dogeboxd.UpgradePup, j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("upgrade")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Upgrading pup %s (%s) from %s to %s", s.Manifest.Meta.Name, s.ID, s.Version, upgrade.TargetVersion)

	// 1. Record if pup was enabled
	wasEnabled := s.Enabled

	// 2. Stop the pup if it's running
	if s.Enabled {
		log.Log("Stopping pup before upgrade...")
		cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
		log.LogCmd(cmd)
		if err := cmd.Run(); err != nil {
			log.Errf("Warning: failed to stop pup: %v", err)
			// Continue anyway, might not be running
		}
	}

	// 3. Create a snapshot of current state for rollback
	log.Log("Creating snapshot for rollback...")
	if t.snapshots != nil {
		if err := t.snapshots.CreateSnapshot(s); err != nil {
			log.Errf("Warning: failed to create snapshot: %v", err)
			// Continue anyway - upgrade is more important than rollback capability
		}
	}

	// 4. Update state to UPGRADING
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UPGRADING)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 5. Download new version (overwrites existing pup directory)
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)
	log.Logf("Downloading new version to %s", pupPath)

	err := t.sources.DownloadPup(pupPath, upgrade.SourceId, s.Manifest.Meta.Name, upgrade.TargetVersion)
	if err != nil {
		log.Errf("Failed to download new version: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// 6. Load the new manifest
	newManifest, err := dogeboxd.LoadManifestFromPath(pupPath)
	if err != nil {
		log.Errf("Failed to load new manifest: %v", err)
		return t.markPupBroken(s, "manifest_load_failed", err)
	}

	// 7. Verify nix file hash
	nixFile, err := os.ReadFile(filepath.Join(pupPath, newManifest.Container.Build.NixFile))
	if err != nil {
		log.Errf("Failed to read nix file: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_FILE_MISSING, err)
	}
	nixFileSha256 := sha256.Sum256(nixFile)
	if fmt.Sprintf("%x", nixFileSha256) != newManifest.Container.Build.NixFileSha256 {
		log.Errf("Nix file hash mismatch")
		if !s.IsDevModeEnabled {
			return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_HASH_MISMATCH, fmt.Errorf("nix hash mismatch"))
		}
	}

	// 8. Update pup state with new version and manifest, preserving config/providers/hooks
	_, err = t.pupManager.UpdatePup(s.ID,
		dogeboxd.SetPupVersion(upgrade.TargetVersion),
		dogeboxd.SetPupManifest(newManifest),
		// Config, Providers, Hooks are preserved automatically
	)
	if err != nil {
		log.Errf("Failed to update pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 9. Write updated config to storage (in case manifest has new config fields)
	updatedState, _, err := t.pupManager.GetPup(s.ID)
	if err != nil {
		log.Errf("Failed to get updated pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, updatedState.Config, log); err != nil {
		log.Errf("Failed to write config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// 10. Rebuild nix configuration
	dbxState := t.sm.Get().Dogebox
	t.nix.WritePupFile(nixPatch, updatedState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	// 11. Mark as ready
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Errf("Failed to update pup installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 12. Re-enable if it was enabled before
	if wasEnabled {
		log.Log("Re-enabling pup after upgrade...")
		if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true)); err != nil {
			log.Errf("Warning: failed to re-enable pup: %v", err)
			// Not a fatal error
		}
	}

	log.Logf("Successfully upgraded pup %s to version %s", s.Manifest.Meta.Name, upgrade.TargetVersion)
	return nil
}

// rollbackPupUpgrade handles rolling back a pup to its previous version using a snapshot
func (t SystemUpdater) rollbackPupUpgrade(j dogeboxd.Job) error {
	s := *j.State
	log := j.Logger.Step("rollback")
	nixPatch := t.nix.NewPatch(log)

	log.Logf("Rolling back pup %s (%s)", s.Manifest.Meta.Name, s.ID)

	// 1. Check if snapshot exists
	if t.snapshots == nil {
		return fmt.Errorf("snapshot manager not available")
	}

	snapshot, err := t.snapshots.GetSnapshot(s.ID)
	if err != nil {
		log.Errf("Failed to get snapshot: %v", err)
		return fmt.Errorf("failed to get snapshot: %w", err)
	}
	if snapshot == nil {
		log.Errf("No snapshot found for rollback")
		return fmt.Errorf("no snapshot available for rollback")
	}

	log.Logf("Found snapshot: rolling back to version %s", snapshot.Version)

	// 2. Stop the pup if running
	cmd := exec.Command("sudo", "_dbxroot", "pup", "stop", "--pupId", s.ID)
	log.LogCmd(cmd)
	_ = cmd.Run() // Ignore error, might not be running

	// 3. Update state to indicate rollback in progress
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_UPGRADING)); err != nil {
		log.Errf("Failed to update state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 4. Download the previous version
	pupPath := filepath.Join(t.config.DataDir, "pups", s.ID)
	log.Logf("Downloading previous version %s", snapshot.Version)

	err = t.sources.DownloadPup(pupPath, snapshot.SourceID, s.Manifest.Meta.Name, snapshot.Version)
	if err != nil {
		log.Errf("Failed to download previous version: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_DOWNLOAD_FAILED, err)
	}

	// 5. Restore state from snapshot
	_, err = t.pupManager.UpdatePup(s.ID,
		dogeboxd.SetPupVersion(snapshot.Version),
		dogeboxd.SetPupManifest(snapshot.Manifest),
		dogeboxd.SetPupConfig(snapshot.Config),
		dogeboxd.SetPupProviders(snapshot.Providers),
	)
	if err != nil {
		log.Errf("Failed to restore pup state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 6. Write config to storage
	if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, s.ID, snapshot.Config, log); err != nil {
		log.Errf("Failed to write config to storage: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STORAGE_CREATION_FAILED, err)
	}

	// 7. Rebuild nix configuration
	restoredState, _, _ := t.pupManager.GetPup(s.ID)
	dbxState := t.sm.Get().Dogebox
	t.nix.WritePupFile(nixPatch, restoredState, dbxState)
	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		log.Errf("Failed to apply nix patch: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_NIX_APPLY_FAILED, err)
	}

	// 8. Mark as ready
	if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY)); err != nil {
		log.Errf("Failed to update installation state: %v", err)
		return t.markPupBroken(s, dogeboxd.BROKEN_REASON_STATE_UPDATE_FAILED, err)
	}

	// 9. Re-enable if it was enabled before
	if snapshot.Enabled {
		log.Log("Re-enabling pup after rollback...")
		if _, err := t.pupManager.UpdatePup(s.ID, dogeboxd.PupEnabled(true)); err != nil {
			log.Errf("Warning: failed to re-enable pup: %v", err)
		}
	}

	// 10. Delete the snapshot after successful rollback
	if err := t.snapshots.DeleteSnapshot(s.ID); err != nil {
		log.Errf("Warning: failed to delete snapshot: %v", err)
		// Not a fatal error
	}

	log.Logf("Successfully rolled back pup %s to version %s", s.Manifest.Meta.Name, snapshot.Version)
	return nil
}
