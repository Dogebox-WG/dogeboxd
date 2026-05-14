package definitions

import (
	"fmt"
	"os"
	"regexp"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
)

const installedOSFlakePath = "/etc/nixos/flake.nix"
const osFlakeIncludePreReleasesConfigKey = "includePreReleases"

var osFlakeMigrationRunPolicy = core.RunPolicy{MaxRuns: 1}
var osFlakeMigrationMetadata = struct {
	Name    string
	Version string
}{
	Name:    "pre_0.9_os_flake",
	Version: "v0.9.0-rc.1",
}

var OSFlakeMigration = core.Migration{
	Name:         osFlakeMigrationMetadata.Name,
	DisplayName:  "OS flake migrator",
	Version:      osFlakeMigrationMetadata.Version,
	RunPolicy:    osFlakeMigrationRunPolicy,
	Requirements: osFlakeMigrationRequirements,
	Run:          runOSFlakeMigration,
}

// This migration handles upgrading to the v0.9.0 OS flake. Those
// systems may boot into a new dogeboxd package while still running an older
// /etc/nixos flake, because the old updater did not pass a staged flake
// directory into the rebuild step.
//
// The migration process:
//  1. read the installed OS flake version from /etc/nixos/flake.nix,
//  2. require an installed OS flake version matching >=v0.9.0-rc.1,
//  3. select the latest eligible OS release,
//  4. compare that target against the installed OS flake version,
//  5. queue the normal SystemUpdate path once, and
//  6. record a run in migrations.json so startup does not loop.

var osFlakeVersionPattern = regexp.MustCompile(`(?m)^\s*dbxRelease\s*=\s*"(v[^"]+)"`)

func getInstalledOSFlakeVersion(readFile func(string) ([]byte, error), flakePath string) (string, error) {
	flakeContents, err := readFile(flakePath)
	if err != nil {
		return "", err
	}

	match := osFlakeVersionPattern.FindStringSubmatch(string(flakeContents))
	if len(match) != 2 {
		return "", fmt.Errorf("installed os flake version not found in %s", flakePath)
	}
	if err := core.ValidateSemverVersion(match[1], "installed os flake"); err != nil {
		return "", fmt.Errorf("installed os flake version %q is not valid semver", match[1])
	}
	return match[1], nil
}

func osFlakeMigrationRequirements(ctx core.Context, record core.MigrationRecord) (bool, string, error) {
	if ctx.Config.Recovery {
		return false, "running in recovery mode", nil
	}

	_, _, applies, reason, err := determineOSFlakeMigrationTarget(ctx, record)
	if err != nil {
		return false, "", err
	}
	if !applies {
		return false, reason, nil
	}

	return true, "", nil
}

func runOSFlakeMigration(ctx core.Context, record core.MigrationRecord) (string, bool, error) {
	_, targetVersion, applies, _, err := determineOSFlakeMigrationTarget(ctx, record)
	if err != nil {
		return "", false, err
	}
	if !applies {
		return "", false, nil
	}

	jobID := ctx.Enqueue(dogeboxd.SystemUpdate{
		Package: "os",
		Version: targetVersion,
	})

	return jobID, true, nil
}

func determineOSFlakeMigrationTarget(ctx core.Context, record core.MigrationRecord) (string, string, bool, string, error) {
	installedFlakeVersion, err := getInstalledOSFlakeVersion(ctx.ReadFileOrDefault(), installedOSFlakePath)
	if err != nil {
		return "", "", false, "", err
	}

	matchesInstalledVersionConstraint, err := core.VersionConstraintCheck(installedFlakeVersion, ">="+osFlakeMigrationMetadata.Version)
	if err != nil {
		return "", "", false, "", err
	}
	if !matchesInstalledVersionConstraint {
		return installedFlakeVersion, "", false, fmt.Sprintf("installed OS flake version %s is older than %s", installedFlakeVersion, osFlakeMigrationMetadata.Version), nil
	}

	releases, err := system.GetUpgradableReleasesWithFetcher(record.BoolConfig(osFlakeIncludePreReleasesConfigKey), ctx.RepoTagsFetcherOrDefault())
	if err != nil {
		return installedFlakeVersion, "", false, "", err
	}
	if len(releases) == 0 {
		return installedFlakeVersion, "", false, fmt.Sprintf("no eligible OS releases are available for %s", osFlakeMigrationMetadata.Version), nil
	}

	targetVersion := releases[0].Version
	targetIsNewer, err := core.VersionConstraintCheck(installedFlakeVersion, "<"+targetVersion)
	if err != nil {
		return installedFlakeVersion, targetVersion, false, "", err
	}
	if !targetIsNewer {
		return installedFlakeVersion, targetVersion, false, fmt.Sprintf("installed OS flake version %s is not older than inferred target %s", installedFlakeVersion, targetVersion), nil
	}

	return installedFlakeVersion, targetVersion, true, "", nil
}

func QueueOSFlakeMigratorIfNeeded(config dogeboxd.ServerConfig, enqueue func(dogeboxd.Action) string) (string, bool, error) {
	return core.RunMigrations(core.Context{
		Config:          config,
		Enqueue:         enqueue,
		ReadFile:        os.ReadFile,
		RepoTagsFetcher: &system.DefaultRepoTagsFetcher{},
	}, []core.Migration{OSFlakeMigration})
}
