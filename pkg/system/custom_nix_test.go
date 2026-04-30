package system

import (
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestMigrateLegacyCustomNixMovesFileIntoDataDir(t *testing.T) {
	dataDir := t.TempDir()
	nixDir := filepath.Join(dataDir, "nix")
	if err := os.MkdirAll(nixDir, 0755); err != nil {
		t.Fatalf("failed to create nix dir: %v", err)
	}

	legacyPath := filepath.Join(nixDir, "custom.nix")
	expectedContent := "{ pkgs, ... }: { }"
	if err := os.WriteFile(legacyPath, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("failed to write legacy custom.nix: %v", err)
	}

	config := dogeboxd.ServerConfig{
		DataDir: dataDir,
		NixDir:  nixDir,
	}

	if err := MigrateLegacyCustomNix(config); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	customNixPath := GetCustomNixPath(config)
	data, err := os.ReadFile(customNixPath)
	if err != nil {
		t.Fatalf("expected migrated custom.nix to exist: %v", err)
	}
	if string(data) != expectedContent {
		t.Fatalf("expected custom.nix content %q, got %q", expectedContent, string(data))
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy custom.nix to be removed, stat err: %v", err)
	}
}
