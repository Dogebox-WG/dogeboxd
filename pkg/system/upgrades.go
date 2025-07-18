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

func GetUpgradableReleases() ([]UpgradableRelease, error) {
	dbxRelease := version.GetDBXRelease()

	tags, err := getRepoTags(RELEASE_REPOSITORY)
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

	// We _only_ support the "dogebox" package for now.
	if pkg != "dogebox" {
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

	// TODO:
	// version.SetPackageVersion("dogeboxd", updateVersion)
	// version.SetPackageVersion("dpanel", updateVersion)
	// version.SetPackageVersion("dkm", updateVersion)

	// TODO: We might need to run `nix flake update` here? Would need to be a new _dbxroot command.

	// Trigger a rebuild of the system. This will read our new version information.
	cmd := exec.Command("sudo", "_dbxroot", "nix", "rs")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We probably won't even get here if dogeboxd is restarted/upgraded during this process.
	return nil
}
