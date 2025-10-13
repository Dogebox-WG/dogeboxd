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
	Sensitive      bool       `json:"sensitive"` // Critical jobs that block others
	Progress       int        `json:"progress"`  // 0-100
	Status         JobStatus  `json:"status"`
	SummaryMessage string     `json:"summaryMessage"`
	ErrorMessage   string     `json:"errorMessage"`
	Logs           []string   `json:"logs"`
	Read           bool       `json:"read"`            // Has user viewed this job?
	PupID          string     `json:"pupID,omitempty"` // Associated pup if applicable
}

// JobManager handles job persistence and state management
type JobManager struct {
	store      *TypeStore[JobRecord]
	activeJobs map[string]*JobRecord // in-memory cache of active jobs
	mu         sync.RWMutex
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
	jm.mu.Lock()
	defer jm.mu.Unlock()

	displayName := jm.getDisplayName(j)
	sensitive := jm.isSensitive(j)

	record := &JobRecord{
		ID:             j.ID,
		Started:        j.Start,
		Finished:       nil,
		DisplayName:    displayName,
		Sensitive:      sensitive,
		Progress:       0,
		Status:         JobStatusQueued,
		SummaryMessage: "Job queued",
		ErrorMessage:   "",
		Logs:           []string{},
		Read:           false,
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
	jm.mu.Lock()
	defer jm.mu.Unlock()

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

	// Add log entry
	logEntry := fmt.Sprintf("[%s] [%s] %s", time.Now().Format("15:04:05"), ap.Step, ap.Msg)
	record.Logs = append(record.Logs, logEntry)

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
	jm.mu.Lock()
	defer jm.mu.Unlock()

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
		eventType := "activity:completed"
		if err != "" {
			eventType = "activity:failed"
		}
		jm.dbx.sendChange(Change{ID: "internal", Type: eventType, Update: record})
	}

	return nil
}

// CancelJob marks a job as cancelled
func (jm *JobManager) CancelJob(jobID string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	record, ok := jm.activeJobs[jobID]
	if !ok {
		// Try to load from store
		recordValue, err := jm.store.Get(jobID)
		if err != nil {
			return fmt.Errorf("job not found: %s", jobID)
		}
		record = &recordValue
	}

	now := time.Now()
	record.Finished = &now
	record.Status = JobStatusCancelled
	record.SummaryMessage = "Job cancelled by user"

	// Remove from active jobs
	delete(jm.activeJobs, jobID)

	// Persist to database
	storeErr := jm.store.Set(record.ID, *record)
	if storeErr != nil {
		return storeErr
	}

	// Emit WebSocket event for job cancellation
	if jm.dbx != nil {
		jm.dbx.sendChange(Change{ID: "internal", Type: "activity:cancelled", Update: record})
	}

	return nil
}

// GetJob retrieves a job record by ID
func (jm *JobManager) GetJob(jobID string) (*JobRecord, error) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

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

// MarkJobAsRead marks a job as read by the user
func (jm *JobManager) MarkJobAsRead(jobID string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	record, err := jm.store.Get(jobID)
	if err != nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	record.Read = true
	return jm.store.Set(jobID, record)
}

// MarkAllJobsAsRead marks all completed/failed jobs as read
func (jm *JobManager) MarkAllJobsAsRead() error {
	jobs, err := jm.GetAllJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if (job.Status == JobStatusCompleted || job.Status == JobStatusFailed || job.Status == JobStatusCancelled) && !job.Read {
			job.Read = true
			if err := jm.store.Set(job.ID, job); err != nil {
				return err
			}
		}
	}

	return nil
}

// ClearCompletedJobs removes completed/failed jobs older than the specified duration
func (jm *JobManager) ClearCompletedJobs(olderThan time.Duration) (int, error) {
	jobs, err := jm.GetAllJobs()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for _, job := range jobs {
		if (job.Status == JobStatusCompleted || job.Status == JobStatusFailed || job.Status == JobStatusCancelled) &&
			job.Finished != nil && job.Finished.Before(cutoff) {
			if err := jm.store.Del(job.ID); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// ClearAllJobs removes ALL jobs (for development/cleanup)
func (jm *JobManager) ClearAllJobs() (int, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	jobs, err := jm.GetAllJobs()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, job := range jobs {
		if err := jm.store.Del(job.ID); err != nil {
			return count, err
		}
		count++
	}

	// Clear active jobs cache
	jm.activeJobs = make(map[string]*JobRecord)

	return count, nil
}

// HasCriticalJobRunning checks if any sensitive job is currently running
func (jm *JobManager) HasCriticalJobRunning() (bool, *JobRecord) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	for _, record := range jm.activeJobs {
		if record.Sensitive && record.Status == JobStatusInProgress {
			return true, record
		}
	}

	return false, nil
}

// getDisplayName returns a human-readable name for the job
func (jm *JobManager) getDisplayName(j Job) string {
	switch a := j.A.(type) {
	case InstallPup:
		return fmt.Sprintf("Install %s", a.PupName)
	case InstallPups:
		if len(a) == 1 {
			return fmt.Sprintf("Install %s", a[0].PupName)
		} else if len(a) > 1 {
			return fmt.Sprintf("Install %d Pups", len(a))
		}
		return "Install Pups"
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
	case UpdateMetrics:
		return "Update Metrics"
	default:
		return "System Operation"
	}
}

// isSensitive determines if a job is critical and should block other operations
func (jm *JobManager) isSensitive(j Job) bool {
	switch j.A.(type) {
	case InstallPup, InstallPups, UninstallPup, PurgePup, EnablePup, DisablePup, UpdatePupProviders:
		return true // Pup operations that trigger system rebuilds
	case UpdatePendingSystemNetwork, ImportBlockchainData:
		return true // System-level operations
	default:
		return false
	}
}

// SyncWithActiveJobs ensures all jobs in the queue are tracked
func (jm *JobManager) SyncWithActiveJobs() error {
	activeJobs, err := jm.GetActiveJobs()
	if err != nil {
		return err
	}

	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Refresh active jobs cache
	jm.activeJobs = make(map[string]*JobRecord)
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
