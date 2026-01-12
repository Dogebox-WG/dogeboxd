package dogeboxd

import (
	"reflect"
	"testing"
)

func TestExtractManifestConfigDefaults(t *testing.T) {
	manifest := PupManifestConfigFields{
		Sections: []PupManifestConfigSection{
			{
				Name: "general",
				Fields: []PupManifestConfigField{
					{Name: "TEXT", Type: "text", Default: "wow"},
					{Name: "NUMBER", Type: "number", Default: 42},
					{Name: "FLAG", Type: "toggle", Default: true},
				},
			},
		},
	}

	defaults, err := ExtractManifestConfigDefaults(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"TEXT":   "wow",
		"NUMBER": "42",
		"FLAG":   "true",
	}

	if !reflect.DeepEqual(expected, defaults) {
		t.Fatalf("expected %v, got %v", expected, defaults)
	}
}

func TestManifestConfigNeedsValues(t *testing.T) {
	manifest := PupManifestConfigFields{
		Sections: []PupManifestConfigSection{
			{
				Name: "rpc",
				Fields: []PupManifestConfigField{
					{Name: "REQUIRED", Type: "text", Required: true},
					{Name: "OPTIONAL", Type: "text"},
				},
			},
		},
	}

	if !ManifestConfigNeedsValues(manifest, map[string]string{}) {
		t.Fatalf("expected missing required field to report true")
	}

	config := map[string]string{
		"REQUIRED": "set",
	}

	if ManifestConfigNeedsValues(manifest, config) {
		t.Fatalf("expected filled required field to report false")
	}
}

func TestCoerceConfigPayload(t *testing.T) {
	manifest := PupManifestConfigFields{
		Sections: []PupManifestConfigSection{
			{
				Name: "rpc",
				Fields: []PupManifestConfigField{
					{Name: "RPC_ENABLED", Type: "toggle"},
					{Name: "RPC_PORT", Type: "number"},
					{Name: "RPC_USER", Type: "text"},
				},
			},
		},
	}

	payload := map[string]any{
		"RPC_ENABLED": "true",
		"RPC_PORT":    "22555",
		"RPC_USER":    "dogebox",
		"IGNORED":     "value",
	}

	result, err := CoerceConfigPayload(manifest, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"RPC_ENABLED": "true",
		"RPC_PORT":    "22555",
		"RPC_USER":    "dogebox",
	}

	if !reflect.DeepEqual(expected, result) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
}
