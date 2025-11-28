package dogeboxd

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// JobStatus represents the current state of a job
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusInProgress JobStatus = "in_progress"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

// JobRecord represents a persisted job for the frontend activity view
type JobRecord struct {
	ID             string     `json:"id"`
	Started        time.Time  `json:"started"`
	Finished       *time.Time `json:"finished"` // nil if not finished
	DisplayName    string     `json:"displayName"`
	Progress       int        `json:"progress"` // 0-100
	Status         JobStatus  `json:"status"`
	SummaryMessage string     `json:"summaryMessage"`
	ErrorMessage   string     `json:"errorMessage"`
	PupID          string     `json:"pupID,omitempty"` // Associated pup if applicable
}

// JobManager handles job persistence and state management
type JobManager struct {
	store      *TypeStore[JobRecord]
	activeJobs map[string]*JobRecord // in-memory cache of active jobs
	jobsMutex  sync.RWMutex
	dbx        *Dogeboxd
}

func NewJobManager(sm *StoreManager, dbx *Dogeboxd) *JobManager {
	return &JobManager{
		store:      GetTypeStore[JobRecord](sm),
		activeJobs: make(map[string]*JobRecord),
		dbx:        dbx,
	}
}

// SetDogeboxd sets the Dogeboxd reference (needed for circular dependency)
func (jm *JobManager) SetDogeboxd(dbx *Dogeboxd) {
	jm.dbx = dbx
}

// CreateJobRecord creates a new job record from a Job
func (jm *JobManager) CreateJobRecord(j Job) (*JobRecord, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	displayName := jm.getDisplayName(j)

	record := &JobRecord{
		ID:             j.ID,
		Started:        j.Start,
		Finished:       nil,
		DisplayName:    displayName,
		Progress:       0,
		Status:         JobStatusQueued,
		SummaryMessage: "Job queued",
		ErrorMessage:   "",
	}

	if j.State != nil {
		record.PupID = j.State.ID
	}

	// Store in database
	if err := jm.store.Set(j.ID, *record); err != nil {
		return nil, fmt.Errorf("failed to store job record: %w", err)
	}

	// Add to active jobs cache
	jm.activeJobs[j.ID] = record

	return record, nil
}

// UpdateJobProgress updates job progress from ActionProgress
func (jm *JobManager) UpdateJobProgress(ap ActionProgress) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[ap.ActionID]
	if !ok {
		// Try to load from store
		var err error
		recordValue, err := jm.store.Get(ap.ActionID)
		if err != nil {
			return fmt.Errorf("job not found: %s", ap.ActionID)
		}
		record = &recordValue
		jm.activeJobs[ap.ActionID] = record
	}

	// Update progress
	if ap.Progress > 0 {
		record.Progress = ap.Progress
	}

	// Update status - move to in_progress as soon as job starts sending updates
	if record.Status == JobStatusQueued {
		record.Status = JobStatusInProgress
	}

	// Update summary message
	record.SummaryMessage = ap.Msg

	// Handle errors
	if ap.Error {
		record.ErrorMessage = ap.Msg
	}

	// Persist to database
	return jm.store.Set(record.ID, *record)
}

// CompleteJob marks a job as completed
func (jm *JobManager) CompleteJob(jobID string, err string) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[jobID]
	if !ok {
		// Try to load from store
		recordValue, loadErr := jm.store.Get(jobID)
		if loadErr != nil {
			return fmt.Errorf("job not found: %s", jobID)
		}
		record = &recordValue
	}

	now := time.Now()
	record.Finished = &now

	if err != "" {
		record.Status = JobStatusFailed
		record.ErrorMessage = err
		// Progress stays at current value
		record.SummaryMessage = "Job failed"
	} else {
		record.Status = JobStatusCompleted
		record.Progress = 100
		record.SummaryMessage = "Job completed successfully"
	}

	// Remove from active jobs
	delete(jm.activeJobs, jobID)

	// Persist to database
	storeErr := jm.store.Set(record.ID, *record)
	if storeErr != nil {
		return storeErr
	}

	// Emit WebSocket event for job completion
	if jm.dbx != nil {
		eventType := "job:completed"
		if err != "" {
			eventType = "job:failed"
		}
		jm.dbx.sendChange(Change{ID: "internal", Type: eventType, Update: record})
	}

	return nil
}

// GetJob retrieves a job record by ID
func (jm *JobManager) GetJob(jobID string) (*JobRecord, error) {
	jm.jobsMutex.RLock()
	defer jm.jobsMutex.RUnlock()

	// Check active jobs first
	if record, ok := jm.activeJobs[jobID]; ok {
		return record, nil
	}

	// Load from store
	record, err := jm.store.Get(jobID)
	if err != nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	return &record, nil
}

// IsJobActive returns true if the job is in the active jobs cache (not yet completed)
// Used to avoid duplicate CompleteJob calls
func (jm *JobManager) IsJobActive(jobID string) bool {
	jm.jobsMutex.RLock()
	defer jm.jobsMutex.RUnlock()
	_, ok := jm.activeJobs[jobID]
	return ok
}

// GetAllJobs retrieves all job records
func (jm *JobManager) GetAllJobs() ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s ORDER BY json_extract(value, '$.started') DESC", jm.store.Table)
	return jm.store.Exec(query)
}

// GetActiveJobs retrieves all jobs that are queued or in progress
func (jm *JobManager) GetActiveJobs() ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s WHERE json_extract(value, '$.status') IN ('queued', 'in_progress') ORDER BY json_extract(value, '$.started') ASC", jm.store.Table)
	return jm.store.Exec(query)
}

// GetRecentJobs retrieves recent completed/failed jobs
func (jm *JobManager) GetRecentJobs(limit int) ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s WHERE json_extract(value, '$.status') IN ('completed', 'failed', 'cancelled') ORDER BY json_extract(value, '$.finished') DESC LIMIT %d", jm.store.Table, limit)
	return jm.store.Exec(query)
}

// ClearCompletedJobs removes completed/failed jobs older than the specified duration
func (jm *JobManager) ClearCompletedJobs(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339Nano)
	query := fmt.Sprintf(`DELETE FROM %s 
		WHERE json_extract(value, '$.status') IN ('completed', 'failed', 'cancelled')
		  AND json_extract(value, '$.finished') IS NOT NULL
		  AND json_extract(value, '$.finished') < ?`, jm.store.Table)

	count, err := jm.store.ExecWrite(query, cutoff)
	return int(count), err
}

// ClearAllJobs removes ALL jobs (for development/cleanup)
func (jm *JobManager) ClearAllJobs() (int, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	query := fmt.Sprintf("DELETE FROM %s", jm.store.Table)
	count, err := jm.store.ExecWrite(query)
	if err != nil {
		return 0, err
	}

	// Clear active jobs cache
	jm.activeJobs = make(map[string]*JobRecord)

	return int(count), nil
}

// ClearOrphanedJobs marks jobs stuck in queued/in_progress state as failed
// Jobs are considered orphaned if they've been queued for longer than the threshold
func (jm *JobManager) ClearOrphanedJobs(olderThan time.Duration) (int, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	cutoff := time.Now().Add(-olderThan)
	now := time.Now()

	count, err := jm.markOrphanedJobsAsFailed(now, cutoff)
	if err != nil {
		return 0, err
	}

	// Clear active jobs cache for these orphaned jobs
	for id, job := range jm.activeJobs {
		if job.Status == JobStatusQueued || job.Status == JobStatusInProgress {
			if job.Started.Before(cutoff) {
				delete(jm.activeJobs, id)
			}
		}
	}

	return count, nil
}

func (jm *JobManager) markOrphanedJobsAsFailed(finished time.Time, startedBefore time.Time) (int, error) {
	query := fmt.Sprintf(`UPDATE %s SET value = json_set(json_set(json_set(value, '$.status', 'failed'), '$.errorMessage', 'Job was orphaned (stuck in queue)'), '$.finished', ?) WHERE json_extract(value, '$.status') IN ('queued', 'in_progress') AND json_extract(value, '$.started') < ?`, jm.store.Table)
	count, err := jm.store.ExecWrite(query, finished.Format(time.RFC3339Nano), startedBefore.Format(time.RFC3339Nano))
	return int(count), err
}

// getDisplayName returns a human-readable name for the job
func (jm *JobManager) getDisplayName(j Job) string {
	switch a := j.A.(type) {
	case InstallPup:
		return fmt.Sprintf("Install %s", a.PupName)
	case InstallPups:
		if len(a) == 1 {
			return fmt.Sprintf("Install %s", a[0].PupName)
		}
		return fmt.Sprintf("Install %d Pups", len(a))
	case UninstallPup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Uninstall %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Uninstall %s", pup.Manifest.Meta.Name)
			}
		}
		return "Uninstall Pup"
	case PurgePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Purge %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Purge %s", pup.Manifest.Meta.Name)
			}
		}
		return "Purge Pup"
	case EnablePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Enable %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Enable %s", pup.Manifest.Meta.Name)
			}
		}
		return "Enable Pup"
	case DisablePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Disable %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Disable %s", pup.Manifest.Meta.Name)
			}
		}
		return "Disable Pup"
	case UpdatePupConfig:
		return "Update Pup Configuration"
	case UpdatePupProviders:
		return "Update Pup Providers"
	case ImportBlockchainData:
		return "Import Blockchain Data"
	case UpdatePendingSystemNetwork:
		return "Update Network Configuration"
	case EnableSSH:
		return "Enable SSH"
	case DisableSSH:
		return "Disable SSH"
	case AddSSHKey:
		return "Add SSH Key"
	case RemoveSSHKey:
		return "Remove SSH Key"
	case AddBinaryCache:
		return "Add Binary Cache"
	case RemoveBinaryCache:
		return "Remove Binary Cache"
	case SystemUpdate:
		return "System Update"
	case UpdateMetrics:
		return "Update Metrics"
	default:
		return "System Operation"
	}
}

// SyncWithActiveJobs ensures all jobs in the queue are tracked
func (jm *JobManager) SyncWithActiveJobs() error {
	activeJobs, err := jm.GetActiveJobs()
	if err != nil {
		return err
	}

	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	// Refresh active jobs cache
	jm.activeJobs = make(map[string]*JobRecord, len(activeJobs))
	for i := range activeJobs {
		jm.activeJobs[activeJobs[i].ID] = &activeJobs[i]
	}

	return nil
}

// JobsUpdate represents a change to the jobs list for WebSocket updates
type JobsUpdate struct {
	Jobs []JobRecord `json:"jobs"`
}

// MarshalJSON custom marshaler to handle the Update interface
func (ju JobsUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Jobs []JobRecord `json:"jobs"`
	}{
		Jobs: ju.Jobs,
	})
}
