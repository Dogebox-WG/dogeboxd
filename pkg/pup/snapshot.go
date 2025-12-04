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
	snapshotsDir = "pup-snapshots"
)

// SnapshotManager handles saving and loading pup version snapshots for rollback
type SnapshotManager struct {
	dataDir string
	mutex   sync.RWMutex
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(dataDir string) *SnapshotManager {
	sm := &SnapshotManager{
		dataDir: dataDir,
	}

	// Ensure snapshots directory exists
	snapshotPath := sm.getSnapshotsPath()
	if err := os.MkdirAll(snapshotPath, 0755); err != nil {
		log.Printf("Warning: failed to create snapshots directory: %v", err)
	}

	return sm
}

// getSnapshotsPath returns the path to the snapshots directory
func (sm *SnapshotManager) getSnapshotsPath() string {
	return filepath.Join(sm.dataDir, snapshotsDir)
}

// getSnapshotFilePath returns the path to a specific pup's snapshot file
func (sm *SnapshotManager) getSnapshotFilePath(pupID string) string {
	return filepath.Join(sm.getSnapshotsPath(), fmt.Sprintf("%s.json", pupID))
}

// CreateSnapshot creates a snapshot of the current pup state before an upgrade
func (sm *SnapshotManager) CreateSnapshot(pupState dogeboxd.PupState) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	snapshot := dogeboxd.PupVersionSnapshot{
		Version:        pupState.Version,
		Manifest:       pupState.Manifest,
		Config:         pupState.Config,
		Providers:      pupState.Providers,
		Enabled:        pupState.Enabled,
		SnapshotDate:   time.Now(),
		SourceID:       pupState.Source.ID,
		SourceLocation: pupState.Source.Location,
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	filePath := sm.getSnapshotFilePath(pupState.ID)

	// Write to temp file first, then rename for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename snapshot file: %w", err)
	}

	log.Printf("Created upgrade snapshot for pup %s (version %s)", pupState.ID, pupState.Version)
	return nil
}

// GetSnapshot retrieves a pup's version snapshot if it exists
func (sm *SnapshotManager) GetSnapshot(pupID string) (*dogeboxd.PupVersionSnapshot, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	filePath := sm.getSnapshotFilePath(pupID)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No snapshot exists
		}
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snapshot dogeboxd.PupVersionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot file: %w", err)
	}

	return &snapshot, nil
}

// HasSnapshot checks if a snapshot exists for a pup
func (sm *SnapshotManager) HasSnapshot(pupID string) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	filePath := sm.getSnapshotFilePath(pupID)
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteSnapshot removes a pup's version snapshot
func (sm *SnapshotManager) DeleteSnapshot(pupID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	filePath := sm.getSnapshotFilePath(pupID)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted, not an error
		}
		return fmt.Errorf("failed to delete snapshot file: %w", err)
	}

	log.Printf("Deleted upgrade snapshot for pup %s", pupID)
	return nil
}

// ListSnapshots returns a list of all pup IDs that have snapshots
func (sm *SnapshotManager) ListSnapshots() ([]string, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	snapshotPath := sm.getSnapshotsPath()
	entries, err := os.ReadDir(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var pupIDs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			pupIDs = append(pupIDs, name[:len(name)-5]) // Remove .json extension
		}
	}

	return pupIDs, nil
}

// CleanOldSnapshots removes snapshots older than the specified duration
func (sm *SnapshotManager) CleanOldSnapshots(maxAge time.Duration) (int, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	snapshotPath := sm.getSnapshotsPath()
	entries, err := os.ReadDir(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	cleanedCount := 0
	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(snapshotPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var snapshot dogeboxd.PupVersionSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		if snapshot.SnapshotDate.Before(cutoff) {
			if err := os.Remove(filePath); err == nil {
				cleanedCount++
				log.Printf("Cleaned old snapshot: %s (created %s)", entry.Name(), snapshot.SnapshotDate)
			}
		}
	}

	return cleanedCount, nil
}

