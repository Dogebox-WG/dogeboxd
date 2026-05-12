package system

import (
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestOSFlakeNeedsMigration(t *testing.T) {
	testCases := []struct {
		version        string
		needsMigration bool
	}{
		{version: "v0.6.0", needsMigration: true},
		{version: "v0.8.1", needsMigration: true},
		{version: "v0.9.0", needsMigration: false},
		{version: "v0.9.7", needsMigration: false},
		{version: "v0.10.0", needsMigration: false},
		{version: "v1.3.2", needsMigration: false},
		{version: "v0.9.0-rc.4", needsMigration: false},
	}

	for _, tc := range testCases {
		if got := osFlakeNeedsMigration(tc.version); got != tc.needsMigration {
			t.Fatalf("expected osFlakeNeedsMigration(%q) to be %v, got %v", tc.version, tc.needsMigration, got)
		}
	}
}

func TestInferTargetReleaseForDogeboxdRevision(t *testing.T) {
	releases := []UpgradableRelease{
		{Version: "v1.3.0"},
		{Version: "v1.2.0"},
	}

	targetVersion, err := inferTargetReleaseForDogeboxdRevision(
		releases,
		"target-rev",
		func(_ string, releaseVersion string) (string, error) {
			if releaseVersion == "v1.2.0" {
				return "target-rev", nil
			}
			return "other-rev", nil
		},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if targetVersion != "v1.2.0" {
		t.Fatalf("expected v1.2.0, got %s", targetVersion)
	}
}

func writeDogeboxdRevisionForTest(t *testing.T, rev string) {
	t.Helper()

	versionDir := os.Getenv("VERSION_PATH_OVERRIDE")
	if versionDir == "" {
		t.Fatal("VERSION_PATH_OVERRIDE is not set")
	}

	pkgDir := filepath.Join(versionDir, "dogeboxd")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create dogeboxd version dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "rev"), []byte(rev), 0644); err != nil {
		t.Fatalf("failed to write dogeboxd rev: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "hash"), []byte("hash"), 0644); err != nil {
		t.Fatalf("failed to write dogeboxd hash: %v", err)
	}
}

func TestQueueOSFlakeMigratorIfNeededQueuesOneUpdateAndWritesMarker(t *testing.T) {
	setupMockVersioning(t, "v0.8.1")
	writeDogeboxdRevisionForTest(t, "target-rev")

	config := dogeboxd.ServerConfig{
		DataDir: t.TempDir(),
		TmpDir:  t.TempDir(),
	}

	var queuedActions []dogeboxd.Action

	jobID, queued, err := queueOSFlakeMigratorIfNeeded(
		config,
		func(action dogeboxd.Action) string {
			queuedActions = append(queuedActions, action)
			return "job-123"
		},
		func(string) ([]byte, error) {
			return []byte(`{
  defaultDbxRelease = "v0.8.1";
}`), nil
		},
		CreateSuccessMock([]RepositoryTag{
			{Tag: "v1.2.0"},
			{Tag: "v1.1.0"},
		}),
		func(_ string, releaseVersion string) (string, error) {
			if releaseVersion == "v1.2.0" {
				return "target-rev", nil
			}
			return "other-rev", nil
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued {
		t.Fatal("expected migrator to queue an update")
	}
	if jobID != "job-123" {
		t.Fatalf("expected job-123, got %s", jobID)
	}
	if len(queuedActions) != 1 {
		t.Fatalf("expected exactly one queued action, got %d", len(queuedActions))
	}

	update, ok := queuedActions[0].(dogeboxd.SystemUpdate)
	if !ok {
		t.Fatalf("expected SystemUpdate action, got %T", queuedActions[0])
	}
	if update.Package != "os" || update.Version != "v1.2.0" {
		t.Fatalf("unexpected queued update: %+v", update)
	}

	markerContents, err := os.ReadFile(getOSFlakeMigratorMarkerPath(config))
	if err != nil {
		t.Fatalf("expected marker file, got %v", err)
	}
	if len(markerContents) != 0 {
		t.Fatalf("expected empty marker file, got %q", string(markerContents))
	}
}

func TestQueueOSFlakeMigratorIfNeededSkipsWhenMarkerExists(t *testing.T) {
	setupMockVersioning(t, "v0.8.1")

	config := dogeboxd.ServerConfig{
		DataDir: t.TempDir(),
		TmpDir:  t.TempDir(),
	}

	if err := writeOSFlakeMigratorMarker(config); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	var queuedActions []dogeboxd.Action
	jobID, queued, err := queueOSFlakeMigratorIfNeeded(
		config,
		func(action dogeboxd.Action) string {
			queuedActions = append(queuedActions, action)
			return "job-456"
		},
		func(string) ([]byte, error) {
			return []byte(`{
  defaultDbxRelease = "v0.8.1";
}`), nil
		},
		CreateSuccessMock([]RepositoryTag{
			{Tag: "v1.2.0"},
		}),
		func(_ string, releaseVersion string) (string, error) {
			if releaseVersion == "v1.2.0" {
				return "target-rev", nil
			}
			return "other-rev", nil
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if queued || jobID != "" {
		t.Fatalf("expected marker to skip queueing, queued=%v jobID=%q", queued, jobID)
	}
	if len(queuedActions) != 0 {
		t.Fatalf("expected no queued actions, got %d", len(queuedActions))
	}
}
