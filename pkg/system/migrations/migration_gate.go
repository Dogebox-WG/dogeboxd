package migrations

import (
	"fmt"

	"golang.org/x/mod/semver"
)

func semverCheck(current string, cutover string) (bool, error) {
	if err := validateSemverVersion(current, "current"); err != nil {
		return false, err
	}
	if err := validateSemverVersion(cutover, "cutover"); err != nil {
		return false, err
	}

	// Treat prereleases for the same cutover release as already at the cutover.
	// Example: v0.9.0-rc.4 should not count as older than v0.9.0 here.
	if semver.Prerelease(cutover) == "" && semver.Prerelease(current) != "" && baseVersion(current) == cutover {
		return false, nil
	}

	return semver.Compare(current, cutover) < 0, nil
}

func validateSemverVersion(version string, label string) error {
	if !semver.IsValid(version) {
		return fmt.Errorf("invalid %s version %q", label, version)
	}

	return nil
}

func baseVersion(version string) string {
	if prerelease := semver.Prerelease(version); prerelease != "" {
		return version[:len(version)-len(prerelease)]
	}

	return version
}
