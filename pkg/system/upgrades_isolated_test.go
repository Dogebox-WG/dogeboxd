// +build test

package system

// This file contains isolated tests for upgrades functionality
// Use build tag "test" to run these tests independently
// Run with: go test -tags=test ./pkg/system -run TestUpgradesIsolated -v

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/semver"
)

func TestUpgradesIsolatedStructs(t *testing.T) {
	// Test RepositoryTag struct
	tag := RepositoryTag{Tag: "v1.2.3"}
	if tag.Tag != "v1.2.3" {
		t.Errorf("Expected tag to be 'v1.2.3', got '%s'", tag.Tag)
	}

	// Test UpgradableRelease struct
	release := UpgradableRelease{
		Version:    "v1.2.3",
		ReleaseURL: "https://github.com/dogebox-wg/os/releases/tag/v1.2.3",
		Summary:    "Test release",
	}

	if release.Version != "v1.2.3" {
		t.Errorf("Expected version to be 'v1.2.3', got '%s'", release.Version)
	}
	if release.ReleaseURL != "https://github.com/dogebox-wg/os/releases/tag/v1.2.3" {
		t.Errorf("Expected ReleaseURL to contain v1.2.3, got '%s'", release.ReleaseURL)
	}
	if release.Summary != "Test release" {
		t.Errorf("Expected summary to be 'Test release', got '%s'", release.Summary)
	}
}

func TestUpgradesIsolatedMockFetcher(t *testing.T) {
	// Test MockRepoTagsFetcher
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
		{Tag: "v1.2.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Test successful fetch
	result, err := mockFetcher.GetRepoTags("test-repo")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(result))
	}
	if result[0].Tag != "v1.0.0" {
		t.Errorf("Expected first tag to be 'v1.0.0', got '%s'", result[0].Tag)
	}

	// Test error case
	errorFetcher := &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("test error"),
	}
	result, err = errorFetcher.GetRepoTags("test-repo")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 tags on error, got %d", len(result))
	}
}

func TestUpgradesIsolatedVersionMocking(t *testing.T) {
	// Create temporary version directory
	tempDir, err := os.MkdirTemp("", "version_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	versionDir := filepath.Join(tempDir, "versioning")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create versioning directory: %v", err)
	}

	testVersion := "v1.0.0"
	if err := os.WriteFile(filepath.Join(versionDir, "dbx"), []byte(testVersion), 0644); err != nil {
		t.Fatalf("Failed to write version file: %v", err)
	}

	// Test version override
	originalPath := os.Getenv("VERSION_PATH_OVERRIDE")
	os.Setenv("VERSION_PATH_OVERRIDE", versionDir)
	defer func() {
		if originalPath == "" {
			os.Unsetenv("VERSION_PATH_OVERRIDE")
		} else {
			os.Setenv("VERSION_PATH_OVERRIDE", originalPath)
		}
	}()

	// Test semver validation with various version formats
	testCases := []struct {
		version string
		valid   bool
	}{
		{"v1.0.0", true},
		{"v1.2.3", true},
		{"v10.20.30", true},
		{"1.0.0", false}, // semver.IsValid requires 'v' prefix in this context
		{"invalid", false},
		{"v1.2", false},
		{"v1.2.3.4", false},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			isValid := semver.IsValid(tc.version)
			if isValid != tc.valid {
				t.Errorf("For version %s, expected valid=%v, got valid=%v", tc.version, tc.valid, isValid)
			}
		})
	}
}

func TestUpgradesIsolatedHelpers(t *testing.T) {
	// Test GenerateSequentialVersions
	tags := GenerateSequentialVersions(1, 0, 3)
	expectedTags := []string{"v1.0.0", "v1.1.0", "v1.2.0"}
	
	if len(tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}
	
	for i, expectedTag := range expectedTags {
		if tags[i].Tag != expectedTag {
			t.Errorf("Expected tag[%d] to be %s, got %s", i, expectedTag, tags[i].Tag)
		}
	}

	// Test CreateSuccessMock
	testTags := []RepositoryTag{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}
	successMock := CreateSuccessMock(testTags)
	
	result, err := successMock.GetRepoTags("test-repo")
	if err != nil {
		t.Errorf("Expected no error from success mock, got: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(result))
	}

	// Test CreateErrorMock
	testErr := fmt.Errorf("test error")
	errorMock := CreateErrorMock(testErr)
	
	result, err = errorMock.GetRepoTags("test-repo")
	if err == nil {
		t.Error("Expected error from error mock, got nil")
	}
	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got '%s'", err.Error())
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 tags on error, got %d", len(result))
	}
}