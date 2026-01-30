package system

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	source "github.com/dogeorg/dogeboxd/pkg/sources"
)

const backupManifestName = "manifest.json"

func (t SystemUpdater) backupConfig(a dogeboxd.BackupConfig, j dogeboxd.Job) error {
	log := j.Logger.Step("backup-config")
	log.Progress(5).Log("Collecting backup files")

	paths, err := collectBackupFiles(t.config)
	if err != nil {
		log.Errf("Failed to collect backup files: %v", err)
		return err
	}

	log.Progress(15).Logf("Found %d files to include", len(paths))

	outputPath, err := resolveBackupOutputPath(t.config, a, j.ID)
	if err != nil {
		log.Errf("Failed to resolve backup output path: %v", err)
		return err
	}

	log.Progress(30).Log("Writing backup archive")
	if _, err := writeBackupArchive(paths, t.config, outputPath); err != nil {
		log.Errf("Failed to write backup archive: %v", err)
		return err
	}

	log.Progress(100).Logf("Backup created at %s", outputPath)
	return nil
}

func (t SystemUpdater) restoreConfig(a dogeboxd.RestoreConfig, j dogeboxd.Job) error {
	log := j.Logger.Step("restore-config")
	log.Progress(5).Log("Reading backup manifest")

	manifest, err := readBackupManifest(a.SourcePath)
	if err != nil {
		log.Errf("Failed to read backup manifest: %v", err)
		return err
	}

	if err := validateManifestTargets(manifest, t.config); err != nil {
		log.Errf("Backup manifest validation failed: %v", err)
		return err
	}

	log.Progress(25).Log("Restoring files")
	if err := t.sm.CloseDB(); err != nil {
		log.Errf("Failed to close database: %v", err)
		return err
	}

	restoreErr := restoreBackupArchive(a.SourcePath, manifest, t.config)
	if openErr := t.sm.OpenDB(); openErr != nil {
		log.Errf("Failed to reopen database: %v", openErr)
		return openErr
	}
	if restoreErr != nil {
		log.Errf("Failed to restore backup archive: %v", restoreErr)
		return restoreErr
	}

	dbxState := t.sm.Get().Dogebox
	dbxState.InitialState.HasGeneratedKey = false
	if err := t.sm.SetDogebox(dbxState); err != nil {
		log.Errf("Failed to update key state: %v", err)
		return err
	}

	if isPathWithin(a.SourcePath, t.config.TmpDir) {
		_ = os.Remove(a.SourcePath)
	}

	log.Progress(60).Log("Rehydrating pups from backup")
	sourceManager := source.NewSourceManager(t.config, t.sm, t.pupManager)
	if pm, ok := t.pupManager.(*pup.PupManager); ok {
		pm.SetSourceManager(sourceManager)
		if err := pm.ReloadFromDisk(); err != nil {
			log.Errf("Failed to reload pups: %v", err)
			return err
		}
	}
	if err := t.rehydratePups(log, sourceManager, a.SessionToken); err != nil {
		log.Errf("Failed to rehydrate pups: %v", err)
		return err
	}

	log.Progress(100).Log("Restore complete")
	return nil
}

func resolveBackupOutputPath(config dogeboxd.ServerConfig, a dogeboxd.BackupConfig, jobID string) (string, error) {
	filename := fmt.Sprintf("dogebox-backup-%s.tar.gz", jobID)
	switch a.Target {
	case dogeboxd.BackupTargetDownload:
		outDir := filepath.Join(config.TmpDir, "backups")
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return "", err
		}
		return filepath.Join(outDir, filename), nil
	case dogeboxd.BackupTargetRemovable:
		if a.DestinationPath == "" {
			return "", fmt.Errorf("destination path is required for removable backups")
		}
		dest := filepath.Clean(a.DestinationPath)
		info, err := os.Stat(dest)
		if err == nil && info.IsDir() {
			return filepath.Join(dest, filename), nil
		}
		if err == nil {
			return dest, nil
		}
		if os.IsNotExist(err) {
			return dest, nil
		}
		return "", err
	default:
		return "", fmt.Errorf("unsupported backup target %q", a.Target)
	}
}

func collectBackupFiles(config dogeboxd.ServerConfig) ([]string, error) {
	files := map[string]struct{}{}
	addFile := func(path string) {
		files[path] = struct{}{}
	}

	dataDir := config.DataDir
	nixDir := config.NixDir

	dbPath := filepath.Join(dataDir, "dogebox.db")
	if isRegularFile(dbPath) {
		addFile(dbPath)
	}

	pupStateGlob := filepath.Join(dataDir, "pups", "pup_*.gob")
	if matches, err := filepath.Glob(pupStateGlob); err == nil {
		for _, match := range matches {
			if isRegularFile(match) {
				addFile(match)
			}
		}
	} else {
		return nil, err
	}

	pupsDir := filepath.Join(dataDir, "pups")
	if entries, err := os.ReadDir(pupsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == "storage" {
				continue
			}
			manifestPath := filepath.Join(pupsDir, entry.Name(), "manifest.json")
			if isRegularFile(manifestPath) {
				addFile(manifestPath)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := filepath.WalkDir(nixDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isRegularFile(path) {
			addFile(path)
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func writeBackupArchive(paths []string, config dogeboxd.ServerConfig, destPath string) (dogeboxd.BackupManifest, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return dogeboxd.BackupManifest{}, err
	}
	file, err := os.Create(destPath)
	if err != nil {
		return dogeboxd.BackupManifest{}, err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	entries := make([]dogeboxd.BackupFileEntry, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		tarPath, err := backupTarPath(path)
		if err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		hash, err := writeFileToTar(tarWriter, tarPath, path, info)
		if err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		entries = append(entries, dogeboxd.BackupFileEntry{
			Path:   filepath.Clean(path),
			Size:   info.Size(),
			Sha256: hash,
		})
	}

	manifest := dogeboxd.BackupManifest{
		Version:   1,
		CreatedAt: time.Now().UTC(),
		DataDir:   config.DataDir,
		NixDir:    config.NixDir,
		Files:     entries,
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return dogeboxd.BackupManifest{}, err
	}
	if err := writeTarEntry(tarWriter, backupManifestName, manifestBytes, 0644, time.Now()); err != nil {
		return dogeboxd.BackupManifest{}, err
	}

	return manifest, nil
}

func readBackupManifest(archivePath string) (dogeboxd.BackupManifest, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return dogeboxd.BackupManifest{}, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return dogeboxd.BackupManifest{}, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		if header.Name != backupManifestName {
			continue
		}
		manifestBytes, err := io.ReadAll(tarReader)
		if err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		var manifest dogeboxd.BackupManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return dogeboxd.BackupManifest{}, err
		}
		if manifest.Version != 1 {
			return dogeboxd.BackupManifest{}, fmt.Errorf("unsupported backup manifest version %d", manifest.Version)
		}
		return manifest, nil
	}

	return dogeboxd.BackupManifest{}, fmt.Errorf("manifest.json not found in backup archive")
}

func restoreBackupArchive(archivePath string, manifest dogeboxd.BackupManifest, config dogeboxd.ServerConfig) error {
	expected := map[string]dogeboxd.BackupFileEntry{}
	for _, entry := range manifest.Files {
		tarPath, err := backupTarPath(entry.Path)
		if err != nil {
			return err
		}
		expected[tarPath] = entry
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	restored := map[string]struct{}{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == backupManifestName {
			continue
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		entry, ok := expected[header.Name]
		if !ok {
			return fmt.Errorf("unexpected archive entry %q", header.Name)
		}
		if header.Size != entry.Size {
			return fmt.Errorf("size mismatch for %s", entry.Path)
		}

		targetPath := filepath.Clean(string(filepath.Separator) + header.Name)
		if !isAllowedRestorePath(targetPath, config) {
			return fmt.Errorf("restore path not allowed: %s", targetPath)
		}

		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		tempFile, err := os.CreateTemp(dir, ".restore-*")
		if err != nil {
			return err
		}
		hash := sha256.New()
		writer := io.MultiWriter(tempFile, hash)
		if _, err := io.Copy(writer, tarReader); err != nil {
			tempFile.Close()
			os.Remove(tempFile.Name())
			return err
		}
		if err := tempFile.Close(); err != nil {
			os.Remove(tempFile.Name())
			return err
		}
		actualHash := hex.EncodeToString(hash.Sum(nil))
		if actualHash != entry.Sha256 && filepath.Base(entry.Path) == "dogebox.db" {
			// allow legacy backups where dogebox.db changed during archive creation
		} else if actualHash != entry.Sha256 {
			os.Remove(tempFile.Name())
			return fmt.Errorf("checksum mismatch for %s", entry.Path)
		}

		if err := os.Rename(tempFile.Name(), targetPath); err != nil {
			os.Remove(tempFile.Name())
			return err
		}
		if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
			return err
		}
		restored[header.Name] = struct{}{}
	}

	for tarPath := range expected {
		if _, ok := restored[tarPath]; !ok {
			return fmt.Errorf("missing file in archive: %s", tarPath)
		}
	}

	return nil
}

func validateManifestTargets(manifest dogeboxd.BackupManifest, config dogeboxd.ServerConfig) error {
	if manifest.DataDir == "" || manifest.NixDir == "" {
		return fmt.Errorf("invalid manifest: missing dataDir or nixDir")
	}
	for _, entry := range manifest.Files {
		if !isAllowedRestorePath(entry.Path, config) {
			return fmt.Errorf("manifest path not allowed: %s", entry.Path)
		}
	}
	return nil
}

func (t SystemUpdater) rehydratePups(log dogeboxd.SubLogger, sourceManager dogeboxd.SourceManager, sessionToken string) error {
	stateMap := t.pupManager.GetStateMap()
	if len(stateMap) == 0 {
		log.Log("No pups to rehydrate")
		return nil
	}

	if sessionToken == "" {
		return fmt.Errorf("missing key session token for pup rehydrate")
	}

	nixPatch := t.nix.NewPatch(log)
	dbxState := t.sm.Get().Dogebox
	successCount := 0
	failures := []string{}

	for _, state := range stateMap {
		if state.Installation == dogeboxd.STATE_UNINSTALLED {
			continue
		}
		if state.IsDevModeEnabled {
			log.Logf("Skipping dev mode pup %s", state.ID)
			continue
		}
		if state.Source.ID == "" && state.Source.Location == "" {
			log.Errf("Missing source for pup %s", state.ID)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		pupName := state.Manifest.Meta.Name
		pupVersion := state.Version
		if pupVersion == "" {
			pupVersion = state.Manifest.Meta.Version
		}
		if pupName == "" || pupVersion == "" {
			log.Errf("Missing name/version for pup %s", state.ID)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		pupPath := filepath.Join(t.config.DataDir, "pups", state.ID)
		manifestPath := filepath.Join(pupPath, "manifest.json")
		manifest := state.Manifest
		sourceID := state.Source.ID
		ensureSource := func() error {
			if sourceID != "" {
				if _, err := sourceManager.GetSource(sourceID); err == nil {
					return nil
				}
			}
			if state.Source.Location == "" {
				return fmt.Errorf("missing source location")
			}
			log.Logf("Restoring missing source for %s from %s", pupName, state.Source.Location)
			newSource, err := sourceManager.AddSource(state.Source.Location)
			if err != nil {
				return err
			}
			sourceID = newSource.Config().ID
			state.Source = newSource.Config()
			_, _ = t.pupManager.UpdatePup(state.ID, dogeboxd.SetPupSource(state.Source))
			return nil
		}
		downloadPup := func() (dogeboxd.PupManifest, error) {
			if err := ensureSource(); err != nil {
				return dogeboxd.PupManifest{}, err
			}
			downloadedManifest, err := sourceManager.DownloadPup(pupPath, sourceID, pupName, pupVersion)
			if err == nil {
				return downloadedManifest, nil
			}
			downloadErr := err
			if state.Source.Type == "git" {
				if src, srcErr := sourceManager.GetSource(sourceID); srcErr == nil {
					log.Logf("Fallback download for %s@%s", pupName, pupVersion)
					fallbackErr := src.Download(pupPath, map[string]string{
						"tag":     pupVersion,
						"subPath": ".",
					})
					if fallbackErr == nil {
						loaded, loadErr := dogeboxd.LoadManifestFromPath(pupPath)
						if loadErr == nil {
							return loaded, nil
						}
						downloadErr = loadErr
					} else {
						downloadErr = fallbackErr
					}
				}
			}
			return dogeboxd.PupManifest{}, downloadErr
		}

		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			log.Logf("Downloading %s@%s", pupName, pupVersion)
			downloadedManifest, err := downloadPup()
			if err != nil {
				log.Errf("Failed to download pup %s: %v", pupName, err)
				failures = append(failures, state.Manifest.Meta.Name)
				continue
			}
			manifest = downloadedManifest
			_, _ = t.pupManager.UpdatePup(state.ID, dogeboxd.SetPupManifest(downloadedManifest))
		} else if err != nil {
			log.Errf("Failed to stat pup %s: %v", pupName, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		} else {
			loaded, err := dogeboxd.LoadManifestFromPath(pupPath)
			if err == nil {
				manifest = loaded
			}
		}

		if err := t.verifyNixFileHash(pupPath, manifest, state.IsDevModeEnabled, log); err != nil {
			log.Logf("Re-downloading %s@%s to recover missing or invalid nix files", pupName, pupVersion)
			downloadedManifest, downloadErr := downloadPup()
			if downloadErr != nil {
				log.Errf("Failed to download pup %s: %v", pupName, downloadErr)
				failures = append(failures, state.Manifest.Meta.Name)
				continue
			}
			manifest = downloadedManifest
			_, _ = t.pupManager.UpdatePup(state.ID, dogeboxd.SetPupManifest(downloadedManifest))
			if err := t.verifyNixFileHash(pupPath, manifest, state.IsDevModeEnabled, log); err != nil {
				log.Errf("Failed to verify nix hash for %s: %v", pupName, err)
				failures = append(failures, state.Manifest.Meta.Name)
				continue
			}
		}

		storagePath := filepath.Join(t.config.DataDir, "pups", "storage", state.ID)
		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			cmd := exec.Command("sudo", "_dbxroot", "pup", "create-storage", "--data-dir", t.config.DataDir, "--pupId", state.ID)
			log.LogCmd(cmd)
			if err := cmd.Run(); err != nil {
				log.Errf("Failed to create storage for %s: %v", pupName, err)
				failures = append(failures, state.Manifest.Meta.Name)
				continue
			}
		}

		keyData, err := t.dkm.MakeDelegate(state.ID, sessionToken)
		if err != nil {
			log.Errf("Failed to create delegate keys for %s: %v", pupName, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		cmd := exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", state.ID, "--key-file", "delegated.key", "--data", keyData.Priv)
		log.LogCmd(cmd)
		if err := cmd.Run(); err != nil {
			log.Errf("Failed to write delegate key for %s: %v", pupName, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		cmd = exec.Command("sudo", "_dbxroot", "pup", "write-key", "--data-dir", t.config.DataDir, "--pupId", state.ID, "--key-file", "delegated.extended.key", "--data", keyData.Wif)
		log.LogCmd(cmd)
		if err := cmd.Run(); err != nil {
			log.Errf("Failed to write delegate key for %s: %v", pupName, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		if err := dogeboxd.WritePupConfigToStorage(t.config.DataDir, state.ID, state.Config, log); err != nil {
			log.Errf("Failed to write config for %s: %v", pupName, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		_, _ = t.pupManager.UpdatePup(state.ID, dogeboxd.SetPupInstallation(dogeboxd.STATE_READY))
		current, _, err := t.pupManager.GetPup(state.ID)
		if err != nil {
			log.Errf("Failed to load pup %s: %v", state.ID, err)
			failures = append(failures, state.Manifest.Meta.Name)
			continue
		}

		t.nix.WritePupFile(nixPatch, current, dbxState)
		successCount++
	}

	if len(failures) > 0 {
		return fmt.Errorf("failed to rehydrate pups: %s", strings.Join(failures, ", "))
	}

	t.nix.UpdateIncludesFile(nixPatch, t.pupManager)

	if err := nixPatch.Apply(); err != nil {
		return err
	}

	if successCount == 0 {
		return fmt.Errorf("no pups were rehydrated")
	}
	return nil
}

func isAllowedRestorePath(path string, config dogeboxd.ServerConfig) bool {
	clean := filepath.Clean(path)
	return isPathWithin(clean, config.DataDir) || isPathWithin(clean, config.NixDir)
}

func isPathWithin(path string, base string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func backupTarPath(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("backup path must be absolute: %s", path)
	}
	return strings.TrimPrefix(filepath.ToSlash(clean), "/"), nil
}

func writeTarEntry(tw *tar.Writer, name string, contents []byte, mode int64, modTime time.Time) error {
	header := &tar.Header{
		Name:    name,
		Mode:    mode,
		Size:    int64(len(contents)),
		ModTime: modTime,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func writeFileToTar(tw *tar.Writer, tarPath string, path string, info os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	header := &tar.Header{
		Name:    tarPath,
		Mode:    int64(info.Mode().Perm()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tw, hash), file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
