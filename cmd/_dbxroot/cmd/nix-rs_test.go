package cmd

import "testing"

func TestBuildSystemdRunRSArgsDoesNotPipeThroughCaller(t *testing.T) {
	args := buildSystemdRunRSArgs("dogebox-system-update-v0-9-0-rc-8", "/tmp/os-upgrade", "v0.9.0-rc.8", true)

	for _, arg := range args {
		if arg == "--pipe" {
			t.Fatal("expected systemd-run args not to include --pipe")
		}
	}
}

func TestBuildSystemdRunRSArgsIncludesRebuildCommand(t *testing.T) {
	args := buildSystemdRunRSArgs("dogebox-system-update-v0-9-0-rc-8", "/tmp/os-upgrade", "v0.9.0-rc.8", true)
	expected := []string{
		"--unit", "dogebox-system-update-v0-9-0-rc-8",
		"--collect",
		"--wait",
		"--setenv=PATH=/run/current-system/sw/bin:/run/wrappers/bin",
		"/run/wrappers/bin/_dbxroot",
		"nix",
		"rs",
		"--flake-dir",
		"/tmp/os-upgrade",
		"--cleanup-flake-dir",
		"--set-release",
		"v0.9.0-rc.8",
	}

	if len(args) != len(expected) {
		t.Fatalf("expected args %v, got %v", expected, args)
	}
	for i, expectedArg := range expected {
		if args[i] != expectedArg {
			t.Fatalf("expected arg %d to be %q, got %q", i, expectedArg, args[i])
		}
	}
}
