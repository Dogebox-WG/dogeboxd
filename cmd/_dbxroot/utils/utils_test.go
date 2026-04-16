package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareReleaseTagOverrideRollsBackNewFile(t *testing.T) {
	previousVersioningDirPath := versioningDirPath
	versioningDirPath = t.TempDir()
	defer func() {
		versioningDirPath = previousVersioningDirPath
	}()

	releaseTagPath := filepath.Join(versioningDirPath, dbxReleaseTagFilename)

	rollback, err := PrepareReleaseTagOverride("v0.9.0-rc.3")
	if err != nil {
		t.Fatalf("PrepareReleaseTagOverride returned error: %v", err)
	}

	contents, err := os.ReadFile(releaseTagPath)
	if err != nil {
		t.Fatalf("failed to read persisted release tag: %v", err)
	}
	if string(contents) != "v0.9.0-rc.3\n" {
		t.Fatalf("unexpected release tag contents: %q", string(contents))
	}

	if err := rollback(); err != nil {
		t.Fatalf("rollback returned error: %v", err)
	}

	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected release tag file to be removed, got err=%v", err)
	}
}

func TestPrepareReleaseTagOverrideRestoresPreviousFile(t *testing.T) {
	previousVersioningDirPath := versioningDirPath
	versioningDirPath = t.TempDir()
	defer func() {
		versioningDirPath = previousVersioningDirPath
	}()

	releaseTagPath := filepath.Join(versioningDirPath, dbxReleaseTagFilename)
	if err := os.WriteFile(releaseTagPath, []byte("v0.9.0-rc.1\n"), 0o644); err != nil {
		t.Fatalf("failed to seed existing release tag file: %v", err)
	}

	rollback, err := PrepareReleaseTagOverride("v0.9.0-rc.3")
	if err != nil {
		t.Fatalf("PrepareReleaseTagOverride returned error: %v", err)
	}

	updatedContents, err := os.ReadFile(releaseTagPath)
	if err != nil {
		t.Fatalf("failed to read updated release tag: %v", err)
	}
	if string(updatedContents) != "v0.9.0-rc.3\n" {
		t.Fatalf("unexpected updated release tag contents: %q", string(updatedContents))
	}

	if err := rollback(); err != nil {
		t.Fatalf("rollback returned error: %v", err)
	}

	restoredContents, err := os.ReadFile(releaseTagPath)
	if err != nil {
		t.Fatalf("failed to read restored release tag: %v", err)
	}
	if string(restoredContents) != "v0.9.0-rc.1\n" {
		t.Fatalf("unexpected restored release tag contents: %q", string(restoredContents))
	}
}
