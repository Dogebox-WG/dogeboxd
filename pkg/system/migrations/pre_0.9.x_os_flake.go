package migrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/mod/semver"
)

const installedOSFlakePath = "/etc/nixos/flake.nix"
const osFlakeMigratorMarkerFilename = "os_flake_migrator_attempted"
const osFlakeMigratorCutoverVersion = "v0.9.0"

var errNoMatchingOSFlakeRelease = errors.New("no matching os release found for current dogeboxd revision")

// This migration repairs systems that upgraded from pre-0.9 releases. Those
// systems may boot into a new dogeboxd package while still running an older
// /etc/nixos flake, because the old updater did not pass a staged flake
// directory into the rebuild step.
//
// The migration keeps its job small:
// 1. read the installed OS flake version from /etc/nixos/flake.nix,
// 2. detect whether it predates the 0.9 generation,
// 3. infer which OS tag the user picked by matching the running dogeboxd
//    revision against OS release flake locks,
// 4. queue the normal SystemUpdate path once, and
// 5. write a marker file so startup does not loop on repeated failures.
func getOSFlakeMigratorMarkerPath(config dogeboxd.ServerConfig) string {
	return filepath.Join(config.DataDir, osFlakeMigratorMarkerFilename)
}

var osFlakeVersionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*defaultDbxRelease\s*=\s*"(v[^"]+)"`),
	regexp.MustCompile(`(?m)^\s*dbxRelease\s*=\s*"(v[^"]+)"`),
}

func getInstalledOSFlakeVersion(readFile func(string) ([]byte, error), flakePath string) (string, error) {
	flakeContents, err := readFile(flakePath)
	if err != nil {
		return "", err
	}

	content := string(flakeContents)
	for _, pattern := range osFlakeVersionPatterns {
		match := pattern.FindStringSubmatch(content)
		if len(match) < 2 {
			continue
		}
		if !semver.IsValid(match[1]) {
			return "", fmt.Errorf("installed os flake version %q is not valid semver", match[1])
		}
		return match[1], nil
	}

	return "", fmt.Errorf("installed os flake version not found in %s", flakePath)
}

func osFlakeNeedsMigration(installedFlakeVersion string) bool {
	return semver.Compare(semver.MajorMinor(installedFlakeVersion), osFlakeMigratorCutoverVersion) < 0
}

func shouldSkipOSFlakeMigrator(config dogeboxd.ServerConfig) (bool, error) {
	_, err := os.Stat(getOSFlakeMigratorMarkerPath(config))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func writeOSFlakeMigratorMarker(config dogeboxd.ServerConfig) error {
	markerPath := getOSFlakeMigratorMarkerPath(config)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
		return err
	}

	file, err := os.Create(markerPath)
	if err != nil {
		return err
	}
	return file.Close()
}

// Each OS release pins a specific dogeboxd revision in its flake.lock. The
// migration uses that to recover the release the user originally selected.
func getDogeboxdRevisionFromOSRelease(tmpDir string, releaseVersion string) (string, error) {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}

	cloneDir, err := os.MkdirTemp(tmpDir, "os-release-match-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(cloneDir)

	_, err = git.PlainClone(cloneDir, false, &git.CloneOptions{
		URL:           system.RELEASE_REPOSITORY,
		ReferenceName: plumbing.NewTagReferenceName(releaseVersion),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone OS release %s: %w", releaseVersion, err)
	}

	flakeLockContents, err := os.ReadFile(filepath.Join(cloneDir, "flake.lock"))
	if err != nil {
		return "", err
	}

	var flakeLock struct {
		Nodes map[string]struct {
			Locked struct {
				Rev string `json:"rev"`
			} `json:"locked"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(flakeLockContents, &flakeLock); err != nil {
		return "", err
	}

	dogeboxdNode, ok := flakeLock.Nodes["dogeboxd"]
	if !ok || dogeboxdNode.Locked.Rev == "" {
		return "", fmt.Errorf("release %s does not define a dogeboxd revision in flake.lock", releaseVersion)
	}

	return dogeboxdNode.Locked.Rev, nil
}

func inferTargetReleaseForDogeboxdRevision(
	releases []system.UpgradableRelease,
	currentDogeboxdRevision string,
	releaseDogeboxdRevision func(string, string) (string, error),
	tmpDir string,
) (string, error) {
	if currentDogeboxdRevision == "" || currentDogeboxdRevision == "unknown" {
		return "", fmt.Errorf("current dogeboxd revision is unknown")
	}

	for _, release := range releases {
		releaseRevision, err := releaseDogeboxdRevision(tmpDir, release.Version)
		if err != nil {
			return "", err
		}

		if releaseRevision == currentDogeboxdRevision {
			return release.Version, nil
		}
	}

	return "", errNoMatchingOSFlakeRelease
}

// QueueOSFlakeMigratorIfNeeded checks whether the installed OS flake still
// needs the one-shot post-upgrade migration and, if so, queues the normal OS
// SystemUpdate path.
func QueueOSFlakeMigratorIfNeeded(config dogeboxd.ServerConfig, enqueue func(dogeboxd.Action) string) (string, bool, error) {
	return queueOSFlakeMigratorIfNeeded(config, enqueue, os.ReadFile, &system.DefaultRepoTagsFetcher{}, getDogeboxdRevisionFromOSRelease)
}

func queueOSFlakeMigratorIfNeeded(
	config dogeboxd.ServerConfig,
	enqueue func(dogeboxd.Action) string,
	readFile func(string) ([]byte, error),
	fetcher system.RepoTagsFetcher,
	releaseDogeboxdRevision func(string, string) (string, error),
) (string, bool, error) {
	if config.Recovery {
		log.Printf("Skipping OS flake migrator in recovery mode")
		return "", false, nil
	}

	installedFlakeVersion, err := getInstalledOSFlakeVersion(readFile, installedOSFlakePath)
	if err != nil {
		return "", false, err
	}
	if !osFlakeNeedsMigration(installedFlakeVersion) {
		log.Printf("Skipping OS flake migrator because installed OS flake version %s is already on the %s generation", installedFlakeVersion, osFlakeMigratorCutoverVersion)
		return "", false, nil
	}

	skip, err := shouldSkipOSFlakeMigrator(config)
	if err != nil {
		return "", false, err
	}
	if skip {
		log.Printf("Skipping OS flake migrator because it was already attempted")
		return "", false, nil
	}

	currentDogeboxdRevision := version.GetDBXRelease().Packages["dogeboxd"].Rev
	releases, err := system.GetUpgradableReleasesWithFetcher(true, fetcher)
	if err != nil {
		return "", false, err
	}

	targetVersion, err := inferTargetReleaseForDogeboxdRevision(releases, currentDogeboxdRevision, releaseDogeboxdRevision, config.TmpDir)
	if err != nil {
		if errors.Is(err, errNoMatchingOSFlakeRelease) {
			log.Printf("Skipping OS flake migrator because no OS release matches dogeboxd revision %s", currentDogeboxdRevision)
			return "", false, nil
		}
		return "", false, err
	}

	if semver.Compare(targetVersion, installedFlakeVersion) <= 0 {
		log.Printf("Skipping OS flake migrator because installed OS flake version %s is not older than inferred target %s", installedFlakeVersion, targetVersion)
		return "", false, nil
	}

	jobID := enqueue(dogeboxd.SystemUpdate{
		Package: "os",
		Version: targetVersion,
	})
	log.Printf("Queueing OS flake migrator for target %s", targetVersion)

	if err := writeOSFlakeMigratorMarker(config); err != nil {
		return jobID, true, err
	}

	return jobID, true, nil
}
