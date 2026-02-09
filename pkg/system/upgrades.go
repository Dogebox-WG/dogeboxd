package system

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

var RELEASE_REPOSITORY = "https://github.com/dogebox-wg/os.git"
var SUDO_COMMAND = "sudo"

// semverSortTags sorts a slice of RepositoryTag by semver version
// direction: "desc" for descending (highest first), "asc" for ascending (lowest first)
func semverSortTags(tags []RepositoryTag, direction string) {
	sort.Slice(tags, func(i, j int) bool {
		if direction == "desc" {
			return semver.Compare(tags[i].Tag, tags[j].Tag) > 0
		}
		return semver.Compare(tags[i].Tag, tags[j].Tag) < 0
	})
}

// semverSortReleases sorts a slice of UpgradableRelease by semver version
// direction: "desc" for descending (highest first), "asc" for ascending (lowest first)
func semverSortReleases(releases []UpgradableRelease, direction string) {
	sort.Slice(releases, func(i, j int) bool {
		if direction == "desc" {
			return semver.Compare(releases[i].Version, releases[j].Version) > 0
		}
		return semver.Compare(releases[i].Version, releases[j].Version) < 0
	})
}

type RepositoryTag struct {
	Tag string
}

func getRepoTags(repo string) ([]RepositoryTag, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repo},
	})

	refs, err := rem.List(&git.ListOptions{
		PeelingOption: git.AppendPeeled,
	})
	if err != nil {
		log.Printf("Failed to get repo %s tags: %v", repo, err)
		return []RepositoryTag{}, err
	}

	var tags []RepositoryTag
	for _, ref := range refs {
		if ref.Name().IsTag() && semver.IsValid(ref.Name().Short()) {
			tags = append(tags, RepositoryTag{
				Tag: ref.Name().Short(),
			})
		}
	}

	semverSortTags(tags, "desc") // Sort descending (highest first)
	return tags, nil
}

type UpgradableRelease struct {
	Version    string
	ReleaseURL string
	Summary    string
}

type InvalidUpdatePackageError struct {
	Package string
}

func (e InvalidUpdatePackageError) Error() string {
	return fmt.Sprintf("invalid package to upgrade: %s", e.Package)
}

type UpdateVersionUnavailableError struct {
	Package string
	Version string
}

func (e UpdateVersionUnavailableError) Error() string {
	return fmt.Sprintf("release %s is not available for %s", e.Version, e.Package)
}

// RepoTagsFetcher interface for mocking getRepoTags
type RepoTagsFetcher interface {
	GetRepoTags(repo string) ([]RepositoryTag, error)
}

// DefaultRepoTagsFetcher implements RepoTagsFetcher using the actual git implementation
type DefaultRepoTagsFetcher struct{}

func (d *DefaultRepoTagsFetcher) GetRepoTags(repo string) ([]RepositoryTag, error) {
	return getRepoTags(repo)
}

// Global variable to allow dependency injection for testing
var repoTagsFetcher RepoTagsFetcher = &DefaultRepoTagsFetcher{}

func GetUpgradableReleases(includePreReleases bool) ([]UpgradableRelease, error) {
	return GetUpgradableReleasesWithFetcher(includePreReleases, repoTagsFetcher)
}

func GetUpgradableReleasesWithFetcher(includePreReleases bool, fetcher RepoTagsFetcher) ([]UpgradableRelease, error) {
	dbxRelease := version.GetDBXRelease()

	tags, err := fetcher.GetRepoTags(RELEASE_REPOSITORY)
	if err != nil {
		return []UpgradableRelease{}, err
	}

	var upgradableTags []UpgradableRelease
	for _, tag := range tags {
		release := UpgradableRelease{
			Version:    tag.Tag,
			ReleaseURL: fmt.Sprintf("https://github.com/dogebox-wg/os/releases/tag/%s", tag.Tag),
			Summary:    "Update for Dogeboxd / DKM / DPanel",
		}

		if semver.Compare(tag.Tag, dbxRelease.Release) > 0 {
			// If not including pre-releases, filter out pre-release versions
			if !includePreReleases && semver.Prerelease(tag.Tag) != "" {
				continue
			}
			upgradableTags = append(upgradableTags, release)
		}
	}

	semverSortReleases(upgradableTags, "desc") // Sort descending (highest first)

	return upgradableTags, nil
}

func DoSystemUpdate(pkg string, updateVersion string, logger dogeboxd.SubLogger) error {
	upgradableReleases, err := GetUpgradableReleases(false)
	if err != nil {
		return err
	}

	// We _only_ support the "os" package for now.
	if pkg != "os" {
		return InvalidUpdatePackageError{Package: pkg}
	}

	ok := false
	for _, release := range upgradableReleases {
		if release.Version == updateVersion {
			ok = true
			break
		}
	}

	if !ok {
		return UpdateVersionUnavailableError{Package: pkg, Version: updateVersion}
	}

	cmd := exec.Command(SUDO_COMMAND, "_dbxroot", "nix", "rs", "--set-release", updateVersion)
	if logger != nil {
		logger.Logf("Running command: %s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))
		cmd.Stdout = io.MultiWriter(os.Stdout, dogeboxd.NewLineWriter(func(s string) {
			logger.Log(s)
		}))
		cmd.Stderr = io.MultiWriter(os.Stderr, dogeboxd.NewLineWriter(func(s string) {
			logger.Log(s)
		}))
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We probably won't even get here if dogeboxd is restarted/upgraded during this process.
	return err
}
