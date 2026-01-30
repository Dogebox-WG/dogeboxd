package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BackupManifest struct {
	Version   int               `json:"version"`
	CreatedAt string            `json:"createdAt"`
	DataDir   string            `json:"dataDir"`
	NixDir    string            `json:"nixDir"`
	Files     []BackupFileEntry `json:"files"`
}

type BackupFileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./scripts/fix-backup-manifest.go /path/to/backup.tar.gz")
		os.Exit(1)
	}

	archivePath := os.Args[1]
	if err := fixArchive(archivePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func fixArchive(archivePath string) error {
	tempDir, err := os.MkdirTemp("", "dogebox-backup-fix-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	manifest, err := extractArchive(archivePath, tempDir)
	if err != nil {
		return err
	}

	if err := recomputeManifestHashes(&manifest, tempDir); err != nil {
		return err
	}

	newArchive := archivePath + ".fixed"
	if err := writeArchive(newArchive, tempDir, manifest); err != nil {
		return err
	}

	backupPath := archivePath + ".bak"
	if err := os.Rename(archivePath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(newArchive, archivePath); err != nil {
		_ = os.Rename(backupPath, archivePath)
		return err
	}

	fmt.Printf("Updated backup written to %s (original at %s)\n", archivePath, backupPath)
	return nil
}

func extractArchive(archivePath string, dest string) (BackupManifest, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return BackupManifest{}, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var manifest BackupManifest
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return BackupManifest{}, err
		}

		if header.Name == "manifest.json" {
			contents, err := io.ReadAll(tarReader)
			if err != nil {
				return BackupManifest{}, err
			}
			if err := json.Unmarshal(contents, &manifest); err != nil {
				return BackupManifest{}, err
			}
			continue
		}

		if header.Typeflag == tar.TypeDir {
			continue
		}

		targetPath := filepath.Join(dest, filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return BackupManifest{}, err
		}
		outFile, err := os.Create(targetPath)
		if err != nil {
			return BackupManifest{}, err
		}
		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return BackupManifest{}, err
		}
		if err := outFile.Close(); err != nil {
			return BackupManifest{}, err
		}
	}

	if manifest.Version == 0 {
		return BackupManifest{}, fmt.Errorf("manifest.json not found in archive")
	}
	return manifest, nil
}

func recomputeManifestHashes(manifest *BackupManifest, baseDir string) error {
	for i, entry := range manifest.Files {
		rel, err := toTarPath(entry.Path)
		if err != nil {
			return err
		}
		fullPath := filepath.Join(baseDir, filepath.FromSlash(rel))
		info, err := os.Stat(fullPath)
		if err != nil {
			return err
		}
		hash, err := fileSha256(fullPath)
		if err != nil {
			return err
		}
		manifest.Files[i].Size = info.Size()
		manifest.Files[i].Sha256 = hash
	}
	return nil
}

func writeArchive(archivePath string, baseDir string, manifest BackupManifest) error {
	outFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	if err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		if err := writeTarFile(tarWriter, rel, path, info); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeTarEntry(tarWriter, "manifest.json", manifestBytes, 0644, time.Now())
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

func writeTarFile(tw *tar.Writer, tarPath string, path string, info os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := &tar.Header{
		Name:    tarPath,
		Mode:    int64(info.Mode().Perm()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}

func fileSha256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func toTarPath(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("backup path must be absolute: %s", path)
	}
	return strings.TrimPrefix(filepath.ToSlash(clean), "/"), nil
}
