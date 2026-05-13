package migrations

import "testing"

func TestSemverCheck(t *testing.T) {
	testCases := []struct {
		name     string
		current  string
		cutover  string
		expected bool
	}{
		{name: "older than cutover", current: "v0.8.1", cutover: "v0.9.0", expected: true},
		{name: "equal stable release", current: "v0.9.0", cutover: "v0.9.0", expected: false},
		{name: "same release rc before stable", current: "v0.9.0-rc.4", cutover: "v0.9.0", expected: true},
		{name: "earlier rc before later rc", current: "v0.9.0-rc.3", cutover: "v0.9.0-rc.4", expected: true},
		{name: "later rc after earlier rc", current: "v0.9.0-rc.5", cutover: "v0.9.0-rc.4", expected: false},
		{name: "future release", current: "v0.10.0", cutover: "v0.9.0", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := semverCheck(tc.current, tc.cutover)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestSemverCheckRejectsInvalidVersions(t *testing.T) {
	if _, err := semverCheck("0.9.0", "v0.9.0"); err == nil {
		t.Fatal("expected invalid current version error")
	}

	if _, err := semverCheck("v0.9.0", "0.9.0"); err == nil {
		t.Fatal("expected invalid cutover version error")
	}
}
