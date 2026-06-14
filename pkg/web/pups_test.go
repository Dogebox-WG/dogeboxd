package web

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestPickDependencySourceSkipsCurrentProvider(t *testing.T) {
	dep := dogeboxd.PupDependencyReport{
		Interface:       "core-rpc",
		CurrentProvider: "existing-provider",
		InstallableProviders: []dogeboxd.PupManifestDependencySource{
			{SourceLocation: "https://example.com/source-a.git", PupName: "Dogecoin Core Remote", PupVersion: "2.0.0"},
		},
	}

	_, ok := pickDependencySource(dep)
	if ok {
		t.Fatal("expected no source to be selected when a current provider is already set")
	}
}

func TestPickDependencySourceSkipsInstalledProvider(t *testing.T) {
	dep := dogeboxd.PupDependencyReport{
		Interface:          "core-zmq",
		InstalledProviders: []string{"dogecoin-core"},
		InstallableProviders: []dogeboxd.PupManifestDependencySource{
			{SourceLocation: "https://example.com/source-a.git", PupName: "Dogecoin Core Remote", PupVersion: "2.0.0"},
		},
	}

	_, ok := pickDependencySource(dep)
	if ok {
		t.Fatal("expected no source to be selected when a compatible provider is already installed")
	}
}

func TestBuildDependencyInstallRequestsPrefersDefaultSource(t *testing.T) {
	sourceLists := map[string]dogeboxd.ManifestSourceList{
		"dogecoin-core": {
			Config: dogeboxd.ManifestSourceConfiguration{
				ID:       "dogecoin-core",
				Location: "https://example.com/source-a.git",
			},
		},
		"dogecoin-remote": {
			Config: dogeboxd.ManifestSourceConfiguration{
				ID:       "dogecoin-remote",
				Location: "https://example.com/source-b.git",
			},
		},
	}

	deps := []dogeboxd.PupDependencyReport{
		{
			Interface: "core-rpc",
			InstallableProviders: []dogeboxd.PupManifestDependencySource{
				{SourceLocation: "https://example.com/source-b.git", PupName: "Dogecoin Core Remote", PupVersion: "2.0.0"},
				{SourceLocation: "https://example.com/source-a.git", PupName: "Dogecoin Core", PupVersion: "1.14.7"},
			},
			DefaultSourceProvider: dogeboxd.PupManifestDependencySource{
				SourceLocation: "https://example.com/source-a.git",
				PupName:        "Dogecoin Core",
				PupVersion:     "1.14.7",
			},
		},
	}

	installs, err := buildDependencyInstallRequests(deps, sourceLists, "session-token", map[string]struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("expected one dependency install, got %d", len(installs))
	}

	install := installs[0]
	if install.SourceId != "dogecoin-core" {
		t.Fatalf("expected dependency source ID to come from the selected source, got %q", install.SourceId)
	}
	if install.PupName != "Dogecoin Core" || install.PupVersion != "1.14.7" {
		t.Fatalf("expected default source to be selected, got %s %s", install.PupName, install.PupVersion)
	}
}

func TestBuildDependencyInstallRequestsFallsBackToFirstInstallableSource(t *testing.T) {
	sourceLists := map[string]dogeboxd.ManifestSourceList{
		"remote-source": {
			Config: dogeboxd.ManifestSourceConfiguration{
				ID:       "remote-source",
				Location: "https://example.com/source-b.git",
			},
		},
	}

	deps := []dogeboxd.PupDependencyReport{
		{
			Interface: "core-rpc",
			InstallableProviders: []dogeboxd.PupManifestDependencySource{
				{SourceLocation: "https://example.com/source-b.git", PupName: "Dogecoin Core Remote", PupVersion: "2.0.0"},
				{SourceLocation: "https://example.com/source-b.git", PupName: "Dogecoin Core Remote", PupVersion: "1.9.0"},
			},
			DefaultSourceProvider: dogeboxd.PupManifestDependencySource{
				SourceLocation: "https://example.com/source-a.git",
				PupName:        "Dogecoin Core",
				PupVersion:     "1.14.7",
			},
		},
	}

	installs, err := buildDependencyInstallRequests(deps, sourceLists, "session-token", map[string]struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("expected one dependency install, got %d", len(installs))
	}

	install := installs[0]
	if install.PupName != "Dogecoin Core Remote" || install.PupVersion != "2.0.0" {
		t.Fatalf("expected the first installable source to be selected, got %s %s", install.PupName, install.PupVersion)
	}
}

func TestBuildDependencyInstallRequestsDeduplicatesSources(t *testing.T) {
	sourceLists := map[string]dogeboxd.ManifestSourceList{
		"shared-source": {
			Config: dogeboxd.ManifestSourceConfiguration{
				ID:       "shared-source",
				Location: "https://example.com/source-a.git",
			},
		},
	}

	source := dogeboxd.PupManifestDependencySource{
		SourceLocation: "https://example.com/source-a.git",
		PupName:        "Dogecoin Core",
		PupVersion:     "1.14.7",
	}

	deps := []dogeboxd.PupDependencyReport{
		{
			Interface:            "core-rpc",
			InstallableProviders: []dogeboxd.PupManifestDependencySource{source},
		},
		{
			Interface:            "core-zmq",
			InstallableProviders: []dogeboxd.PupManifestDependencySource{source},
		},
	}

	installs, err := buildDependencyInstallRequests(deps, sourceLists, "session-token", map[string]struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("expected duplicate source installs to be collapsed, got %d installs", len(installs))
	}
	if installs[0].SessionToken != "session-token" {
		t.Fatalf("expected session token to be preserved, got %q", installs[0].SessionToken)
	}
}
