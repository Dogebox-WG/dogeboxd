package definitions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
)

type mockRepoTagsFetcher struct {
	tags []system.RepositoryTag
	err  error
}

func (m mockRepoTagsFetcher) GetRepoTags(string) ([]system.RepositoryTag, error) {
	return m.tags, m.err
}

func testOSFlakeReadFile(version string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) {
		return []byte(fmt.Sprintf(`{
  dbxRelease = %q;
}`, version)), nil
	}
}

func setupTestDBXRelease(t *testing.T, currentRelease string) {
	t.Helper()

	versionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(versionDir, "dbx"), []byte(currentRelease+"\n"), 0644); err != nil {
		t.Fatalf("failed to write test dbx release: %v", err)
	}
	t.Setenv("VERSION_PATH_OVERRIDE", versionDir)
}

func TestOSFlakeMigrationRequirementsAllowEligibleInstalledVersions(t *testing.T) {
	testCases := []struct {
		name                  string
		installedFlakeVersion string
	}{
		{
			name:                  "pre 0.9 installed flake",
			installedFlakeVersion: "v0.8.1",
		},
		{
			name:                  "0.9 prerelease installed flake",
			installedFlakeVersion: "v0.9.0-rc.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := core.Context{
				Config: dogeboxd.ServerConfig{
					DataDir: t.TempDir(),
					TmpDir:  t.TempDir(),
				},
				ReadFile: testOSFlakeReadFile(tc.installedFlakeVersion),
				RepoTagsFetcher: mockRepoTagsFetcher{
					tags: []system.RepositoryTag{
						{Tag: "v1.2.0-rc.1"},
						{Tag: "v1.2.0"},
						{Tag: "v1.1.0"},
					},
				},
			}

			applies, reason, err := OSFlakeMigration.Requirements(ctx, core.MigrationRecord{})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !applies {
				t.Fatalf("expected migration to apply, got reason %q", reason)
			}
		})
	}
}

func TestOSFlakeMigrationRequirementsSkipWhenInstalledVersionIsAfterConstraint(t *testing.T) {
	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFile("v0.9.0"),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.2.0"},
			},
		},
	}

	applies, reason, err := OSFlakeMigration.Requirements(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if applies {
		t.Fatal("expected installed version after constraint to skip migration")
	}
	if reason == "" {
		t.Fatal("expected skip reason")
	}
}

func TestOSFlakeMigrationRequirementsSkipWhenNoStableReleaseAvailable(t *testing.T) {
	setupTestDBXRelease(t, "v0.8.1")

	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFile("v0.9.0-rc.1"),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.2.0-rc.1"},
			},
		},
	}

	applies, reason, err := OSFlakeMigration.Requirements(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if applies {
		t.Fatal("expected no stable release to skip migration")
	}
	if reason == "" {
		t.Fatal("expected skip reason")
	}
	if !strings.Contains(reason, "newer than current DBX release v0.8.1") {
		t.Fatalf("expected skip reason to reference current DBX release, got %q", reason)
	}
	if !strings.Contains(reason, "pre-releases excluded") {
		t.Fatalf("expected skip reason to mention pre-release policy, got %q", reason)
	}
}

func TestRunOSFlakeMigrationQueuesLatestStableUpdate(t *testing.T) {
	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFile("v0.8.1"),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.2.0-rc.1"},
				{Tag: "v1.2.0"},
				{Tag: "v1.1.0"},
			},
		},
	}

	var queuedActions []dogeboxd.Action
	ctx.Enqueue = func(action dogeboxd.Action) string {
		queuedActions = append(queuedActions, action)
		return "job-123"
	}

	jobID, queued, err := OSFlakeMigration.Run(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued {
		t.Fatal("expected migration to queue an update")
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
}

func TestRunOSFlakeMigrationQueuesLatestPrereleaseWhenEnabled(t *testing.T) {
	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFile("v0.9.0-rc.1"),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.3.0-rc.2"},
				{Tag: "v1.2.0"},
				{Tag: "v1.1.0"},
			},
		},
	}

	var queuedActions []dogeboxd.Action
	ctx.Enqueue = func(action dogeboxd.Action) string {
		queuedActions = append(queuedActions, action)
		return "job-prerelease"
	}

	jobID, queued, err := OSFlakeMigration.Run(ctx, core.MigrationRecord{
		Config: map[string]any{
			osFlakeIncludePreReleasesConfigKey: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued {
		t.Fatal("expected migration to queue a prerelease update")
	}
	if jobID != "job-prerelease" {
		t.Fatalf("expected job-prerelease, got %s", jobID)
	}
	if len(queuedActions) != 1 {
		t.Fatalf("expected exactly one queued action, got %d", len(queuedActions))
	}

	update, ok := queuedActions[0].(dogeboxd.SystemUpdate)
	if !ok {
		t.Fatalf("expected SystemUpdate action, got %T", queuedActions[0])
	}
	if update.Package != "os" || update.Version != "v1.3.0-rc.2" {
		t.Fatalf("unexpected queued update: %+v", update)
	}
}
