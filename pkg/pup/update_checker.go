package pup

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// UpdateChecker manages checking for pup updates
type UpdateChecker struct {
	pupManager    dogeboxd.PupManager
	sourceManager dogeboxd.SourceManager
	githubClient  *GitHubClient
	checkInterval time.Duration
	updateCache   map[string]dogeboxd.PupUpdateInfo
	cacheMutex    sync.RWMutex
}

// NewUpdateChecker creates a new update checker
func NewUpdateChecker(pm dogeboxd.PupManager, sm dogeboxd.SourceManager) *UpdateChecker {
	return &UpdateChecker{
		pupManager:    pm,
		sourceManager: sm,
		githubClient:  NewGitHubClient(),
		checkInterval: time.Hour, // Check every hour
		updateCache:   make(map[string]dogeboxd.PupUpdateInfo),
	}
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
	currentVer, err := semver.NewVersion(pup.Version)
	if err != nil {
		log.Printf("Failed to parse current version: %v", err)
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
		ver, err := semver.NewVersion(versionStr)
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
	allUpdates := make(map[string]dogeboxd.PupUpdateInfo)
	stateMap := uc.pupManager.GetStateMap()

	log.Printf("Checking %d installed pup(s) for updates", len(stateMap))

	for pupID, pupState := range stateMap {
		log.Printf("--- Checking %s ---", pupState.Manifest.Meta.Name)
		updateInfo, err := uc.CheckForUpdates(pupID)
		if err != nil {
			log.Printf("Error checking %s: %v", pupState.Manifest.Meta.Name, err)
			// Continue checking other pups
			continue
		}
		allUpdates[pupID] = updateInfo
	}

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
		time.Sleep(30 * time.Second)
		uc.CheckAllPupUpdates()

		ticker := time.NewTicker(uc.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Println("Running periodic pup update check...")
				uc.CheckAllPupUpdates()
			case <-stop:
				log.Println("Stopping periodic update checker")
				return
			}
		}
	}()
}
