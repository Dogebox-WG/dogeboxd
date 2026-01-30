package system

import (
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func TestCollectBackupFilesManifestOnly(t *testing.T) {
	dataDir := t.TempDir()
	nixDir := filepath.Join(t.TempDir(), "nix")
	if err := os.MkdirAll(nixDir, 0755); err != nil {
		t.Fatalf("failed to create nix dir: %v", err)
	}

	mustWriteFile(t, filepath.Join(dataDir, "dogebox.db"), "db")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "pup_one.gob"), "gob")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "pup_two.gob"), "gob")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "alpha", "manifest.json"), "{}")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "alpha", "ignored.txt"), "nope")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "storage", "alpha", "delegated.key"), "key")
	mustWriteFile(t, filepath.Join(dataDir, "pups", "storage", "alpha", ".dbx", "config.env"), "env")
	mustWriteFile(t, filepath.Join(nixDir, "dogebox.nix"), "nix")

	config := dogeboxd.ServerConfig{
		DataDir: dataDir,
		NixDir:  nixDir,
	}

	paths, err := collectBackupFiles(config)
	if err != nil {
		t.Fatalf("collectBackupFiles failed: %v", err)
	}

	assertContains(t, paths, filepath.Join(dataDir, "dogebox.db"))
	assertContains(t, paths, filepath.Join(dataDir, "pups", "pup_one.gob"))
	assertContains(t, paths, filepath.Join(dataDir, "pups", "pup_two.gob"))
	assertContains(t, paths, filepath.Join(dataDir, "pups", "alpha", "manifest.json"))
	assertContains(t, paths, filepath.Join(nixDir, "dogebox.nix"))

	assertNotContains(t, paths, filepath.Join(dataDir, "pups", "alpha", "ignored.txt"))
	assertNotContains(t, paths, filepath.Join(dataDir, "pups", "storage", "alpha", "delegated.key"))
	assertNotContains(t, paths, filepath.Join(dataDir, "pups", "storage", "alpha", ".dbx", "config.env"))
}

func mustWriteFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}

func assertContains(t *testing.T, values []string, target string) {
	t.Helper()
	for _, value := range values {
		if value == target {
			return
		}
	}
	t.Fatalf("expected list to include %s", target)
}

func assertNotContains(t *testing.T, values []string, target string) {
	t.Helper()
	for _, value := range values {
		if value == target {
			t.Fatalf("expected list to exclude %s", target)
		}
	}
}
