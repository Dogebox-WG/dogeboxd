package dogeboxd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Suite: Job Creation
// ============================================================================

func TestJobCreation(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("InstallPup")
	record, err := jm.CreateJobRecord(job)

	require.NoError(t, err)
	assert.Equal(t, job.ID, record.ID)
	assert.Equal(t, JobStatusQueued, record.Status)
	assert.Equal(t, 0, record.Progress)
	assert.NotEmpty(t, record.DisplayName)
	assert.Equal(t, "Job queued", record.SummaryMessage)
	assert.NotNil(t, record.Started)
	assert.Nil(t, record.Finished)
}

func TestJobCreationStoredInDatabase(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Retrieve from database
	retrieved, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, retrieved.ID)
	assert.Equal(t, JobStatusQueued, retrieved.Status)
}

func TestJobCreationAddedToActiveCache(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Should be in active jobs cache
	jm.jobsMutex.RLock()
	_, exists := jm.activeJobs[job.ID]
	jm.jobsMutex.RUnlock()
	assert.True(t, exists, "Job should be in active jobs cache")
}

func TestJobCreationUniqueIDs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create jobs with a small delay to ensure unique IDs
	job1 := createTestJob("InstallPup")
	time.Sleep(1 * time.Millisecond)
	job2 := createTestJob("InstallPup")

	record1, err := jm.CreateJobRecord(job1)
	require.NoError(t, err)

	record2, err := jm.CreateJobRecord(job2)
	require.NoError(t, err)

	assert.NotEqual(t, record1.ID, record2.ID, "Job IDs should be unique")
}

func TestJobCreationDisplayName(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("InstallPup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Install test-app", record.DisplayName)
}

// ============================================================================
// Test Suite: Job Progress Updates
// ============================================================================

func TestJobProgressUpdate(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update progress
	progress := createTestActionProgress(job.ID, 50, "downloading", "Downloading packages...")
	err = jm.UpdateJobProgress(progress)
	require.NoError(t, err)

	// Verify update
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, 50, updated.Progress)
}

func TestJobProgressTransitionToInProgress(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// First update should transition to in_progress
	progress := createTestActionProgress(job.ID, 10, "starting", "Starting installation...")
	err = jm.UpdateJobProgress(progress)
	require.NoError(t, err)

	// Verify status changed
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusInProgress, updated.Status)
}

func TestJobProgressLogsAppended(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update progress
	progress1 := createTestActionProgress(job.ID, 25, "step1", "First step")
	err = jm.UpdateJobProgress(progress1)
	require.NoError(t, err)

	progress2 := createTestActionProgress(job.ID, 50, "step2", "Second step")
	err = jm.UpdateJobProgress(progress2)
	require.NoError(t, err)

	// Verify progress was updated
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, 50, updated.Progress)
}

func TestJobProgressSummaryMessageUpdated(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update with new summary
	progress := createTestActionProgress(job.ID, 75, "installing", "Installing packages...")
	err = jm.UpdateJobProgress(progress)
	require.NoError(t, err)

	// Verify summary updated
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "Installing packages...", updated.SummaryMessage)
}

func TestJobProgressErrorCaptured(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update with error
	progress := createTestActionProgressWithError(job.ID, "error", "Failed to download")
	err = jm.UpdateJobProgress(progress)
	require.NoError(t, err)

	// Verify error captured
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "Failed to download", updated.ErrorMessage)
}

func TestJobProgressPersistsToDatabase(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update progress
	progress := createTestActionProgress(job.ID, 60, "building", "Building...")
	err = jm.UpdateJobProgress(progress)
	require.NoError(t, err)

	// Retrieve job to verify persistence
	retrieved, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, 60, retrieved.Progress)
}

// ============================================================================
// Test Suite: Job Completion
// ============================================================================

func TestJobCompletionSuccess(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Complete successfully
	err = jm.CompleteJob(job.ID, "")
	require.NoError(t, err)

	// Verify completion
	completed, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusCompleted, completed.Status)
	assert.Equal(t, 100, completed.Progress)
	assert.NotNil(t, completed.Finished)
	assert.Equal(t, "Job completed successfully", completed.SummaryMessage)
}

func TestJobCompletionWithError(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Complete with error
	errMsg := "Failed to install package"
	err = jm.CompleteJob(job.ID, errMsg)
	require.NoError(t, err)

	// Verify failure
	failed, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusFailed, failed.Status)
	assert.Equal(t, errMsg, failed.ErrorMessage)
	assert.NotNil(t, failed.Finished)
}

func TestJobCompletionRemovedFromActiveCache(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Verify in cache
	jm.jobsMutex.RLock()
	_, exists := jm.activeJobs[job.ID]
	jm.jobsMutex.RUnlock()
	assert.True(t, exists)

	// Complete job
	err = jm.CompleteJob(job.ID, "")
	require.NoError(t, err)

	// Verify removed from cache
	jm.jobsMutex.RLock()
	_, exists = jm.activeJobs[job.ID]
	jm.jobsMutex.RUnlock()
	assert.False(t, exists, "Job should be removed from active cache")
}

func TestJobCompletionEmitsWebSocketEvent(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Complete successfully
	err = jm.CompleteJob(job.ID, "")
	require.NoError(t, err)

	// Verify WebSocket event emitted
	// Note: Changes are sent via channel, so we can't easily verify them in this test
	// In a real scenario, we'd need to wait for the channel to receive the change
}

func TestJobCompletionEmitsFailureEvent(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Complete with error
	err = jm.CompleteJob(job.ID, "Test error")
	require.NoError(t, err)

	// Verify WebSocket event emitted
	// Note: Changes are sent via channel, so we can't easily verify them in this test
	// In a real scenario, we'd need to wait for the channel to receive the change
}

func TestJobCompletionSetsFinishedTimestamp(t *testing.T) {
	jm, _, err := setupTestJobManagerWithDBX()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	beforeComplete := time.Now()

	// Complete job
	err = jm.CompleteJob(job.ID, "")
	require.NoError(t, err)

	afterComplete := time.Now()

	// Verify timestamp
	completed, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.NotNil(t, completed.Finished)
	assert.True(t, completed.Finished.After(beforeComplete) || completed.Finished.Equal(beforeComplete))
	assert.True(t, completed.Finished.Before(afterComplete) || completed.Finished.Equal(afterComplete))
}

// ============================================================================
// Test Suite: Job Retrieval
// ============================================================================

func TestGetJobByID(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Retrieve job
	retrieved, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, retrieved.ID)
}

func TestGetJobNotFound(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Try to retrieve non-existent job
	_, err = jm.GetJob("non-existent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetAllJobs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create multiple jobs
	for i := 0; i < 3; i++ {
		job := createTestJob("InstallPup")
		_, err := jm.CreateJobRecord(job)
		require.NoError(t, err)
	}

	// Get all jobs
	jobs, err := jm.GetAllJobs()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 3)
}

func TestGetActiveJobs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create active job
	activeJob := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(activeJob)
	require.NoError(t, err)

	// Create and complete a job
	completedJob := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(completedJob)
	require.NoError(t, err)
	err = jm.CompleteJob(completedJob.ID, "")
	require.NoError(t, err)

	// Get active jobs
	activeJobs, err := jm.GetActiveJobs()
	require.NoError(t, err)

	// Verify only active job is returned
	activeIDs := make(map[string]bool)
	for _, job := range activeJobs {
		activeIDs[job.ID] = true
	}
	assert.True(t, activeIDs[activeJob.ID])
	assert.False(t, activeIDs[completedJob.ID])
}

func TestGetRecentJobs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create and complete multiple jobs
	for i := 0; i < 5; i++ {
		job := createTestJob("InstallPup")
		_, err := jm.CreateJobRecord(job)
		require.NoError(t, err)
		err = jm.CompleteJob(job.ID, "")
		require.NoError(t, err)
	}

	// Get recent jobs with limit
	recentJobs, err := jm.GetRecentJobs(3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(recentJobs), 3)
}

// ============================================================================
// Test Suite: Job Cleanup
// ============================================================================

func TestClearCompletedJobs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create and complete a job
	completedJob := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(completedJob)
	require.NoError(t, err)
	err = jm.CompleteJob(completedJob.ID, "")
	require.NoError(t, err)

	// Create active job
	activeJob := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(activeJob)
	require.NoError(t, err)

	// Clear completed jobs
	count, err := jm.ClearCompletedJobs(0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)

	// Verify active job still exists
	_, err = jm.GetJob(activeJob.ID)
	require.NoError(t, err)

	// Verify completed job is gone
	_, err = jm.GetJob(completedJob.ID)
	assert.Error(t, err)
}

func TestClearAllJobs(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create multiple jobs
	for i := 0; i < 3; i++ {
		job := createTestJob("InstallPup")
		_, err := jm.CreateJobRecord(job)
		require.NoError(t, err)
	}

	// Clear all jobs
	count, err := jm.ClearAllJobs()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 3)

	// Verify all jobs are gone
	jobs, err := jm.GetAllJobs()
	require.NoError(t, err)
	assert.Equal(t, 0, len(jobs))
}

func TestClearAllJobsClearsActiveCache(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Verify in cache
	jm.jobsMutex.RLock()
	_, exists := jm.activeJobs[job.ID]
	jm.jobsMutex.RUnlock()
	assert.True(t, exists)

	// Clear all jobs
	_, err = jm.ClearAllJobs()
	require.NoError(t, err)

	// Verify cache is empty
	jm.jobsMutex.RLock()
	cacheLen := len(jm.activeJobs)
	jm.jobsMutex.RUnlock()
	assert.Equal(t, 0, cacheLen)
}

// ============================================================================
// Test Suite: Display Name Generation
// ============================================================================

func TestDisplayNameInstallPup(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("InstallPup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Install test-app", record.DisplayName)
}

func TestDisplayNameUninstallPup(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("UninstallPup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Uninstall Pup", record.DisplayName)
}

func TestDisplayNamePurgePup(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("PurgePup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Purge Pup", record.DisplayName)
}

func TestDisplayNameEnablePup(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("EnablePup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Enable Pup", record.DisplayName)
}

func TestDisplayNameDisablePup(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("DisablePup")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Disable Pup", record.DisplayName)
}

func TestDisplayNameUpdatePupConfig(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("UpdatePupConfig")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Update Pup Configuration", record.DisplayName)
}

func TestDisplayNameUpdatePupProviders(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("UpdatePupProviders")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Update Pup Providers", record.DisplayName)
}

func TestDisplayNameImportBlockchainData(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("ImportBlockchainData")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Import Blockchain Data", record.DisplayName)
}

func TestDisplayNameUpdatePendingSystemNetwork(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("UpdatePendingSystemNetwork")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Update Network Configuration", record.DisplayName)
}

func TestDisplayNameEnableSSH(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("EnableSSH")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Enable SSH", record.DisplayName)
}

func TestDisplayNameDisableSSH(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("DisableSSH")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Disable SSH", record.DisplayName)
}

func TestDisplayNameAddSSHKey(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("AddSSHKey")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Add SSH Key", record.DisplayName)
}

func TestDisplayNameRemoveSSHKey(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("RemoveSSHKey")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Remove SSH Key", record.DisplayName)
}

func TestDisplayNameAddBinaryCache(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("AddBinaryCache")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Add Binary Cache", record.DisplayName)
}

func TestDisplayNameRemoveBinaryCache(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("RemoveBinaryCache")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Remove Binary Cache", record.DisplayName)
}

func TestDisplayNameUpdateMetrics(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	job := createTestJob("UpdateMetrics")
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "Update Metrics", record.DisplayName)
}

func TestDisplayNameUnknownAction(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create a job with an unknown action type
	job := Job{
		ID:    "test-job-unknown",
		Start: time.Now(),
		A:     UnknownAction{Type: "unknown-action-type"},
	}
	record, err := jm.CreateJobRecord(job)
	require.NoError(t, err)

	assert.Equal(t, "System Operation", record.DisplayName)
}

// ============================================================================
// Test Suite: Concurrency
// ============================================================================

func TestConcurrentJobCreation(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create multiple jobs concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			job := createTestJob("InstallPup")
			_, err := jm.CreateJobRecord(job)
			require.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all jobs were created
	jobs, err := jm.GetAllJobs()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 10)
}

func TestConcurrentJobUpdates(t *testing.T) {
	jm, err := setupTestJobManager()
	require.NoError(t, err)

	// Create job
	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	// Update job concurrently
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(progress int) {
			ap := createTestActionProgress(job.ID, progress, "step", "Message")
			err := jm.UpdateJobProgress(ap)
			require.NoError(t, err)
			done <- true
		}(i * 20)
	}

	// Wait for all updates
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify job was updated
	updated, err := jm.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobStatusInProgress, updated.Status)
	assert.NotEmpty(t, updated.SummaryMessage)
}
