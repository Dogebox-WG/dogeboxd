package version

import (
	"os"
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
