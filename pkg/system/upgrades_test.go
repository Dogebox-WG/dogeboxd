package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/semver"
)

// MockRepoTagsFetcher implements RepoTagsFetcher for testing
type MockRepoTagsFetcher struct {
	tags []RepositoryTag
	err  error
}

func (m *MockRepoTagsFetcher) GetRepoTags(repo string) ([]RepositoryTag, error) {
	return m.tags, m.err
}

func TestRepositoryTag_Struct(t *testing.T) {
	tag := RepositoryTag{Tag: "v1.2.3"}
	if tag.Tag != "v1.2.3" {
		t.Errorf("Expected tag to be 'v1.2.3', got '%s'", tag.Tag)
	}
}

func TestUpgradableRelease_Struct(t *testing.T) {
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

func TestGetUpgradableReleases_WithMockFetcher(t *testing.T) {
	// Create a mock fetcher with test data
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
		{Tag: "v1.2.0"},
		{Tag: "v2.0.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Create temporary version file to simulate current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should only return versions newer than v1.1.0 (v1.2.0 and v2.0.0)
	expectedCount := 2
	if len(releases) != expectedCount {
		t.Errorf("Expected %d upgradable releases, got %d", expectedCount, len(releases))
	}

	// Check first upgrade (v1.2.0)
	if releases[0].Version != "v1.2.0" {
		t.Errorf("Expected first upgrade to be v1.2.0, got %s", releases[0].Version)
	}

	// Check second upgrade (v2.0.0)
	if releases[1].Version != "v2.0.0" {
		t.Errorf("Expected second upgrade to be v2.0.0, got %s", releases[1].Version)
	}

	// Verify release URLs are correctly formatted
	expectedURL1 := "https://github.com/dogebox-wg/os/releases/tag/v1.2.0"
	if releases[0].ReleaseURL != expectedURL1 {
		t.Errorf("Expected release URL to be %s, got %s", expectedURL1, releases[0].ReleaseURL)
	}

	// Verify all releases have the expected summary
	for _, release := range releases {
		expectedSummary := "Update for Dogeboxd / DKM / DPanel"
		if release.Summary != expectedSummary {
			t.Errorf("Expected summary to be '%s', got '%s'", expectedSummary, release.Summary)
		}
	}
}

func TestGetUpgradableReleases_WithErrorFromMock(t *testing.T) {
	// Test with error from mock
	mockFetcher := &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("mock error: failed to fetch tags"),
	}

	releases, err := GetUpgradableReleasesWithFetcher(mockFetcher)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if len(releases) != 0 {
		t.Errorf("Expected empty releases slice on error, got %d releases", len(releases))
	}

	expectedErrorMsg := "mock error: failed to fetch tags"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestGetUpgradableReleases_NoUpgrades(t *testing.T) {
	// Test when current version is newer than all available versions
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version to be newer than available tags
	tempDir := setupMockVersioning(t, "v2.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("Expected no upgradable releases, got %d", len(releases))
	}
}

func TestGetUpgradableReleases_EmptyTags(t *testing.T) {
	// Test with empty tags
	mockFetcher := &MockRepoTagsFetcher{tags: []RepositoryTag{}, err: nil}

	tempDir := setupMockVersioning(t, "v1.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("Expected no releases with empty tags, got %d", len(releases))
	}
}

func TestDoSystemUpdate_InvalidPackage(t *testing.T) {
	// Mock successful tag retrieval
	mockTags := []RepositoryTag{
		{Tag: "v1.2.0"},
	}
	
	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()
	
	repoTagsFetcher = &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	err := DoSystemUpdate("invalid-package", "v1.2.0")
	if err == nil {
		t.Fatal("Expected error for invalid package, got nil")
	}

	expectedError := "Invalid package to upgrade: invalid-package"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestDoSystemUpdate_UnavailableVersion(t *testing.T) {
	// Mock tag retrieval with specific versions
	mockTags := []RepositoryTag{
		{Tag: "v1.2.0"},
		{Tag: "v1.3.0"},
	}
	
	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()
	
	repoTagsFetcher = &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	// Try to upgrade to a version that doesn't exist in upgradable releases
	err := DoSystemUpdate("os", "v2.0.0")
	if err == nil {
		t.Fatal("Expected error for unavailable version, got nil")
	}

	expectedError := "Release v2.0.0 is not available for os"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestDoSystemUpdate_ErrorFromGetUpgradableReleases(t *testing.T) {
	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()
	
	// Mock error from repo fetcher
	repoTagsFetcher = &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("network error"),
	}

	err := DoSystemUpdate("os", "v1.2.0")
	if err == nil {
		t.Fatal("Expected error from GetUpgradableReleases, got nil")
	}

	if err.Error() != "network error" {
		t.Errorf("Expected 'network error', got '%s'", err.Error())
	}
}

// Test helper functions for semver validation
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

// Benchmark tests
func BenchmarkGetUpgradableReleases(b *testing.B) {
	// Setup mock data
	mockTags := make([]RepositoryTag, 100)
	for i := 0; i < 100; i++ {
		mockTags[i] = RepositoryTag{Tag: fmt.Sprintf("v1.%d.0", i)}
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version
	tempDir := setupMockVersioning(b, "v1.50.0")
	defer os.RemoveAll(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetUpgradableReleasesWithFetcher(mockFetcher)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

// setupMockVersioning creates a temporary version directory for testing
func setupMockVersioning(t testing.TB, release string) string {
	tempDir, err := os.MkdirTemp("", "version_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	versionDir := filepath.Join(tempDir, "versioning")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create versioning directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(versionDir, "dbx"), []byte(release), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to write version file: %v", err)
	}

	// Temporarily change the version lookup path to our temp directory
	originalPath := os.Getenv("VERSION_PATH_OVERRIDE")
	os.Setenv("VERSION_PATH_OVERRIDE", versionDir)
	
	// Restore original path when test completes
	t.Cleanup(func() {
		if originalPath == "" {
			os.Unsetenv("VERSION_PATH_OVERRIDE")
		} else {
			os.Setenv("VERSION_PATH_OVERRIDE", originalPath)
		}
	})

	return tempDir
}

// TestIntegrationSuite runs the full integration test suite
func TestIntegrationSuite(t *testing.T) {
	RunIntegrationTestSuite(t)
}

// TestTableDrivenUpgradeScenarios tests various upgrade scenarios using table-driven approach
func TestTableDrivenUpgradeScenarios(t *testing.T) {
	testCases := []TableDrivenTest{
		{
			Name:           "Single upgrade available",
			CurrentVersion: "v1.0.0",
			AvailableTags:  []RepositoryTag{{Tag: "v1.1.0"}},
			ExpectedCount:  1,
			ExpectedVersions: []string{"v1.1.0"},
		},
		{
			Name:           "Multiple pre-release and release versions",
			CurrentVersion: "v1.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v1.1.0-alpha"},  // Should be ignored (invalid semver without rc/beta/alpha handling)
				{Tag: "v1.1.0"},
				{Tag: "v1.2.0"},
			},
			ExpectedCount:    2,
			ExpectedVersions: []string{"v1.1.0", "v1.2.0"},
		},
		{
			Name:           "Empty repository",
			CurrentVersion: "v1.0.0",
			AvailableTags:  []RepositoryTag{},
			ExpectedCount:  0,
		},
	}

	RunTableDrivenTests(t, testCases)
}

// TestMockFetcherCreationHelpers tests the helper functions for creating mocks
func TestMockFetcherCreationHelpers(t *testing.T) {
	// Test CreateSuccessMock
	tags := []RepositoryTag{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}
	successMock := CreateSuccessMock(tags)
	
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

// TestGenerateSequentialVersions tests the version generation helper
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