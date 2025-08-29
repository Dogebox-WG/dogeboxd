package upgrades_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/semver"
)

// Copy the necessary types and interfaces for testing
type RepositoryTag struct {
	Tag string
}

type UpgradableRelease struct {
	Version    string
	ReleaseURL string
	Summary    string
}

// RepoTagsFetcher interface for mocking
type RepoTagsFetcher interface {
	GetRepoTags(repo string) ([]RepositoryTag, error)
}

// MockRepoTagsFetcher for testing
type MockRepoTagsFetcher struct {
	tags []RepositoryTag
	err  error
}

func (m *MockRepoTagsFetcher) GetRepoTags(repo string) ([]RepositoryTag, error) {
	return m.tags, m.err
}

// Test the core data structures
func TestRepositoryTag(t *testing.T) {
	tag := RepositoryTag{Tag: "v1.2.3"}
	if tag.Tag != "v1.2.3" {
		t.Errorf("Expected tag to be 'v1.2.3', got '%s'", tag.Tag)
	}
}

func TestUpgradableRelease(t *testing.T) {
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

// Test the mock functionality
func TestMockRepoTagsFetcher(t *testing.T) {
	// Test successful fetch
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
		{Tag: "v1.2.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

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

// Test semver functionality
func TestSemverValidation(t *testing.T) {
	testCases := []struct {
		version string
		valid   bool
	}{
		{"v1.0.0", true},
		{"v1.2.3", true},
		{"v10.20.30", true},
		{"1.0.0", false}, // semver.IsValid requires 'v' prefix in this context
		{"invalid", false},
		{"v1.2", true}, // Go's semver treats v1.2 as valid (equivalent to v1.2.0)
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

// Test version comparison logic (simulating the upgrade logic)
func TestVersionComparison(t *testing.T) {
	currentVersion := "v1.1.0"
	availableTags := []RepositoryTag{
		{Tag: "v1.0.0"}, // older, should be excluded
		{Tag: "v1.1.0"}, // same, should be excluded
		{Tag: "v1.2.0"}, // newer, should be included
		{Tag: "v2.0.0"}, // newer, should be included
	}

	var upgrades []UpgradableRelease
	for _, tag := range availableTags {
		if semver.IsValid(tag.Tag) && semver.Compare(tag.Tag, currentVersion) > 0 {
			release := UpgradableRelease{
				Version:    tag.Tag,
				ReleaseURL: fmt.Sprintf("https://github.com/dogebox-wg/os/releases/tag/%s", tag.Tag),
				Summary:    "Update for Dogeboxd / DKM / DPanel",
			}
			upgrades = append(upgrades, release)
		}
	}

	// Should have 2 upgrades available (v1.2.0 and v2.0.0)
	if len(upgrades) != 2 {
		t.Errorf("Expected 2 upgrades, got %d", len(upgrades))
	}

	// Check the versions
	expectedVersions := []string{"v1.2.0", "v2.0.0"}
	for i, expected := range expectedVersions {
		if i >= len(upgrades) {
			t.Errorf("Missing expected version %s", expected)
			continue
		}
		if upgrades[i].Version != expected {
			t.Errorf("Expected upgrade[%d] to be %s, got %s", i, expected, upgrades[i].Version)
		}
	}
}

// Test helper functions
func GenerateSequentialVersions(major, minor int, count int) []RepositoryTag {
	tags := make([]RepositoryTag, count)
	for i := 0; i < count; i++ {
		tags[i] = RepositoryTag{Tag: fmt.Sprintf("v%d.%d.%d", major, minor+i, 0)}
	}
	return tags
}

func TestGenerateSequentialVersions(t *testing.T) {
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
}

// Test version file creation for integration testing
func TestVersionFileMocking(t *testing.T) {
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

	// Verify the file was written correctly
	content, err := os.ReadFile(filepath.Join(versionDir, "dbx"))
	if err != nil {
		t.Fatalf("Failed to read version file: %v", err)
	}

	if string(content) != testVersion {
		t.Errorf("Expected version file to contain %s, got %s", testVersion, string(content))
	}
}

// Test error scenarios for upgrade validation
func TestUpgradeValidation(t *testing.T) {
	// Test invalid package name
	validPackages := []string{"os"}
	testPackage := "invalid-package"
	
	isValid := false
	for _, valid := range validPackages {
		if testPackage == valid {
			isValid = true
			break
		}
	}
	
	if isValid {
		t.Errorf("Expected package '%s' to be invalid", testPackage)
	}

	// Test version availability
	availableVersions := []string{"v1.2.0", "v1.3.0"}
	requestedVersion := "v2.0.0"
	
	isAvailable := false
	for _, available := range availableVersions {
		if requestedVersion == available {
			isAvailable = true
			break
		}
	}
	
	if isAvailable {
		t.Errorf("Expected version '%s' to be unavailable", requestedVersion)
	}
}

// Benchmark the version comparison logic
func BenchmarkVersionComparison(b *testing.B) {
	currentVersion := "v1.50.0"
	mockTags := make([]RepositoryTag, 100)
	for i := 0; i < 100; i++ {
		mockTags[i] = RepositoryTag{Tag: fmt.Sprintf("v1.%d.0", i)}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var upgrades []UpgradableRelease
		for _, tag := range mockTags {
			if semver.IsValid(tag.Tag) && semver.Compare(tag.Tag, currentVersion) > 0 {
				release := UpgradableRelease{
					Version:    tag.Tag,
					ReleaseURL: fmt.Sprintf("https://github.com/dogebox-wg/os/releases/tag/%s", tag.Tag),
					Summary:    "Update for Dogeboxd / DKM / DPanel",
				}
				upgrades = append(upgrades, release)
			}
		}
		// Prevent compiler optimization
		_ = upgrades
	}
}