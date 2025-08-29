package version

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	
	// Allow override for testing
	versionPath := "/opt/versioning"
	if overridePath := os.Getenv("VERSION_PATH_OVERRIDE"); overridePath != "" {
		versionPath = overridePath
	}

	if dbxReleaseData, err := os.ReadFile(filepath.Join(versionPath, "dbx")); err == nil {
		release = strings.TrimSpace(string(dbxReleaseData))
	}

	packages := make(map[string]DBXVersionInputTuple)
	if entries, err := os.ReadDir(versionPath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pkgName := entry.Name()
				var tuple DBXVersionInputTuple

				if revData, err := os.ReadFile(filepath.Join(versionPath, pkgName, "rev")); err == nil {
					tuple.Rev = strings.TrimSpace(string(revData))
				}

				if hashData, err := os.ReadFile(filepath.Join(versionPath, pkgName, "hash")); err == nil {
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

func SetPackageVersion(pkg string, rev string) (bool, error) {
	metaDir := "/opt/versioning/" + pkg

	// Check 'rev' and 'hash' files exist for this package
	if _, err := os.Stat(metaDir + "/rev"); err != nil {
		return false, err
	}

	if _, err := os.Stat(metaDir + "/hash"); err != nil {
		return false, err
	}

	// TODO : check hash and rev match? Check git checkout is clean?

	// Retrieve hash value from system flake.lock
	cmd := exec.Command("nix", "flake", "metadata", "/etc/nixos", "--json")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// We don't know what the package name will be at compile time, so this is
	// to traverse the JSON structure at runtime
	var data map[string]any
	if err = json.Unmarshal([]byte(output), &data); err != nil {
		return false, err
	}

	locks := data["locks"].(map[string]any)
	nodes := locks["nodes"].(map[string]any)
	pkgStruct := nodes[pkg].(map[string]any)
	locked := pkgStruct["locked"].(map[string]any)
	hash := locked["narHash"].(string)

	if jsonRev := locked["rev"].(string); jsonRev != rev {
		return false, fmt.Errorf("rev requested doesn't match rev in system flake.lock")
	}

	// write current 'rev' and 'hash' to 'prevRev' and 'prevHash'
	if err = os.Rename(metaDir+"/rev", metaDir+"/prevRev"); err != nil {
		return false, err
	}

	if err = os.Rename(metaDir+"/hash", metaDir+"/prevHash"); err != nil {
		return false, err
	}

	// write out new 'rev' and 'hash'
	if err = os.WriteFile(metaDir+"/rev", []byte(rev), 0644); err != nil {
		return false, err
	}

	if err = os.WriteFile(metaDir+"/hash", []byte(hash), 0644); err != nil {
		return false, err
	}

	return true, nil
}
