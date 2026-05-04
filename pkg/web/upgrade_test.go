package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCheckForUpdatesReturnsSyntheticUpdateForOSRef(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(tempDir+"/dbx", []byte("v1.1.0"), 0644); err != nil {
		t.Fatalf("failed to write test version file: %v", err)
	}

	originalVersionPath := os.Getenv("VERSION_PATH_OVERRIDE")
	defer func() {
		if originalVersionPath == "" {
			_ = os.Unsetenv("VERSION_PATH_OVERRIDE")
			return
		}
		_ = os.Setenv("VERSION_PATH_OVERRIDE", originalVersionPath)
	}()
	if err := os.Setenv("VERSION_PATH_OVERRIDE", tempDir); err != nil {
		t.Fatalf("failed to set VERSION_PATH_OVERRIDE: %v", err)
	}

	req := httptest.NewRequest("GET", "/system/updates?osRef=669d109203eec7d2e4fb02044a324014cf5183bb", nil)
	rec := httptest.NewRecorder()

	api{}.checkForUpdates(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response UpdatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	pkg, ok := response.Packages["dogebox"]
	if !ok {
		t.Fatal("expected synthetic dogebox package update")
	}

	if pkg.CurrentVersion != "v1.1.0" {
		t.Fatalf("expected current version %q, got %q", "v1.1.0", pkg.CurrentVersion)
	}

	expectedVersion := "v1.1.0-osref.669d109203ee"
	if pkg.LatestUpdate != expectedVersion {
		t.Fatalf("expected latest update %q, got %q", expectedVersion, pkg.LatestUpdate)
	}

	if len(pkg.Updates) != 1 {
		t.Fatalf("expected 1 synthetic update, got %d", len(pkg.Updates))
	}

	if pkg.Updates[0].Version != expectedVersion {
		t.Fatalf("expected synthetic update version %q, got %q", expectedVersion, pkg.Updates[0].Version)
	}
}
