package pup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

const (
	githubAPIBase = "https://api.github.com"
	userAgent     = "Dogebox/1.0"
)

// GitHubClient handles interactions with GitHub API
type GitHubClient struct {
	httpClient *http.Client
	token      string // Optional authentication token
}

// NewGitHubClient creates a new GitHub API client
func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ParseGitHubURL extracts owner and repo from a GitHub URL
// Supports: https://github.com/owner/repo, https://github.com/owner/repo.git, git@github.com:owner/repo.git
func ParseGitHubURL(repoURL string) (owner, repo string, err error) {
	// Handle git@ URLs
	if strings.HasPrefix(repoURL, "git@github.com:") {
		repoURL = strings.TrimPrefix(repoURL, "git@github.com:")
		repoURL = strings.TrimSuffix(repoURL, ".git")
		parts := strings.Split(repoURL, "/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid GitHub URL format")
		}
		return parts[0], parts[1], nil
	}

	// Handle https URLs
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	if parsedURL.Host != "github.com" {
		return "", "", fmt.Errorf("not a GitHub URL: %s", parsedURL.Host)
	}

	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub URL path: %s", parsedURL.Path)
	}

	owner = pathParts[0]
	repo = strings.TrimSuffix(pathParts[1], ".git")

	return owner, repo, nil
}

// FetchReleases fetches all releases for a GitHub repository
func (c *GitHubClient) FetchReleases(repoURL string) ([]dogeboxd.GitHubRelease, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases", githubAPIBase, owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var releases []dogeboxd.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode releases: %w", err)
	}

	return releases, nil
}

// FetchLatestRelease fetches the latest non-draft, non-prerelease release
func (c *GitHubClient) FetchLatestRelease(repoURL string) (*dogeboxd.GitHubRelease, error) {
	releases, err := c.FetchReleases(repoURL)
	if err != nil {
		return nil, err
	}

	for _, release := range releases {
		if !release.Draft && !release.Prerelease {
			return &release, nil
		}
	}

	return nil, fmt.Errorf("no stable releases found")
}
