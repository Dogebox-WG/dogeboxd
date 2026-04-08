package system

import (
	"fmt"
	"os"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
)

func (t SystemUpdater) initialBootstrap(a dogeboxd.InitialBootstrap, j dogeboxd.Job) error {
	log := j.Logger.Step("initial-bootstrap")
	log.Progress(5).Log("Starting initial bootstrap")

	dbxState := t.sm.Get().Dogebox
	if dbxState.InitialState.HasFullyConfigured {
		return fmt.Errorf("system has already been initialised")
	}
	if !dbxState.InitialState.HasGeneratedKey || !dbxState.InitialState.HasSetNetwork {
		return fmt.Errorf("system not ready to initialise")
	}

	nixPatch := t.nix.NewPatch(j.Logger.Step("bootstrap-network").Progress(15))

	if err := t.network.TryConnect(nixPatch); err != nil {
		return fmt.Errorf("error connecting to network: %w", err)
	}

	systemLog := j.Logger.Step("bootstrap-system").Progress(30)
	systemLog.Log("Preparing system configuration")
	t.nix.InitSystem(nixPatch, dbxState)

	if err := nixPatch.Apply(); err != nil {
		return fmt.Errorf("error initialising system: %w", err)
	}

	if dbxState.StorageDevice != "" {
		storageLog := j.Logger.Step("bootstrap-storage").Progress(55)
		storageLog.Logf("Initialising storage device: %s", dbxState.StorageDevice)

		dbClosed := false
		defer func() {
			if !dbClosed {
				return
			}
			if err := t.sm.OpenDB(); err != nil {
				storageLog.Errf("Error re-opening store manager: %v", err)
			}
		}()

		if err := t.sm.CloseDB(); err != nil {
			return fmt.Errorf("error closing DB: %w", err)
		}
		dbClosed = true

		tempDir, err := os.MkdirTemp("", "dbx-data-overlay")
		if err != nil {
			return fmt.Errorf("error creating temporary directory: %w", err)
		}
		storageLog.Logf("Created temporary directory: %s", tempDir)

		partitionName, err := InitStorageDevice(dbxState)
		if err != nil {
			return fmt.Errorf("error initialising storage device: %w", err)
		}

		if err := utils.CopyFiles(t.config.DataDir, tempDir); err != nil {
			return fmt.Errorf("error copying data to temp dir: %w", err)
		}

		overlayPatch := t.nix.NewPatch(storageLog.Progress(65))
		t.nix.UpdateStorageOverlay(overlayPatch, partitionName)
		if err := overlayPatch.Apply(); err != nil {
			return fmt.Errorf("error applying overlay patch: %w", err)
		}

		if err := utils.CopyFiles(tempDir, t.config.DataDir); err != nil {
			return fmt.Errorf("error copying data back to data dir: %w", err)
		}

		reoverlayPatch := t.nix.NewPatch(storageLog.Progress(75))
		t.nix.UpdateStorageOverlay(reoverlayPatch, partitionName)
		if err := reoverlayPatch.ApplyCustom(dogeboxd.NixPatchApplyOptions{
			DangerousNoRebuild: true,
		}); err != nil {
			return fmt.Errorf("error re-applying overlay patch: %w", err)
		}

		if err := t.sm.OpenDB(); err != nil {
			return fmt.Errorf("error re-opening store manager: %w", err)
		}
		dbClosed = false
	}

	if a.ReflectorToken != "" && a.ReflectorHost != "" {
		reflectorLog := j.Logger.Step("bootstrap-reflector").Progress(82)
		if err := SaveReflectorTokenForReboot(t.config, a.ReflectorHost, a.ReflectorToken); err != nil {
			reflectorLog.Errf("Error saving reflector data: %v", err)
		} else {
			reflectorLog.Log("Saved reflector data for post-reboot submission")
		}
	}

	sourcesLog := j.Logger.Step("bootstrap-sources").Progress(86)
	if _, err := t.sources.AddSource("https://github.com/Dogebox-WG/pups.git"); err != nil {
		return fmt.Errorf("error adding dogeorg source: %w", err)
	}
	sourcesLog.Log("Added default pups source")

	if a.InitialSSHKey != "" {
		sshLog := j.Logger.Step("bootstrap-ssh").Progress(90)
		if err := t.AddSSHKey(a.InitialSSHKey, sshLog); err != nil {
			return fmt.Errorf("error adding initial SSH key: %w", err)
		}
		if err := t.EnableSSH(sshLog.Progress(93)); err != nil {
			return fmt.Errorf("error enabling SSH: %w", err)
		}
	}

	cacheLog := j.Logger.Step("bootstrap-caches").Progress(95)
	if a.UseFoundationOSBinaryCache {
		if err := t.AddBinaryCache(dogeboxd.AddBinaryCache{
			Host: "https://dbx.nix.dogecoin.org",
			Key:  "dbx.nix.dogecoin.org:ODXaHC+9DNqXQ8ZTijaCT4JpieqmOatZeZBbdN51Obc=",
		}, cacheLog); err != nil {
			return fmt.Errorf("error adding foundation OS binary cache: %w", err)
		}
	}

	if a.UseFoundationPupBinaryCache {
		if err := t.AddBinaryCache(dogeboxd.AddBinaryCache{
			Host: "https://pups.nix.dogecoin.org",
			Key:  "pups.nix.dogecoin.org:hQx/w1TQlN423VyK+D/AnD10Ul8ovVxLcPrMRBt9T3Q=",
		}, cacheLog.Progress(97)); err != nil {
			return fmt.Errorf("error adding foundation pups binary cache: %w", err)
		}
	}

	finalLog := j.Logger.Step("bootstrap-finish").Progress(100)
	dbxs := t.sm.Get().Dogebox
	dbxs.InitialState.HasFullyConfigured = true
	if err := t.sm.SetDogebox(dbxs); err != nil {
		return fmt.Errorf("error persisting flags: %w", err)
	}

	j.Success = map[string]any{
		"status":  "OK",
		"message": "Initial bootstrap completed",
	}
	finalLog.Log("Dogebox successfully bootstrapped")

	if t.lifecycle != nil {
		go func() {
			time.Sleep(5 * time.Second)
			t.lifecycle.Reboot()
		}()
	}

	return nil
}
