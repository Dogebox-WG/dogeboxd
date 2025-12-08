package pup

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

const (
	skippedUpdatesFileName = "skipped-updates.json"
)

// SkippedUpdatesManager manages skipped update preferences
type SkippedUpdatesManager struct {
	skippedUpdates map[string]dogeboxd.SkippedPupUpdate
	mutex          sync.RWMutex
	dataDir        string
}

// skippedUpdatesFile represents the structure stored on disk
type skippedUpdatesFile struct {
	Version        int                                  `json:"version"`
	UpdatedAt      time.Time                            `json:"updatedAt"`
	SkippedUpdates map[string]dogeboxd.SkippedPupUpdate `json:"skippedUpdates"`
}

// NewSkippedUpdatesManager creates a new skipped updates manager
func NewSkippedUpdatesManager(dataDir string) *SkippedUpdatesManager {
	sm := &SkippedUpdatesManager{
		skippedUpdates: make(map[string]dogeboxd.SkippedPupUpdate),
		dataDir:        dataDir,
	}

	// Load persisted data from disk on startup
	if err := sm.loadFromDisk(); err != nil {
		log.Printf("Failed to load skipped updates from disk: %v (starting fresh)", err)
	}

	return sm
}

// getFilePath returns the path to the skipped updates file
func (sm *SkippedUpdatesManager) getFilePath() string {
	return filepath.Join(sm.dataDir, skippedUpdatesFileName)
}

// loadFromDisk loads the skipped updates from disk
func (sm *SkippedUpdatesManager) loadFromDisk() error {
	filePath := sm.getFilePath()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No existing skipped updates file found at %s", filePath)
			return nil // Not an error, just no data yet
		}
		return fmt.Errorf("failed to read skipped updates file: %w", err)
	}

	var file skippedUpdatesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("failed to parse skipped updates file: %w", err)
	}

	sm.mutex.Lock()
	sm.skippedUpdates = file.SkippedUpdates
	sm.mutex.Unlock()

	log.Printf("Loaded skipped updates from disk with %d entries (last updated: %s)",
		len(file.SkippedUpdates), file.UpdatedAt.Format(time.RFC3339))

	return nil
}

// saveToDisk persists the skipped updates to disk
func (sm *SkippedUpdatesManager) saveToDisk() error {
	sm.mutex.RLock()
	skippedCopy := make(map[string]dogeboxd.SkippedPupUpdate)
	for k, v := range sm.skippedUpdates {
		skippedCopy[k] = v
	}
	sm.mutex.RUnlock()

	file := skippedUpdatesFile{
		Version:        1,
		UpdatedAt:      time.Now(),
		SkippedUpdates: skippedCopy,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal skipped updates: %w", err)
	}

	filePath := sm.getFilePath()

	// Write to temp file first, then rename for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write skipped updates file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to rename skipped updates file: %w", err)
	}

	return nil
}

// SkipUpdate marks updates as skipped for a specific pup
func (sm *SkippedUpdatesManager) SkipUpdate(pupID, currentVersion, latestVersion string) error {
	sm.mutex.Lock()
	sm.skippedUpdates[pupID] = dogeboxd.SkippedPupUpdate{
		PupID:               pupID,
		SkippedAtVersion:    currentVersion,
		LatestVersionAtSkip: latestVersion,
		SkippedAt:           time.Now(),
	}
	sm.mutex.Unlock()

	// Persist to disk
	if err := sm.saveToDisk(); err != nil {
		log.Printf("Failed to save skipped updates after skip: %v", err)
		return err
	}

	log.Printf("Skipped updates for pup %s up to version %s", pupID, latestVersion)
	return nil
}

// ClearSkipped removes the skip status for a specific pup
func (sm *SkippedUpdatesManager) ClearSkipped(pupID string) error {
	sm.mutex.Lock()
	delete(sm.skippedUpdates, pupID)
	sm.mutex.Unlock()

	// Persist to disk
	if err := sm.saveToDisk(); err != nil {
		log.Printf("Failed to save skipped updates after clear: %v", err)
		return err
	}

	log.Printf("Cleared skip status for pup %s", pupID)
	return nil
}

// IsSkipped checks if updates are currently skipped for a pup
// Returns true if the given latestVersion is <= the latestVersionAtSkip
func (sm *SkippedUpdatesManager) IsSkipped(pupID, latestVersion string) bool {
	sm.mutex.RLock()
	skipInfo, exists := sm.skippedUpdates[pupID]
	sm.mutex.RUnlock()

	if !exists {
		return false
	}

	// Parse versions using lenient parsing (same as update checker)
	current, err := ParseVersionLenient(latestVersion)
	if err != nil {
		log.Printf("Failed to parse current version '%s' in IsSkipped: %v", latestVersion, err)
		// Fall back to string comparison if parsing fails
		return latestVersion <= skipInfo.LatestVersionAtSkip
	}

	skip, err := ParseVersionLenient(skipInfo.LatestVersionAtSkip)
	if err != nil {
		log.Printf("Failed to parse skip version '%s' in IsSkipped: %v", skipInfo.LatestVersionAtSkip, err)
		// Fall back to string comparison if parsing fails
		return latestVersion <= skipInfo.LatestVersionAtSkip
	}

	// Return true if current version is less than or equal to the skipped version
	return current.LessThan(skip) || current.Equal(skip)
}

// GetSkipInfo retrieves skip info for a specific pup
func (sm *SkippedUpdatesManager) GetSkipInfo(pupID string) (dogeboxd.SkippedPupUpdate, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	info, exists := sm.skippedUpdates[pupID]
	return info, exists
}

// GetAllSkipped returns all skipped updates
func (sm *SkippedUpdatesManager) GetAllSkipped() map[string]dogeboxd.SkippedPupUpdate {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]dogeboxd.SkippedPupUpdate)
	for k, v := range sm.skippedUpdates {
		result[k] = v
	}
	return result
}
