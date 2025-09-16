package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"

	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

var RELEASE_REPOSITORY = "https://github.com/dogebox-wg/os.git"
var REBUILD_COMMAND_PREFIX = "sudo"

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

/*
func gitGetSingleFile(repo string, file string, branch string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "git-get-single-file")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Shallow clone
	gitRepo, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL:           repo,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	})

	if err != nil {
		return nil, err
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return nil, err
	}

	// Read the content of the desired file
	content, err := os.ReadFile(filepath.Join(worktree.Filesystem.Root(), file))
	if err != nil {
		return nil, err
	}

	return content, nil
}

func getTagHashes(repo string, tag string) (map[string]string, error) {
	flakelock, err := gitGetSingleFile(repo, "/flake.lock", tag)

	var data map[string]any
	if err = json.Unmarshal([]byte(flakelock), &data); err != nil {
		return nil, err
	}

	var tagHashes map[string]string

	// TODO : Look for just 'dkm','dogeboxd' an 'dpanel' for now
	for _, pkg := range [...]string{"dkm", "dogeboxd", "dpanel"} {
		locks := data["locks"].(map[string]any)
		nodes := locks["nodes"].(map[string]any)
		pkgStruct := nodes[pkg].(map[string]any)
		locked := pkgStruct["locked"].(map[string]any)
		hash := locked["narHash"].(string)

		if jsonRev := locked["rev"].(string); jsonRev != tag {
			return nil, fmt.Errorf("rev requested doesn't match rev in system flake.lock")
		}

		tagHashes[pkg] = hash
	}

	return tagHashes, nil
}
*/

func DoSystemUpdate(pkg string, updateVersion string) error {
	upgradableReleases, err := GetUpgradableReleases(false)
	if err != nil {
		return err
	}

	// We _only_ support the "os" package for now.
	if pkg != "os" {
		return fmt.Errorf("invalid package to upgrade: %s", pkg)
	}

	ok := false
	for _, release := range upgradableReleases {
		if release.Version == updateVersion {
			ok = true
			break
		}
	}

	if !ok {
		return fmt.Errorf("release %s is not available for %s", updateVersion, pkg)
	}

	/*
		tagHashes, err := getTagHashes(RELEASE_REPOSITORY, updateVersion)
		if err != nil {
			return err
		}

		// Update our filesystem with our new package version tags.
		version.SetPackageVersion("dogeboxd", updateVersion, tagHashes["dogeboxd"])
		version.SetPackageVersion("dpanel", updateVersion, tagHashes["dpanel"])
		version.SetPackageVersion("dkm", updateVersion, tagHashes["dkm"])

		// Trigger a rebuild of the system. This will read our new version information.
		cmd := exec.Command(REBUILD_COMMAND_PREFIX, "_dbxroot", "nix", "rs")
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run dbx-upgrade: %w", err)
		}
	*/

	cmd := exec.Command(REBUILD_COMMAND_PREFIX, "_dbxroot", "nix", "rs", "--set-release", updateVersion)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We probably won't even get here if dogeboxd is restarted/upgraded during this process.
	return err
}
