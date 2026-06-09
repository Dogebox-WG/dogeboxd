package dogeboxd

import "testing"

func TestPupManifestBuildValidateLegacy(t *testing.T) {
	build := PupManifestBuild{
		NixFile:       "pup.nix",
		NixFileSha256: "abc123",
	}

	if err := build.Validate(); err != nil {
		t.Fatalf("expected legacy build to validate, got %v", err)
	}

	if !build.IsLegacy() {
		t.Fatal("expected legacy build to be marked as legacy")
	}

	requiredFiles := build.RequiredFiles()
	if len(requiredFiles) != 1 || requiredFiles[0] != "pup.nix" {
		t.Fatalf("unexpected legacy required files: %#v", requiredFiles)
	}

	warnings := BuildSupportWarnings(build)
	if len(warnings) != 1 {
		t.Fatalf("expected one legacy warning, got %#v", warnings)
	}
	if warnings[0].Code != PUP_WARNING_LEGACY_BUILD_DEPRECATED {
		t.Fatalf("unexpected warning code: %#v", warnings[0])
	}
}

func TestPupManifestBuildValidateFlake(t *testing.T) {
	build := PupManifestBuild{
		Type: string(PupManifestBuildTypeFlake),
		Flake: PupManifestBuildFlake{
			Package: "test-pup-flake",
		},
	}

	if err := build.Validate(); err != nil {
		t.Fatalf("expected flake build to validate, got %v", err)
	}

	if build.IsLegacy() {
		t.Fatal("expected flake build to not be marked as legacy")
	}

	requiredFiles := build.RequiredFiles()
	if len(requiredFiles) != 2 || requiredFiles[0] != "flake.nix" || requiredFiles[1] != "flake.lock" {
		t.Fatalf("unexpected flake required files: %#v", requiredFiles)
	}

	if warnings := BuildSupportWarnings(build); len(warnings) != 0 {
		t.Fatalf("expected no warnings for flake build, got %#v", warnings)
	}
}

func TestPupManifestBuildValidateFlakeRequiresPackage(t *testing.T) {
	build := PupManifestBuild{
		Type:  string(PupManifestBuildTypeFlake),
		Flake: PupManifestBuildFlake{},
	}

	if err := build.Validate(); err == nil {
		t.Fatal("expected flake build without package to fail validation")
	}
}
