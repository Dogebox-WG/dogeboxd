package system

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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

func cloneReleaseRepository(destination, version string) error {
	return cloneReleaseRepositoryAtRef(destination, version, "")
}

func resolveRepositoryRevision(repo *git.Repository, ref string) (*plumbing.Hash, error) {
	candidates := []plumbing.Revision{
		plumbing.Revision(ref),
		plumbing.Revision("refs/heads/" + ref),
		plumbing.Revision("refs/tags/" + ref),
		plumbing.Revision("refs/remotes/origin/" + ref),
	}

	for _, candidate := range candidates {
		hash, err := repo.ResolveRevision(candidate)
		if err == nil {
			return hash, nil
		}
	}

	return nil, fmt.Errorf("failed to resolve OS ref %s", ref)
}

func cloneReleaseRepositoryAtRef(destination, version string, osRef string) error {
	if osRef == "" {
		_, err := git.PlainClone(destination, false, &git.CloneOptions{
			URL:           RELEASE_REPOSITORY,
			ReferenceName: plumbing.NewTagReferenceName(version),
			SingleBranch:  true,
			Depth:         1,
		})
		if err != nil {
			return fmt.Errorf("failed to clone OS release %s: %w", version, err)
		}

		return nil
	}

	repo, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL: RELEASE_REPOSITORY,
	})
	if err != nil {
		return fmt.Errorf("failed to clone OS repository for ref %s: %w", osRef, err)
	}

	hash, err := resolveRepositoryRevision(repo, osRef)
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to open OS repository worktree: %w", err)
	}

	if err := worktree.Checkout(&git.CheckoutOptions{
		Hash:  *hash,
		Force: true,
	}); err != nil {
		return fmt.Errorf("failed to checkout OS ref %s: %w", osRef, err)
	}

	return nil
}

func getRepositoryHeadHash(repoDir string) (string, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return "", fmt.Errorf("failed to open cloned repository %s: %w", repoDir, err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to read cloned repository HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

func buildStagedReleaseDirPath(tmpDir string, updateVersion string, commitHash string) string {
	return filepath.Join(tmpDir, fmt.Sprintf("os-upgrade-%s-%s", updateVersion, commitHash))
}

func stageReleaseFlake(tmpDir, updateVersion string, osRef string, logger dogeboxd.SubLogger) (string, error) {
	return stageReleaseFlakeWithClone(tmpDir, updateVersion, osRef, logger, cloneReleaseRepositoryAtRef)
}

func stageReleaseFlakeWithClone(tmpDir, updateVersion string, osRef string, logger dogeboxd.SubLogger, cloneFunc func(string, string, string) error) (string, error) {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}

	cloneDir, err := os.MkdirTemp(tmpDir, "os-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for OS release %s: %w", updateVersion, err)
	}

	if logger != nil {
		if osRef != "" {
			logger.Logf("Cloning OS release %s from ref %s into %s", updateVersion, osRef, cloneDir)
		} else {
			logger.Logf("Cloning OS release %s into %s", updateVersion, cloneDir)
		}
	}

	if err := cloneFunc(cloneDir, updateVersion, osRef); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", err
	}

	flakePath := filepath.Join(cloneDir, "flake.nix")
	if _, err := os.Stat(flakePath); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", fmt.Errorf("staged OS release %s is missing flake.nix: %w", updateVersion, err)
	}

	commitHash, err := getRepositoryHeadHash(cloneDir)
	if err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", err
	}

	finalDir := buildStagedReleaseDirPath(tmpDir, updateVersion, commitHash)
	if err := os.RemoveAll(finalDir); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", fmt.Errorf("failed to clear existing staged OS release dir %s: %w", finalDir, err)
	}

	if err := os.Rename(cloneDir, finalDir); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", fmt.Errorf("failed to move staged OS release into %s: %w", finalDir, err)
	}

	return finalDir, nil
}

func doSystemUpdate(pkg string, updateVersion string, osRef string, tmpDir string, logger dogeboxd.SubLogger) error {
	return doSystemUpdateWithDependencies(pkg, updateVersion, osRef, tmpDir, logger, cloneReleaseRepositoryAtRef, exec.Command)
}

func doSystemUpdateWithDependencies(
	pkg string,
	updateVersion string,
	osRef string,
	tmpDir string,
	logger dogeboxd.SubLogger,
	cloneFunc func(string, string, string) error,
	execCommand func(string, ...string) *exec.Cmd,
) error {
	// We _only_ support the "os" package for now.
	if pkg != "os" {
		return InvalidUpdatePackageError{Package: pkg}
	}

	rebuildRelease := updateVersion
	if osRef == "" {
		upgradableReleases, err := GetUpgradableReleases(true)
		if err != nil {
			return err
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
	} else {
		// Direct OS ref upgrades are dev-only and intentionally keep the
		// existing package override release while swapping only the OS flake.
		rebuildRelease = version.GetDBXRelease().Release
	}

	stagedFlakeDir, err := stageReleaseFlakeWithClone(tmpDir, updateVersion, osRef, logger, cloneFunc)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(stagedFlakeDir); err != nil && logger != nil {
			logger.Errf("Failed to clean up staged flake %s: %v", stagedFlakeDir, err)
		}
	}()

	cmd := execCommand(SUDO_COMMAND, "_dbxroot", "nix", "rs", "--flake-dir", stagedFlakeDir, "--set-release", rebuildRelease)
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

func (t SystemUpdater) DoSystemUpdate(pkg string, updateVersion string, osRef string, logger dogeboxd.SubLogger) error {
	return doSystemUpdate(pkg, updateVersion, osRef, t.config.TmpDir, logger)
}

func DoSystemUpdate(pkg string, updateVersion string, osRef string, logger dogeboxd.SubLogger) error {
	return doSystemUpdate(pkg, updateVersion, osRef, "", logger)
}
