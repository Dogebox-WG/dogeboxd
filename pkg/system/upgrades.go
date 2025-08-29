package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

const RELEASE_REPOSITORY = "https://github.com/dogebox-wg/os.git"

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

	return tags, nil
}

type UpgradableRelease struct {
	Version    string
	ReleaseURL string
	Summary    string
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

func GetUpgradableReleases() ([]UpgradableRelease, error) {
	return GetUpgradableReleasesWithFetcher(repoTagsFetcher)
}

func GetUpgradableReleasesWithFetcher(fetcher RepoTagsFetcher) ([]UpgradableRelease, error) {
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
			upgradableTags = append(upgradableTags, release)
		}
	}

	return upgradableTags, nil
}

func DoSystemUpdate(pkg string, updateVersion string) error {
	upgradableReleases, err := GetUpgradableReleases()
	if err != nil {
		return err
	}

	// We _only_ support the "os" package for now.
	if pkg != "os" {
		return fmt.Errorf("Invalid package to upgrade: %s", pkg)
	}

	ok := false
	for _, release := range upgradableReleases {
		if release.Version == updateVersion {
			ok = true
			break
		}
	}

	if !ok {
		return fmt.Errorf("Release %s is not available for %s", updateVersion, pkg)
	}

	// Update our filesystem with our new package version tags.
	oldCWD, err := os.Getwd()
	if err := os.Chdir("/etc/nixos"); err != nil {
		return fmt.Errorf("problem entering system config directory /etc/nixos: %w", err)
	}

	cmd := exec.Command("git", "reset", "--hard")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset os git repo in /etc/nixos to a known clean state: %w", err)
	}

	exec.Command("git", "fetch", "--tags")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch tags for os git repo: %w", err)
	}

	exec.Command("git", "checkout", updateVersion)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout desired desired updateVersion of %s: %w", updateVersion, err)
	}

	version.SetPackageVersion("dogeboxd", updateVersion)
	version.SetPackageVersion("dpanel", updateVersion)
	version.SetPackageVersion("dkm", updateVersion)

	// Trigger a rebuild of the system. This will read our new version information.
	cmd = exec.Command("sudo", "_dbxroot", "nix", "rs")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We probably won't even get here if dogeboxd is restarted/upgraded during this process.
	err = os.Chdir(oldCWD)
	return nil
}
