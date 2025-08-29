package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestHelper provides utilities for testing upgrades functionality
type TestHelper struct {
	t               *testing.T
	tempDir         string
	originalWorkDir string
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T) *TestHelper {
	tempDir, err := os.MkdirTemp("", "upgrades_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	originalWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	return &TestHelper{
		t:               t,
		tempDir:         tempDir,
		originalWorkDir: originalWorkDir,
	}
}

// Cleanup removes temporary test files and restores working directory
func (th *TestHelper) Cleanup() {
	os.Chdir(th.originalWorkDir)
	os.RemoveAll(th.tempDir)
}

// CreateMockGitRepo creates a mock git repository for testing
func (th *TestHelper) CreateMockGitRepo() string {
	repoDir := filepath.Join(th.tempDir, "mock_repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		th.t.Fatalf("Failed to create mock repo directory: %v", err)
	}

	// Create a basic git repository structure (without actual git operations)
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		th.t.Fatalf("Failed to create .git directory: %v", err)
	}

	return repoDir
}

// AssertUpgradableRelease checks if an upgradable release has expected properties
func AssertUpgradableRelease(t *testing.T, release UpgradableRelease, expectedVersion string) {
	if release.Version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, release.Version)
	}

	expectedURL := fmt.Sprintf("https://github.com/dogebox-wg/os/releases/tag/%s", expectedVersion)
	if release.ReleaseURL != expectedURL {
		t.Errorf("Expected release URL %s, got %s", expectedURL, release.ReleaseURL)
	}

	expectedSummary := "Update for Dogeboxd / DKM / DPanel"
	if release.Summary != expectedSummary {
		t.Errorf("Expected summary '%s', got '%s'", expectedSummary, release.Summary)
	}
}

// Common test data generators
func GenerateSequentialVersions(major, minor int, count int) []RepositoryTag {
	tags := make([]RepositoryTag, count)
	for i := 0; i < count; i++ {
		tags[i] = RepositoryTag{Tag: fmt.Sprintf("v%d.%d.%d", major, minor+i, 0)}
	}
	return tags
}

// Error scenarios for testing
var (
	ErrNetworkTimeout   = fmt.Errorf("network timeout")
	ErrUnauthorized     = fmt.Errorf("unauthorized access")
	ErrRepoNotFound     = fmt.Errorf("repository not found")
	ErrInvalidResponse  = fmt.Errorf("invalid response format")
)

// CreateErrorMock creates a mock that returns an error
func CreateErrorMock(err error) *MockRepoTagsFetcher {
	return &MockRepoTagsFetcher{
		tags: nil,
		err:  err,
	}
}

// CreateSuccessMock creates a mock that returns specific tags
func CreateSuccessMock(tags []RepositoryTag) *MockRepoTagsFetcher {
	return &MockRepoTagsFetcher{
		tags: tags,
		err:  nil,
	}
}

// TableDrivenTest represents a test case for table-driven testing
type TableDrivenTest struct {
	Name            string
	CurrentVersion  string
	AvailableTags   []RepositoryTag
	MockError       error
	ExpectedCount   int
	ExpectedError   string
	ExpectedVersions []string
}

// RunTableDrivenTests executes a series of table-driven tests for GetUpgradableReleases
func RunTableDrivenTests(t *testing.T, tests []TableDrivenTest) {
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			// Setup mock
			mockFetcher := &MockRepoTagsFetcher{
				tags: tt.AvailableTags,
				err:  tt.MockError,
			}

			// Mock version
			tempDir := setupMockVersioning(t, tt.CurrentVersion)
			defer os.RemoveAll(tempDir)

			releases, err := GetUpgradableReleasesWithFetcher(mockFetcher)

			// Check error expectations
			if tt.ExpectedError != "" {
				if err == nil {
					t.Fatalf("Expected error '%s', got nil", tt.ExpectedError)
				}
				if err.Error() != tt.ExpectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.ExpectedError, err.Error())
				}
				return
			}

			// Check success expectations
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(releases) != tt.ExpectedCount {
				t.Errorf("Expected %d releases, got %d", tt.ExpectedCount, len(releases))
			}

			// Check specific versions if provided
			if len(tt.ExpectedVersions) > 0 {
				if len(releases) != len(tt.ExpectedVersions) {
					t.Errorf("Expected %d versions, got %d", len(tt.ExpectedVersions), len(releases))
				}

				for i, expectedVersion := range tt.ExpectedVersions {
					if i >= len(releases) {
						t.Errorf("Missing expected version %s at index %d", expectedVersion, i)
						continue
					}
					AssertUpgradableRelease(t, releases[i], expectedVersion)
				}
			}
		})
	}
}

// IntegrationTestSuite contains a collection of integration-style tests
func RunIntegrationTestSuite(t *testing.T) {
	testCases := []TableDrivenTest{
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