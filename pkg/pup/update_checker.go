package pup

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

const (
	cacheFileName = "pup-update-cache.json"
)

// UpdateChecker manages checking for pup updates
type UpdateChecker struct {
	pupManager    dogeboxd.PupManager
	sourceManager dogeboxd.SourceManager
	githubClient  *GitHubClient
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
		githubClient:  NewGitHubClient(),
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

// parseVersionLenient attempts to parse a version string, handling non-semver formats
func parseVersionLenient(versionStr string) (*semver.Version, error) {
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

// CheckForUpdates checks if a specific pup has updates available
func (uc *UpdateChecker) CheckForUpdates(pupID string) (dogeboxd.PupUpdateInfo, error) {
	pup, _, err := uc.pupManager.GetPup(pupID)
	if err != nil {
		return dogeboxd.PupUpdateInfo{}, fmt.Errorf("failed to get pup: %w", err)
	}

	log.Printf("Checking pup: %s (current version: %s)", pup.Manifest.Meta.Name, pup.Version)
	log.Printf("Source: %s (%s)", pup.Source.Type, pup.Source.Location)

	updateInfo := dogeboxd.PupUpdateInfo{
		PupID:             pupID,
		CurrentVersion:    pup.Version,
		AvailableVersions: []dogeboxd.PupVersion{},
		UpdateAvailable:   false,
		LastChecked:       time.Now(),
	}

	// Only check for updates from git sources
	if pup.Source.Type != "git" {
		log.Printf("Skipping: source type '%s' not supported for updates", pup.Source.Type)
		return updateInfo, nil
	}

	// Fetch releases from GitHub
	log.Printf("Fetching releases from GitHub...")
	releases, err := uc.githubClient.FetchReleases(pup.Source.Location)
	if err != nil {
		log.Printf("Failed to fetch releases: %v", err)
		return updateInfo, fmt.Errorf("failed to fetch releases: %w", err)
	}
	log.Printf("Found %d total releases", len(releases))

	// Filter to stable releases only and parse versions
	currentVer, err := parseVersionLenient(pup.Version)
	if err != nil {
		log.Printf("Failed to parse current version '%s': %v", pup.Version, err)
		return updateInfo, fmt.Errorf("failed to parse current version: %w", err)
	}

	var availableVersions []dogeboxd.PupVersion
	var latestVersion *semver.Version
	skippedCount := 0

	for _, release := range releases {
		// Skip drafts and pre-releases
		if release.Draft || release.Prerelease {
			skippedCount++
			continue
		}

		// Parse version from tag name
		versionStr := strings.TrimPrefix(release.TagName, "v")
		ver, err := parseVersionLenient(versionStr)
		if err != nil {
			log.Printf("Skipping invalid version %s: %v", versionStr, err)
			skippedCount++
			continue
		}

		// Only include versions newer than current
		if ver.GreaterThan(currentVer) {
			log.Printf("Found newer version: %s (released %s)", versionStr, release.PublishedAt.Format("2006-01-02"))
			availableVersions = append(availableVersions, dogeboxd.PupVersion{
				Version:      versionStr,
				ReleaseNotes: release.Body,
				ReleaseDate:  release.PublishedAt,
				ReleaseURL:   release.HTMLURL,
			})

			// Track latest version
			if latestVersion == nil || ver.GreaterThan(latestVersion) {
				latestVersion = ver
			}
		} else {
			skippedCount++
		}
	}

	log.Printf("Filtered results: %d newer versions found, %d skipped (older/draft/pre-release/invalid)", len(availableVersions), skippedCount)

	// Update the info struct
	updateInfo.AvailableVersions = availableVersions
	if latestVersion != nil {
		updateInfo.LatestVersion = latestVersion.String()
		updateInfo.UpdateAvailable = true
		log.Printf("✓ Update available: %s → %s", pup.Version, latestVersion.String())
	} else {
		log.Printf("✓ No updates available, pup is up to date")
	}

	// Cache the result
	uc.cacheMutex.Lock()
	uc.updateCache[pupID] = updateInfo
	uc.cacheMutex.Unlock()

	return updateInfo, nil
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

	log.Printf("Checking %d installed pup(s) for updates", len(stateMap))

	updatesAvailable := 0
	for pupID, pupState := range stateMap {
		log.Printf("--- Checking %s ---", pupState.Manifest.Meta.Name)
		updateInfo, err := uc.CheckForUpdates(pupID)
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
