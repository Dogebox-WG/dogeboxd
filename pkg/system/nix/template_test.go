package nix

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func renderPupTemplate(t *testing.T, values dogeboxd.NixPupContainerTemplateValues) string {
	t.Helper()

	tmpl, err := template.New("pup_container.nix").Funcs(tmplFuncs).Parse(string(rawPupContainerTemplate))
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, values); err != nil {
		t.Fatalf("failed to render template: %v", err)
	}

	return out.String()
}

func TestPupContainerTemplateRendersLegacyBuild(t *testing.T) {
	rendered := renderPupTemplate(t, dogeboxd.NixPupContainerTemplateValues{
		PUP_ID:       "legacy",
		PUP_ENABLED:  true,
		INTERNAL_IP:  "10.69.0.2",
		STORAGE_PATH: "/opt/dogebox/pups/storage/legacy",
		PUP_PATH:     "/opt/dogebox/pups/legacy",
		NIX_FILE:     "/opt/dogebox/pups/legacy/pup.nix",
		SERVICES: []dogeboxd.NixPupContainerServiceValues{
			{
				NAME: "legacy-pup",
				EXEC: "/bin/run.sh",
			},
		},
	})

	if !strings.Contains(rendered, `import /opt/dogebox/pups/legacy/pup.nix { inherit pkgs; };`) {
		t.Fatalf("expected legacy import in rendered template, got:\n%s", rendered)
	}
}

func TestPupContainerTemplateRendersFlakeBuild(t *testing.T) {
	rendered := renderPupTemplate(t, dogeboxd.NixPupContainerTemplateValues{
		PUP_ID:         "flake",
		PUP_ENABLED:    true,
		INTERNAL_IP:    "10.69.0.3",
		STORAGE_PATH:   "/opt/dogebox/pups/storage/flake",
		PUP_PATH:       "/opt/dogebox/pups/flake",
		FLAKE_REF:      "path:/opt/dogebox/pups/flake",
		FLAKE_PACKAGE:  "test-pup-flake",
		IS_FLAKE_BUILD: true,
		SERVICES: []dogeboxd.NixPupContainerServiceValues{
			{
				NAME: "test-pup-flake",
				EXEC: "/bin/run.sh",
			},
		},
	})

	if !strings.Contains(rendered, `builtins.getFlake "path:/opt/dogebox/pups/flake"`) {
		t.Fatalf("expected flake reference in rendered template, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `builtins.getAttr "test-pup-flake"`) {
		t.Fatalf("expected flake package lookup in rendered template, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `builtins.listToAttrs`) {
		t.Fatalf("expected flake service attr mapping in rendered template, got:\n%s", rendered)
	}
}
