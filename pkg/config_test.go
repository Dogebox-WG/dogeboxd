package dogeboxd

import "testing"

func TestGetSystemEnvironmentVariablesForContainerUsesInternalPort(t *testing.T) {
	env := GetSystemEnvironmentVariablesForContainer(8082)

	if env["DBX_HOST"] != "10.69.0.1" {
		t.Fatalf("expected DBX_HOST 10.69.0.1, got %q", env["DBX_HOST"])
	}
	if env["DBX_PORT"] != "8082" {
		t.Fatalf("expected DBX_PORT 8082, got %q", env["DBX_PORT"])
	}
}
