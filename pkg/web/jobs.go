package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// Get all jobs
func (t api) getJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := t.dbx.JobManager.GetAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Get a single job by ID
func (t api) getJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}

	job, err := t.dbx.JobManager.GetJob(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Job not found")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"job":     job,
	})
}

// Get active jobs (queued or in progress)
func (t api) getActiveJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := t.dbx.JobManager.GetActiveJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve active jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Get recent completed/failed jobs
func (t api) getRecentJobs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	jobs, err := t.dbx.JobManager.GetRecentJobs(limit)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve recent jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Mark a job as read
func (t api) markJobAsRead(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}

	err := t.dbx.JobManager.MarkJobAsRead(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to mark job as read")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
	})
}

// Mark all completed/failed jobs as read
func (t api) markAllJobsAsRead(w http.ResponseWriter, r *http.Request) {
	err := t.dbx.JobManager.MarkAllJobsAsRead()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to mark jobs as read")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
	})
}

// Clear old completed jobs
func (t api) clearCompletedJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OlderThanDays int `json:"olderThanDays"`
	}

	// Try to decode body, but don't fail if empty
	_ = json.NewDecoder(r.Body).Decode(&req)

	// If olderThanDays is 0 or negative, clear ALL completed jobs
	if req.OlderThanDays <= 0 {
		req.OlderThanDays = 0 // Clear all completed jobs regardless of age
	}

	olderThan := time.Duration(req.OlderThanDays) * 24 * time.Hour
	count, err := t.dbx.JobManager.ClearCompletedJobs(olderThan)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to clear jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"cleared": count,
	})
}

// Get job statistics
func (t api) getJobStats(w http.ResponseWriter, r *http.Request) {
	allJobs, err := t.dbx.JobManager.GetAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job stats")
		return
	}

	stats := map[string]int{
		"total":      len(allJobs),
		"queued":     0,
		"inProgress": 0,
		"completed":  0,
		"failed":     0,
		"cancelled":  0,
		"unread":     0,
	}

	for _, job := range allJobs {
		switch job.Status {
		case dogeboxd.JobStatusQueued:
			stats["queued"]++
		case dogeboxd.JobStatusInProgress:
			stats["inProgress"]++
		case dogeboxd.JobStatusCompleted:
			stats["completed"]++
		case dogeboxd.JobStatusFailed:
			stats["failed"]++
		case dogeboxd.JobStatusCancelled:
			stats["cancelled"]++
		}

		if !job.Read && (job.Status == dogeboxd.JobStatusCompleted ||
			job.Status == dogeboxd.JobStatusFailed ||
			job.Status == dogeboxd.JobStatusCancelled) {
			stats["unread"]++
		}
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// Clear ALL jobs (development/cleanup endpoint)
func (t api) clearAllJobs(w http.ResponseWriter, r *http.Request) {
	count, err := t.dbx.JobManager.ClearAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to clear all jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"cleared": count,
		"message": fmt.Sprintf("Cleared all %d jobs", count),
	})
}
