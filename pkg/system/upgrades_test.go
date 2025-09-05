package system

import (
	"fmt"
	"os"
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

// TODO : this only tests the semver module
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
		{"v1.2", true}, // we're allowing 'vMAJOR.MINOR' (semver equates this to 'vMAJOR.MINOR.0')
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

// TODO : this just tests MockRepoTagsFetcher, is this or SuccessMock/Errormock from helpers even needed?
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

// TODO : This is a trivial function, so maybe this is doing too much?
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

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// normal case with upgrade versions available
func TestGetUpgradableReleases_UpgradesAvailable(t *testing.T) {
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

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// normal case with no upgrade versions available
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

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// no tags in repository.
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

// TODO : This appears to test an error condition by feeding an error /in/ to GetUpgradableReleasesWithFetcher
// and checks that the same error propogates?
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

// TestTableDrivenUpgradeScenarios tests various upgrade scenarios using table-driven approach
func TestIntegrationSuite(t *testing.T) {
	testCases := []TableDrivenTest{
		{
			Name:             "Single upgrade available",
			CurrentVersion:   "v1.0.0",
			AvailableTags:    []RepositoryTag{{Tag: "v1.1.0"}},
			ExpectedCount:    1,
			ExpectedVersions: []string{"v1.1.0"},
		},
		{
			Name:           "Multiple pre-release and release versions",
			CurrentVersion: "v1.0.0",
			AvailableTags: []RepositoryTag{
				// TODO : There's nothing to discard tags with invalid semver, do we want this?
				//{Tag: "v1.1.0-alpha"}, // Should be ignored (invalid semver without rc/beta/alpha handling)
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
		{
			Name:           "Multiple upgrades available",
			CurrentVersion: "v1.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v0.9.0"},
				{Tag: "v1.0.0"},
				{Tag: "v1.1.0"},
				{Tag: "v1.2.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount:    3,
			ExpectedVersions: []string{"v1.1.0", "v1.2.0", "v2.0.0"},
		},
		{
			Name:           "No upgrades available - current is latest",
			CurrentVersion: "v2.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v1.0.0"},
				{Tag: "v1.1.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount: 0,
		},
		{
			Name:           "No upgrades available - current is newer",
			CurrentVersion: "v3.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v1.0.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount: 0,
		},
		{
			Name:           "Network error",
			CurrentVersion: "v1.0.0",
			MockError:      ErrNetworkTimeout,
			ExpectedError:  "network timeout",
		},
		{
			Name:           "Repository not found",
			CurrentVersion: "v1.0.0",
			MockError:      ErrRepoNotFound,
			ExpectedError:  "repository not found",
		},
	}

	RunTableDrivenTests(t, testCases)
}

// TODO : Unable to test a working case of DoSystemUpdate

// specifying invalid package name
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
		t.Fatal("expected error for invalid package, got nil")
	}

	expectedError := "invalid package to upgrade: invalid-package"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// specifying non-existant version
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
		t.Fatal("expected error for unavailable version, got nil")
	}

	expectedError := "release v2.0.0 is not available for os"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// mocking error
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
		t.Fatal("expected error from GetUpgradableReleases, got nil")
	}

	if err.Error() != "network error" {
		t.Errorf("expected 'network error', got '%s'", err.Error())
	}
}

/* TODO : check if versioning file(s) were written out
 */
