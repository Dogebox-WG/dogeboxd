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

	return semver.Compare(current, cutover) < 0, nil
}

func validateSemverVersion(version string, label string) error {
	if !semver.IsValid(version) {
		return fmt.Errorf("invalid %s version %q", label, version)
	}

	return nil
}
