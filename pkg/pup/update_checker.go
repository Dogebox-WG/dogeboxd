package pup

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type githubReleaseResponse struct {
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	CreatedAt   string `json:"created_at"`
}

type githubReleaseMemoEntry struct {
	found bool
	body  string
	url   string
	date  *time.Time
}

func ParseGitHubOwnerRepo(remote string) (string, bool) {
	// https://github.com/<owner>/<repo>.git
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "http://") {
		location := strings.TrimSuffix(remote, ".git")
		parts := strings.Split(location, "github.com/")
		if len(parts) == 2 && parts[1] != "" {
			return parts[1], true
		}
	}

	// git@github.com:<owner>/<repo>.git
	if strings.HasPrefix(remote, "git@github.com:") {
		location := strings.TrimPrefix(remote, "git@github.com:")
		location = strings.TrimSuffix(location, ".git")
		if location != "" {
			return location, true
		}
	}

	return "", false
}

func githubTagCandidates(tag string) []string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	if strings.HasPrefix(tag, "v") {
		alt := strings.TrimPrefix(tag, "v")
		if alt != "" && alt != tag {
			return []string{tag, alt}
		}
		return []string{tag}
	}
	return []string{tag, "v" + tag}
}

func fetchGitHubReleaseByTag(client *http.Client, ownerRepo, tagName, token string) (githubReleaseMemoEntry, error) {
	// GET https://api.github.com/repos/<owner>/<repo>/releases/tags/<tag>
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", ownerRepo, tagName)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return githubReleaseMemoEntry{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dogeboxd")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return githubReleaseMemoEntry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return githubReleaseMemoEntry{found: false}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Keep it lightweight: don't read the whole body for logs unless needed by caller.
		return githubReleaseMemoEntry{}, fmt.Errorf("github release api returned %s", resp.Status)
	}

	var out githubReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return githubReleaseMemoEntry{}, err
	}

	body := strings.TrimSpace(out.Body)
	url := strings.TrimSpace(out.HTMLURL)

	var releasedAt *time.Time
	if out.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, out.PublishedAt); err == nil {
			releasedAt = &t
		}
	}
	if releasedAt == nil && out.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, out.CreatedAt); err == nil {
			releasedAt = &t
		}
	}

	return githubReleaseMemoEntry{
		found: body != "",
		body:  body,
		url:   url,
		date:  releasedAt,
	}, nil
}

const (
	cacheFileName = "pup-update-cache.json"
)

// UpdateChecker manages checking for pup updates
type UpdateChecker struct {
	pupManager    dogeboxd.PupManager
	sourceManager dogeboxd.SourceManager
	checkInterval time.Duration
	updateCache   map[string]dogeboxd.PupUpdateInfo
	cacheMutex    sync.RWMutex
	dataDir       string
	eventChannel  chan dogeboxd.PupUpdatesCheckedEvent
}

// updateCacheFile represents the structure stored on disk
type updateCacheFile struct {
	Version   int                               `json:"version"`
	UpdatedAt time.Time                         `json:"updatedAt"`
	Cache     map[string]dogeboxd.PupUpdateInfo `json:"cache"`
}

// NewUpdateChecker creates a new update checker
func NewUpdateChecker(pm dogeboxd.PupManager, sm dogeboxd.SourceManager, dataDir string) *UpdateChecker {
	uc := &UpdateChecker{
		pupManager:    pm,
		sourceManager: sm,
		checkInterval: time.Hour, // Check every hour
		updateCache:   make(map[string]dogeboxd.PupUpdateInfo),
		dataDir:       dataDir,
		eventChannel:  make(chan dogeboxd.PupUpdatesCheckedEvent, 10),
	}

	// Load cached data from disk on startup
	if err := uc.loadCacheFromDisk(); err != nil {
		log.Printf("Failed to load update cache from disk: %v (starting fresh)", err)
	}

	return uc
}

// GetEventChannel returns the channel for update check completion events
func (uc *UpdateChecker) GetEventChannel() <-chan dogeboxd.PupUpdatesCheckedEvent {
	return uc.eventChannel
}

// emitEvent sends an event to the event channel (non-blocking)
func (uc *UpdateChecker) emitEvent(event dogeboxd.PupUpdatesCheckedEvent) {
	select {
	case uc.eventChannel <- event:
	default:
		log.Printf("Warning: event channel full, dropping pup updates checked event")
	}
}

// getCacheFilePath returns the path to the cache file
func (uc *UpdateChecker) getCacheFilePath() string {
	return filepath.Join(uc.dataDir, cacheFileName)
}

// loadCacheFromDisk loads the update cache from disk
func (uc *UpdateChecker) loadCacheFromDisk() error {
	filePath := uc.getCacheFilePath()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No existing update cache found at %s", filePath)
			return nil // Not an error, just no cache yet
		}
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	var cacheFile updateCacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return fmt.Errorf("failed to parse cache file: %w", err)
	}

	// Check if cache is too old (older than 24 hours)
	if time.Since(cacheFile.UpdatedAt) > 24*time.Hour {
		log.Printf("Update cache is older than 24 hours, will refresh on next check")
		// Still load it so we have data immediately, but it will be refreshed soon
	}

	uc.cacheMutex.Lock()
	uc.updateCache = cacheFile.Cache
	uc.cacheMutex.Unlock()

	log.Printf("Loaded update cache from disk with %d entries (last updated: %s)",
		len(cacheFile.Cache), cacheFile.UpdatedAt.Format(time.RFC3339))

	return nil
}

// saveCacheToDisk persists the update cache to disk
func (uc *UpdateChecker) saveCacheToDisk() error {
	uc.cacheMutex.RLock()
	cacheCopy := make(map[string]dogeboxd.PupUpdateInfo)
	for k, v := range uc.updateCache {
		cacheCopy[k] = v
	}
	uc.cacheMutex.RUnlock()

	cacheFile := updateCacheFile{
		Version:   1,
		UpdatedAt: time.Now(),
		Cache:     cacheCopy,
	}

	data, err := json.MarshalIndent(cacheFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	filePath := uc.getCacheFilePath()

	// Write to temp file first, then rename for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// ClearCacheEntry removes a specific pup from the cache
func (uc *UpdateChecker) ClearCacheEntry(pupID string) {
	uc.cacheMutex.Lock()
	delete(uc.updateCache, pupID)
	uc.cacheMutex.Unlock()

	// Save to disk asynchronously
	go func() {
		if err := uc.saveCacheToDisk(); err != nil {
			log.Printf("Failed to save cache after clearing entry: %v", err)
		}
	}()

	log.Printf("Cleared update cache entry for pup %s", pupID)
}

// ClearAllCache clears the entire update cache
func (uc *UpdateChecker) ClearAllCache() {
	uc.cacheMutex.Lock()
	uc.updateCache = make(map[string]dogeboxd.PupUpdateInfo)
	uc.cacheMutex.Unlock()

	// Save to disk asynchronously
	go func() {
		if err := uc.saveCacheToDisk(); err != nil {
			log.Printf("Failed to save cache after clearing all: %v", err)
		}
	}()

	log.Printf("Cleared all update cache entries")
}

// ParseVersionLenient attempts to parse a version string, handling non-semver formats
func ParseVersionLenient(versionStr string) (*semver.Version, error) {
	// First try standard semver parsing
	ver, err := semver.NewVersion(versionStr)
	if err == nil {
		return ver, nil
	}

	// Try with 'v' prefix removed if present
	cleanVersion := strings.TrimPrefix(versionStr, "v")
	ver, err = semver.NewVersion(cleanVersion)
	if err == nil {
		return ver, nil
	}

	// Try to extract just the major.minor.patch part
	// This handles versions like "1.0.0-rc1" or "1.0.0.beta.1"
	parts := strings.Split(cleanVersion, "-")
	if len(parts) > 0 {
		ver, err = semver.NewVersion(parts[0])
		if err == nil {
			return ver, nil
		}
	}

	// Try splitting by non-numeric separators
	parts = strings.FieldsFunc(cleanVersion, func(r rune) bool {
		return r != '.' && (r < '0' || r > '9')
	})
	if len(parts) > 0 {
		ver, err = semver.NewVersion(parts[0])
		if err == nil {
			return ver, nil
		}
	}

	return nil, fmt.Errorf("unable to parse version: %s", versionStr)
}

func (uc *UpdateChecker) checkForUpdatesWithMemo(pupID string, memo map[string]githubReleaseMemoEntry) (dogeboxd.PupUpdateInfo, error) {
	pup, _, err := uc.pupManager.GetPup(pupID)
	if err != nil {
		return dogeboxd.PupUpdateInfo{}, fmt.Errorf("failed to get pup: %w", err)
	}

	updateInfo := dogeboxd.PupUpdateInfo{
		PupID:             pupID,
		CurrentVersion:    pup.Version,
		AvailableVersions: []dogeboxd.PupVersion{},
		UpdateAvailable:   false,
		LastChecked:       time.Now(),
	}

	// Only check for updates from git sources
	if pup.Source.Type != "git" {
		return updateInfo, nil
	}

	// Get the source to fetch available versions
	source, err := uc.sourceManager.GetSource(pup.Source.ID)
	if err != nil {
		return updateInfo, fmt.Errorf("failed to get source: %w", err)
	}

	// Force refresh to get latest versions from git tags
	sourceList, err := source.List(true)
	if err != nil {
		return updateInfo, fmt.Errorf("failed to list source pups: %w", err)
	}

	// Parse current version
	currentVer, err := ParseVersionLenient(pup.Version)
	if err != nil {
		return updateInfo, fmt.Errorf("failed to parse current version: %w", err)
	}

	var availableVersions []dogeboxd.PupVersion
	var latestVersion *semver.Version
	tagHintByVersion := map[string]string{}

	// Find all versions of this specific pup from the source
	for _, sourcePup := range sourceList.Pups {
		// Match by pup name
		if sourcePup.Name != pup.Manifest.Meta.Name {
			continue
		}

		// Parse the manifest version
		ver, err := ParseVersionLenient(sourcePup.Version)
		if err != nil {
			continue
		}

		// Only include versions newer than current
		if ver.GreaterThan(currentVer) {
			if sourcePup.Location != nil {
				if t, ok := sourcePup.Location["tag"]; ok && strings.TrimSpace(t) != "" {
					tagHintByVersion[sourcePup.Version] = strings.TrimSpace(t)
				}
			}
			availableVersions = append(availableVersions, dogeboxd.PupVersion{
				Version:      sourcePup.Version,
				ReleaseNotes: sourcePup.ReleaseNotes,
				ReleaseDate:  sourcePup.ReleaseDate,
				ReleaseURL:   sourcePup.ReleaseURL,
			})

			// Track latest version
			if latestVersion == nil || ver.GreaterThan(latestVersion) {
				latestVersion = ver
			}
		}
	}

	// Preserve any cached release notes/date/url already stored for this pup/version.
	uc.cacheMutex.RLock()
	prev, ok := uc.updateCache[pupID]
	uc.cacheMutex.RUnlock()
	if ok && len(prev.AvailableVersions) > 0 && len(availableVersions) > 0 {
		prevByVersion := map[string]dogeboxd.PupVersion{}
		for _, v := range prev.AvailableVersions {
			if strings.TrimSpace(v.Version) != "" {
				prevByVersion[v.Version] = v
			}
		}
		for i := range availableVersions {
			if pv, exists := prevByVersion[availableVersions[i].Version]; exists {
				if strings.TrimSpace(availableVersions[i].ReleaseNotes) == "" && strings.TrimSpace(pv.ReleaseNotes) != "" {
					availableVersions[i].ReleaseNotes = pv.ReleaseNotes
				}
				if availableVersions[i].ReleaseDate == nil && pv.ReleaseDate != nil {
					availableVersions[i].ReleaseDate = pv.ReleaseDate
				}
				if strings.TrimSpace(availableVersions[i].ReleaseURL) == "" && strings.TrimSpace(pv.ReleaseURL) != "" {
					availableVersions[i].ReleaseURL = pv.ReleaseURL
				}
			}
		}
	}

	// Fetch GitHub release notes for all newer versions (only if missing).
	ownerRepo, isGitHub := ParseGitHubOwnerRepo(pup.Source.Location)
	if isGitHub && len(availableVersions) > 0 {
		client := &http.Client{Timeout: 4 * time.Second}
		token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		rateLimited := false

		for i := range availableVersions {
			if strings.TrimSpace(availableVersions[i].ReleaseNotes) != "" {
				continue
			}
			if rateLimited {
				break
			}

			tagHint := strings.TrimSpace(tagHintByVersion[availableVersions[i].Version])
			baseTag := tagHint
			if baseTag == "" {
				baseTag = availableVersions[i].Version
			}

			// Ensure we have a sensible fallback release URL even if API doesn't return html_url
			if availableVersions[i].ReleaseURL == "" {
				fallbackTag := baseTag
				if fallbackTag == "" {
					fallbackTag = availableVersions[i].Version
				}
				availableVersions[i].ReleaseURL = fmt.Sprintf("https://github.com/%s/releases/tag/%s", ownerRepo, fallbackTag)
			}

			for _, tag := range githubTagCandidates(baseTag) {
				key := ownerRepo + "|" + tag
				if entry, ok := memo[key]; ok {
					if entry.found {
						availableVersions[i].ReleaseNotes = entry.body
						if availableVersions[i].ReleaseDate == nil && entry.date != nil {
							availableVersions[i].ReleaseDate = entry.date
						}
						if entry.url != "" {
							availableVersions[i].ReleaseURL = entry.url
						}
					}
					break // whether found or not, don't refetch this tag
				}

				entry, err := fetchGitHubReleaseByTag(client, ownerRepo, tag, token)
				if err != nil {
					// Keep logs minimal: only real errors
					if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "429") {
						log.Printf("GitHub release fetch rate-limited/forbidden for %s: %v", ownerRepo, err)
						rateLimited = true
					} else {
						log.Printf("GitHub release fetch failed for %s (tag %s): %v", ownerRepo, tag, err)
					}
					memo[key] = githubReleaseMemoEntry{found: false}
					break
				}
				memo[key] = entry
				if entry.found {
					availableVersions[i].ReleaseNotes = entry.body
					if availableVersions[i].ReleaseDate == nil && entry.date != nil {
						availableVersions[i].ReleaseDate = entry.date
					}
					if entry.url != "" {
						availableVersions[i].ReleaseURL = entry.url
					}
					break
				}
			}
		}
	}

	// Update the info struct
	updateInfo.AvailableVersions = availableVersions
	if latestVersion != nil {
		updateInfo.LatestVersion = latestVersion.String()
		updateInfo.UpdateAvailable = true
	}

	// Cache the result
	uc.cacheMutex.Lock()
	uc.updateCache[pupID] = updateInfo
	uc.cacheMutex.Unlock()

	// Save to disk immediately for this single pup check
	if err := uc.saveCacheToDisk(); err != nil {
		log.Printf("Failed to save update cache to disk: %v", err)
	}

	// Emit event to notify that cache has been updated
	updatesAvailable := 0
	if updateInfo.UpdateAvailable {
		updatesAvailable = 1
	}
	uc.emitEvent(dogeboxd.PupUpdatesCheckedEvent{
		PupsChecked:      1,
		UpdatesAvailable: updatesAvailable,
		IsPeriodicCheck:  false,
	})

	return updateInfo, nil
}

// CheckForUpdates checks if a specific pup has updates available
func (uc *UpdateChecker) CheckForUpdates(pupID string) (dogeboxd.PupUpdateInfo, error) {
	return uc.checkForUpdatesWithMemo(pupID, map[string]githubReleaseMemoEntry{})
}

// CheckAllPupUpdates checks for updates on all installed pups
func (uc *UpdateChecker) CheckAllPupUpdates() map[string]dogeboxd.PupUpdateInfo {
	return uc.checkAllPupUpdatesInternal(false)
}

// checkAllPupUpdatesInternal checks for updates on all installed pups
// isPeriodic indicates if this is a periodic background check
func (uc *UpdateChecker) checkAllPupUpdatesInternal(isPeriodic bool) map[string]dogeboxd.PupUpdateInfo {
	allUpdates := make(map[string]dogeboxd.PupUpdateInfo)
	stateMap := uc.pupManager.GetStateMap()
	memo := map[string]githubReleaseMemoEntry{}

	updatesAvailable := 0
	for pupID, pupState := range stateMap {
		updateInfo, err := uc.checkForUpdatesWithMemo(pupID, memo)
		if err != nil {
			log.Printf("Error checking %s: %v", pupState.Manifest.Meta.Name, err)
			// Continue checking other pups
			continue
		}
		allUpdates[pupID] = updateInfo
		if updateInfo.UpdateAvailable {
			updatesAvailable++
		}
	}

	// Save cache to disk after checking all pups
	if err := uc.saveCacheToDisk(); err != nil {
		log.Printf("Failed to persist update cache to disk: %v", err)
	}

	// Emit event to notify frontend
	uc.emitEvent(dogeboxd.PupUpdatesCheckedEvent{
		PupsChecked:      len(allUpdates),
		UpdatesAvailable: updatesAvailable,
		IsPeriodicCheck:  isPeriodic,
	})

	return allUpdates
}

// GetCachedUpdateInfo retrieves cached update info for a pup
func (uc *UpdateChecker) GetCachedUpdateInfo(pupID string) (dogeboxd.PupUpdateInfo, bool) {
	uc.cacheMutex.RLock()
	defer uc.cacheMutex.RUnlock()

	info, ok := uc.updateCache[pupID]
	return info, ok
}

// GetAllCachedUpdates returns all cached update information
func (uc *UpdateChecker) GetAllCachedUpdates() map[string]dogeboxd.PupUpdateInfo {
	uc.cacheMutex.RLock()
	defer uc.cacheMutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]dogeboxd.PupUpdateInfo)
	for k, v := range uc.updateCache {
		result[k] = v
	}

	return result
}

// DetectInterfaceChanges compares interfaces between two manifests
func (uc *UpdateChecker) DetectInterfaceChanges(oldManifest, newManifest dogeboxd.PupManifest) []dogeboxd.PupInterfaceVersion {
	changes := []dogeboxd.PupInterfaceVersion{}

	// Create maps of old and new interfaces for easy lookup
	oldInterfaces := make(map[string]dogeboxd.PupManifestInterface)
	for _, iface := range oldManifest.Interfaces {
		oldInterfaces[iface.Name] = iface
	}

	newInterfaces := make(map[string]dogeboxd.PupManifestInterface)
	for _, iface := range newManifest.Interfaces {
		newInterfaces[iface.Name] = iface
	}

	// Check for changed interfaces
	for name, newIface := range newInterfaces {
		oldIface, exists := oldInterfaces[name]
		if !exists {
			// New interface added
			continue
		}

		// Compare versions
		oldVer, err := semver.NewVersion(oldIface.Version)
		if err != nil {
			continue
		}

		newVer, err := semver.NewVersion(newIface.Version)
		if err != nil {
			continue
		}

		// Determine change type
		var changeType string
		if newVer.Major() > oldVer.Major() {
			changeType = "major"
		} else if newVer.Minor() > oldVer.Minor() {
			changeType = "minor"
		} else if newVer.Patch() > oldVer.Patch() {
			changeType = "patch"
		} else {
			continue // No change
		}

		// Find affected pups
		affectedPups := uc.FindAffectedPups(name, oldIface.Version, newIface.Version)

		changes = append(changes, dogeboxd.PupInterfaceVersion{
			InterfaceName: name,
			OldVersion:    oldIface.Version,
			NewVersion:    newIface.Version,
			ChangeType:    changeType,
			AffectedPups:  affectedPups,
		})
	}

	return changes
}

// FindAffectedPups finds all pups that depend on a specific interface
func (uc *UpdateChecker) FindAffectedPups(interfaceName, oldVersion, newVersion string) []string {
	affectedPups := []string{}
	stateMap := uc.pupManager.GetStateMap()

	for pupID, pupState := range stateMap {
		// Check if this pup depends on the interface
		for _, dep := range pupState.Manifest.Dependencies {
			if dep.InterfaceName == interfaceName {
				// Check if the version change would affect this pup
				// For now, just add it to the list
				affectedPups = append(affectedPups, pupID)
				break
			}
		}
	}

	return affectedPups
}

// StartPeriodicCheck starts a background goroutine that checks for updates periodically
func (uc *UpdateChecker) StartPeriodicCheck(stop chan bool) {
	go func() {
		// Initial check after 30 seconds (to allow system to fully boot)
		// But we already have cached data loaded from disk, so UI can show updates immediately
		time.Sleep(30 * time.Second)
		uc.checkAllPupUpdatesInternal(true)

		ticker := time.NewTicker(uc.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				uc.checkAllPupUpdatesInternal(true)
			case <-stop:
				return
			}
		}
	}()
}
