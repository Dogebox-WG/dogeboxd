package version

import (
	"os"
	"os/exec"
	"strings"

	"github.com/carlmjohnson/versioninfo"
)

type DBXVersionInfoGit struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type DBXVersionInputTuple struct {
	Rev  string `json:"rev"`
	Hash string `json:"hash"`
}

type DBXVersionInfo struct {
	Release  string                          `json:"release"`
	Packages map[string]DBXVersionInputTuple `json:"packages"`
	Git      DBXVersionInfoGit               `json:"git"`
}

func GetDBXRelease() *DBXVersionInfo {
	release := "unknown"

	if dbxReleaseData, err := os.ReadFile("/opt/versioning/dbx"); err == nil {
		release = strings.TrimSpace(string(dbxReleaseData))
	} else {
		// Fallback: try to get the current git branch
		if gitBranch, err := getGitBranch(); err == nil && gitBranch != "" {
			release = "file missing, current branch: '" + gitBranch + "'"
		}
	}

	packages := make(map[string]DBXVersionInputTuple)
	if entries, err := os.ReadDir("/opt/versioning"); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pkgName := entry.Name()
				var tuple DBXVersionInputTuple

				if revData, err := os.ReadFile("/opt/versioning/" + pkgName + "/rev"); err == nil {
					tuple.Rev = strings.TrimSpace(string(revData))
				}

				if hashData, err := os.ReadFile("/opt/versioning/" + pkgName + "/hash"); err == nil {
					tuple.Hash = strings.TrimSpace(string(hashData))
				}

				packages[pkgName] = tuple
			}
		}
	}

	return &DBXVersionInfo{
		Release:  release,
		Packages: packages,
		Git: DBXVersionInfoGit{
			Commit: versioninfo.Revision,
			Dirty:  versioninfo.DirtyBuild,
		},
	}
}

// getGitBranch attempts to get the current git branch name
func getGitBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	branch := strings.TrimSpace(string(output))
	return branch, nil
}
