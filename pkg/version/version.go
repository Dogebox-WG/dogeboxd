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

type DBXVersionInfo struct {
	Release string            `json:"release"`
	NurHash string            `json:"nurHash"`
	Git     DBXVersionInfoGit `json:"git"`
}

func GetDBXRelease() *DBXVersionInfo {
	release := "unknown"
	nurHash := "unknown"

	if dbxReleaseData, err := os.ReadFile("/opt/dbx-release"); err == nil {
		release = strings.TrimSpace(string(dbxReleaseData))
	}

	if nurHashData, err := os.ReadFile("/opt/dbx-nur-hash"); err == nil {
		nurHash = strings.TrimSpace(string(nurHashData))
	}

	return &DBXVersionInfo{
		Release: release,
		NurHash: nurHash,
		Git: DBXVersionInfoGit{
			Commit: versioninfo.Revision,
			Dirty:  versioninfo.DirtyBuild,
		},
	}
}
