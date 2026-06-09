package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func writeManifest(t *testing.T, dir string, manifest dogeboxd.PupManifest) {
	t.Helper()

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
}

func TestManifestSourceDiskListAcceptsLegacyPupAndAddsWarning(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, dogeboxd.PupManifest{
		ManifestVersion: 1,
		Meta: dogeboxd.PupManifestMeta{
			Name:    "Legacy Pup",
			Version: "1.0.0",
		},
		Config: dogeboxd.PupManifestConfigFields{},
		Container: dogeboxd.PupManifestContainer{
			Build: dogeboxd.PupManifestBuild{
				NixFile:       "pup.nix",
				NixFileSha256: "abc123",
			},
			Services: []dogeboxd.PupManifestService{
				{
					Name: "legacy-pup",
					Command: dogeboxd.PupManifestCommand{
						Exec: "/bin/run.sh",
					},
				},
			},
		},
	})

	if err := os.WriteFile(filepath.Join(dir, "pup.nix"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("failed to write pup.nix: %v", err)
	}

	list, err := ManifestSourceDisk{
		config: dogeboxd.ManifestSourceConfiguration{Location: dir},
	}.List(false)
	if err != nil {
		t.Fatalf("expected legacy source to list successfully, got %v", err)
	}

	if len(list.Pups) != 1 {
		t.Fatalf("expected one legacy pup, got %d", len(list.Pups))
	}

	if len(list.Pups[0].Warnings) != 1 || list.Pups[0].Warnings[0].Code != dogeboxd.PUP_WARNING_LEGACY_BUILD_DEPRECATED {
		t.Fatalf("expected legacy warning, got %#v", list.Pups[0].Warnings)
	}
}

func TestValidatePupFilesRequiresDeclaredLogo(t *testing.T) {
	dir := t.TempDir()
	manifest := dogeboxd.PupManifest{
		ManifestVersion: 1,
		Meta: dogeboxd.PupManifestMeta{
			Name:     "Logo Pup",
			Version:  "1.0.0",
			LogoPath: "logo.png",
		},
		Config: dogeboxd.PupManifestConfigFields{},
		Container: dogeboxd.PupManifestContainer{
			Build: dogeboxd.PupManifestBuild{
				NixFile:       "pup.nix",
				NixFileSha256: "abc123",
			},
			Services: []dogeboxd.PupManifestService{
				{
					Name: "logo-pup",
					Command: dogeboxd.PupManifestCommand{
						Exec: "/bin/run.sh",
					},
				},
			},
		},
	}

	writeManifest(t, dir, manifest)
	if err := os.WriteFile(filepath.Join(dir, "pup.nix"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("failed to write pup.nix: %v", err)
	}

	if err := validatePupFiles(dir, manifest); err == nil {
		t.Fatal("expected missing declared logo to fail validation")
	}
}

func TestManifestSourceDiskListAcceptsFlakePup(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, dogeboxd.PupManifest{
		ManifestVersion: 1,
		Meta: dogeboxd.PupManifestMeta{
			Name:    "Flake Pup",
			Version: "1.0.0",
		},
		Config: dogeboxd.PupManifestConfigFields{},
		Container: dogeboxd.PupManifestContainer{
			Build: dogeboxd.PupManifestBuild{
				Type: string(dogeboxd.PupManifestBuildTypeFlake),
				Flake: dogeboxd.PupManifestBuildFlake{
					Path:    ".",
					Package: "test-pup-flake",
				},
			},
			Services: []dogeboxd.PupManifestService{
				{
					Name: "test-pup-flake",
					Command: dogeboxd.PupManifestCommand{
						Exec: "/bin/run.sh",
					},
				},
			},
		},
	})

	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), []byte("{ }\n"), 0644); err != nil {
		t.Fatalf("failed to write flake.nix: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flake.lock"), []byte("{ }\n"), 0644); err != nil {
		t.Fatalf("failed to write flake.lock: %v", err)
	}

	list, err := ManifestSourceDisk{
		config: dogeboxd.ManifestSourceConfiguration{Location: dir},
	}.List(false)
	if err != nil {
		t.Fatalf("expected flake source to list successfully, got %v", err)
	}

	if len(list.Pups) != 1 {
		t.Fatalf("expected one flake pup, got %d", len(list.Pups))
	}

	if len(list.Pups[0].Warnings) != 0 {
		t.Fatalf("expected flake pup to have no warnings, got %#v", list.Pups[0].Warnings)
	}
}
