package definitions

import (
	"fmt"
	"log"
	"os"
	"regexp"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
)

var installedOSFlakePath = "/etc/nixos/flake.nix"
var currentSystemProfileActivatePath = "/nix/var/nix/profiles/system/activate"
var runningSystemActivatePath = "/run/current-system/activate"

const osFlakeIncludePreReleasesConfigKey = "includePreReleases"

var osFlakeMigrationRunPolicy = core.RunPolicy{}
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
//  2. require an installed OS flake version matching <=v0.9.0-rc.1,
//  3. select the latest eligible OS release,
//  4. compare that target against the installed OS flake version,
//  5. repair the current profile activation when rc8 landed partially, or
//  6. queue the normal SystemUpdate path when the target OS flake still needs to land.

var osFlakeVersionPattern = regexp.MustCompile(`(?m)^\s*dbxRelease\s*=\s*"(v[^"]+)"`)
var activationScriptVersionPattern = regexp.MustCompile(`(?m)^\s*echo\s+['"]?(v[^'"\s]+)['"]?\s*>\s*/opt/versioning/dbx\s*$`)

type osFlakeMigrationDecision struct {
	installedFlakeVersion string
	currentDBXRelease     string
	currentProfileRelease string
	runningSystemRelease  string
	targetVersion         string
	includePreReleases    bool
	complete              bool
	repairRequired        bool
	reason                string
}

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

func getActivationScriptRelease(readFile func(string) ([]byte, error), activatePath string) (string, error) {
	contents, err := readFile(activatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	match := activationScriptVersionPattern.FindStringSubmatch(string(contents))
	if len(match) != 2 {
		return "", nil
	}
	if err := core.ValidateSemverVersion(match[1], "activation script dbx release"); err != nil {
		return "", fmt.Errorf("activation script dbx release %q is not valid semver", match[1])
	}
	return match[1], nil
}

func semverIsAtOrAbove(version string, threshold string) (bool, error) {
	if version == "" {
		return false, nil
	}

	return core.VersionConstraintCheck(version, ">="+threshold)
}

func getNewerKnownVersion(versions ...string) string {
	newest := ""
	for _, candidate := range versions {
		if candidate == "" {
			continue
		}
		if newest == "" {
			newest = candidate
			continue
		}

		isNewer, err := core.VersionConstraintCheck(newest, "<"+candidate)
		if err != nil {
			continue
		}
		if isNewer {
			newest = candidate
		}
	}

	return newest
}

func detectOSFlakePartialState(installedFlakeVersion string, currentDBXRelease string, currentProfileRelease string, runningSystemRelease string) (bool, string, error) {
	currentProfileInRange, err := semverIsAtOrAbove(currentProfileRelease, osFlakeMigrationMetadata.Version)
	if err != nil {
		return false, "", err
	}
	runningSystemInRange, err := semverIsAtOrAbove(runningSystemRelease, osFlakeMigrationMetadata.Version)
	if err != nil {
		return false, "", err
	}
	if !currentProfileInRange && !runningSystemInRange {
		return false, "", nil
	}

	latestKnownRelease := getNewerKnownVersion(currentProfileRelease, runningSystemRelease)
	if latestKnownRelease == "" {
		return false, "", nil
	}

	if currentProfileRelease != "" && runningSystemRelease != "" {
		profileAheadOfRunning, err := core.VersionConstraintCheck(runningSystemRelease, "<"+currentProfileRelease)
		if err != nil {
			return false, "", err
		}
		if profileAheadOfRunning {
			return true, fmt.Sprintf("current system profile %s is newer than running system %s", currentProfileRelease, runningSystemRelease), nil
		}
	}

	installedBehindKnownRelease, err := core.VersionConstraintCheck(installedFlakeVersion, "<"+latestKnownRelease)
	if err != nil {
		return false, "", err
	}
	if installedBehindKnownRelease {
		return true, fmt.Sprintf("installed OS flake %s is older than activated profile %s", installedFlakeVersion, latestKnownRelease), nil
	}

	currentReleaseBehindKnownRelease, err := core.VersionConstraintCheck(currentDBXRelease, "<"+latestKnownRelease)
	if err != nil {
		return false, "", err
	}
	if currentReleaseBehindKnownRelease {
		return true, fmt.Sprintf("recorded DBX release %s is older than activated profile %s", currentDBXRelease, latestKnownRelease), nil
	}

	return false, "", nil
}

func isOSFlakeMigrationComplete(installedFlakeVersion string, currentDBXRelease string, currentProfileRelease string, runningSystemRelease string) (bool, error) {
	repairRequired, _, err := detectOSFlakePartialState(installedFlakeVersion, currentDBXRelease, currentProfileRelease, runningSystemRelease)
	if err != nil {
		return false, err
	}
	if repairRequired {
		return false, nil
	}

	for _, candidate := range []string{installedFlakeVersion, currentDBXRelease, runningSystemRelease} {
		ok, err := semverIsAtOrAbove(candidate, osFlakeMigrationMetadata.Version)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	if currentProfileRelease != "" {
		ok, err := semverIsAtOrAbove(currentProfileRelease, osFlakeMigrationMetadata.Version)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

func osFlakeMigrationRequirements(ctx core.Context, record core.MigrationRecord) (bool, string, error) {
	if ctx.Config.Recovery {
		return false, "running in recovery mode", nil
	}

	decision, err := determineOSFlakeMigrationDecision(ctx, record, true)
	if err != nil {
		return false, "", err
	}
	if decision.complete {
		if err := core.SetDoNotRun(ctx.Config, osFlakeMigrationMetadata.Name, true); err != nil {
			return false, "", err
		}
		log.Printf(
			"OS flake migrator marking migration complete after verifying target state: installed OS flake=%s, current DBX release=%s, current system profile=%s, running system=%s",
			decision.installedFlakeVersion,
			decision.currentDBXRelease,
			decision.currentProfileRelease,
			decision.runningSystemRelease,
		)
		return false, "verified target state already satisfies the migration", nil
	}
	if decision.reason != "" && !decision.repairRequired {
		return false, decision.reason, nil
	}

	return true, "", nil
}

func runOSFlakeMigration(ctx core.Context, record core.MigrationRecord) (string, bool, error) {
	decision, err := determineOSFlakeMigrationDecision(ctx, record, false)
	if err != nil {
		return "", false, err
	}
	if decision.complete {
		return "", false, nil
	}

	activeJobs, err := ctx.ActiveJobsOrDefault()()
	if err != nil {
		return "", false, err
	}
	for _, job := range activeJobs {
		if job.Action == (dogeboxd.SystemUpdate{}).ActionName() || job.Action == (dogeboxd.RepairSystemActivation{}).ActionName() {
			log.Printf("Skipping OS flake migrator queue because active job %s (%s) is already %s", job.ID, job.Action, job.Status)
			return "", false, nil
		}
	}

	if decision.repairRequired {
		log.Printf("Queueing OS flake activation repair because %s", decision.reason)
		jobID := ctx.Enqueue(dogeboxd.RepairSystemActivation{
			TargetVersion: decision.currentProfileRelease,
			Reason:        decision.reason,
		})
		return jobID, true, nil
	}
	if decision.targetVersion == "" {
		return "", false, nil
	}

	jobID := ctx.Enqueue(dogeboxd.SystemUpdate{
		Package: "os",
		Version: decision.targetVersion,
	})

	return jobID, true, nil
}

func determineOSFlakeMigrationDecision(ctx core.Context, record core.MigrationRecord, logDiscovery bool) (osFlakeMigrationDecision, error) {
	installedFlakeVersion, err := getInstalledOSFlakeVersion(ctx.ReadFileOrDefault(), installedOSFlakePath)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}

	currentDBXRelease := version.GetDBXRelease().Release
	currentProfileRelease, err := getActivationScriptRelease(ctx.ReadFileOrDefault(), currentSystemProfileActivatePath)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	runningSystemRelease, err := getActivationScriptRelease(ctx.ReadFileOrDefault(), runningSystemActivatePath)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}

	includePreReleases := record.BoolConfig(osFlakeIncludePreReleasesConfigKey)
	repairRequired, repairReason, err := detectOSFlakePartialState(installedFlakeVersion, currentDBXRelease, currentProfileRelease, runningSystemRelease)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	complete, err := isOSFlakeMigrationComplete(installedFlakeVersion, currentDBXRelease, currentProfileRelease, runningSystemRelease)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}

	decision := osFlakeMigrationDecision{
		installedFlakeVersion: installedFlakeVersion,
		currentDBXRelease:     currentDBXRelease,
		currentProfileRelease: currentProfileRelease,
		runningSystemRelease:  runningSystemRelease,
		includePreReleases:    includePreReleases,
		complete:              complete,
		repairRequired:        repairRequired,
		reason:                repairReason,
	}

	releases, err := system.GetUpgradableReleasesForVersionWithFetcher(currentDBXRelease, includePreReleases, ctx.RepoTagsFetcherOrDefault())
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if logDiscovery {
		latestEligibleRelease := "none"
		if len(releases) > 0 {
			latestEligibleRelease = releases[0].Version
		}
		log.Printf(
			"OS flake migrator release discovery: installed OS flake=%s, current DBX release=%s, current system profile=%s, running system=%s, includePreReleases=%t, latest eligible OS release=%s",
			installedFlakeVersion,
			currentDBXRelease,
			currentProfileRelease,
			runningSystemRelease,
			includePreReleases,
			latestEligibleRelease,
		)
	}

	if repairRequired || complete {
		return decision, nil
	}

	matchesInstalledVersionConstraint, err := core.VersionConstraintCheck(installedFlakeVersion, "<="+osFlakeMigrationMetadata.Version)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if !matchesInstalledVersionConstraint {
		decision.reason = fmt.Sprintf("installed OS flake version %s is newer than %s", installedFlakeVersion, osFlakeMigrationMetadata.Version)
		return decision, nil
	}

	if len(releases) == 0 {
		preReleasePolicy := "pre-releases excluded"
		if includePreReleases {
			preReleasePolicy = "pre-releases included"
		}
		decision.reason = fmt.Sprintf("no eligible OS releases are available newer than current DBX release %s (%s)", currentDBXRelease, preReleasePolicy)
		return decision, nil
	}

	targetVersion := releases[0].Version
	targetIsNewer, err := core.VersionConstraintCheck(installedFlakeVersion, "<"+targetVersion)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if !targetIsNewer {
		decision.targetVersion = targetVersion
		decision.reason = fmt.Sprintf("installed OS flake version %s is not older than inferred target %s", installedFlakeVersion, targetVersion)
		return decision, nil
	}

	decision.targetVersion = targetVersion
	return decision, nil
}

func QueueOSFlakeMigratorIfNeeded(
	config dogeboxd.ServerConfig,
	enqueue func(dogeboxd.Action) string,
	activeJobs func() ([]dogeboxd.JobRecord, error),
) (string, bool, error) {
	return core.RunMigrations(core.Context{
		Config:          config,
		Enqueue:         enqueue,
		ActiveJobs:      activeJobs,
		ReadFile:        os.ReadFile,
		RepoTagsFetcher: &system.DefaultRepoTagsFetcher{},
	}, []core.Migration{OSFlakeMigration})
}
