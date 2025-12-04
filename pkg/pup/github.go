package pup

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

const (
	githubAPIBase       = "https://api.github.com"
	userAgent           = "Dogebox/1.0"
	defaultPerPage      = 100
	maxRetries          = 3
	initialBackoffSecs  = 5
	rateLimitBackoffMin = 60 // Wait at least 60 seconds on rate limit
)

// GitHubClient handles interactions with GitHub API
type GitHubClient struct {
	httpClient *http.Client
	token      string // Optional authentication token
}

// NewGitHubClient creates a new GitHub API client
// Optionally reads GITHUB_TOKEN from environment for higher rate limits
func NewGitHubClient() *GitHubClient {
	token := os.Getenv("GITHUB_TOKEN")
	return &GitHubClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// SetToken sets the GitHub API token for authenticated requests
func (c *GitHubClient) SetToken(token string) {
	c.token = token
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

// RateLimitError represents a GitHub API rate limit error
type RateLimitError struct {
	ResetTime time.Time
	Message   string
}

func (e *RateLimitError) Error() string {
	return e.Message
}

// doRequest performs an HTTP request with rate limit handling and retries
func (c *GitHubClient) doRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	var lastErr error
	backoff := initialBackoffSecs

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("GitHub API retry %d/%d after %ds backoff", attempt+1, maxRetries, backoff)
			time.Sleep(time.Duration(backoff) * time.Second)
			backoff *= 2 // Exponential backoff
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Check for rate limiting
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// Check if this is specifically a rate limit error
			if strings.Contains(string(body), "rate limit") || resp.StatusCode == http.StatusTooManyRequests {
				resetTime := c.parseRateLimitReset(resp)
				waitSecs := rateLimitBackoffMin

				if !resetTime.IsZero() {
					waitSecs = int(time.Until(resetTime).Seconds()) + 1
					if waitSecs < rateLimitBackoffMin {
						waitSecs = rateLimitBackoffMin
					}
				}

				log.Printf("GitHub API rate limit hit, waiting %ds before retry", waitSecs)
				lastErr = &RateLimitError{
					ResetTime: resetTime,
					Message:   fmt.Sprintf("rate limit exceeded, resets at %v", resetTime),
				}

				// Only wait and retry if we have attempts left
				if attempt < maxRetries-1 {
					time.Sleep(time.Duration(waitSecs) * time.Second)
				}
				continue
			}

			// Not a rate limit error, return the error
			return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Check for other errors
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// parseRateLimitReset extracts the rate limit reset time from response headers
func (c *GitHubClient) parseRateLimitReset(resp *http.Response) time.Time {
	resetHeader := resp.Header.Get("X-RateLimit-Reset")
	if resetHeader == "" {
		return time.Time{}
	}

	resetUnix, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(resetUnix, 0)
}

// parseNextPageURL extracts the next page URL from the Link header
func (c *GitHubClient) parseNextPageURL(resp *http.Response) string {
	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		return ""
	}

	// Parse Link header format: <url>; rel="next", <url>; rel="last"
	re := regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

// FetchReleases fetches all releases for a GitHub repository with pagination
func (c *GitHubClient) FetchReleases(repoURL string) ([]dogeboxd.GitHubRelease, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	var allReleases []dogeboxd.GitHubRelease
	nextURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", githubAPIBase, owner, repo, defaultPerPage)

	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.doRequest(req)
		if err != nil {
			return nil, err
		}

		var releases []dogeboxd.GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode releases: %w", err)
		}

		// Get next page URL before closing response
		nextURL = c.parseNextPageURL(resp)
		resp.Body.Close()

		allReleases = append(allReleases, releases...)

		// Safety limit to prevent infinite loops
		if len(allReleases) > 1000 {
			log.Printf("Warning: truncating releases at 1000 entries for %s/%s", owner, repo)
			break
		}
	}

	return allReleases, nil
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

// GetRateLimitStatus returns the current rate limit status
func (c *GitHubClient) GetRateLimitStatus() (remaining int, resetTime time.Time, err error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rate_limit", githubAPIBase), nil)
	if err != nil {
		return 0, time.Time{}, err
	}

	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	remaining, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	resetTime = c.parseRateLimitReset(resp)

	return remaining, resetTime, nil
}
