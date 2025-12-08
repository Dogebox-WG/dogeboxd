package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// GET /pup/updates - Get all available pup updates
func (t api) getAllPupUpdates(w http.ResponseWriter, r *http.Request) {
	updates := t.dbx.PupUpdateChecker.GetAllCachedUpdates()

	sendResponse(w, updates)
}

// GET /pup/:pupId/updates - Get updates for specific pup
func (t api) getPupUpdates(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/updates")

	updateInfo, ok := t.dbx.PupUpdateChecker.GetCachedUpdateInfo(pupID)
	if !ok {
		// No cached update info - return empty update info (not an error, just no data yet)
		updateInfo = dogeboxd.PupUpdateInfo{
			PupID:             pupID,
			UpdateAvailable:   false,
			AvailableVersions: []dogeboxd.PupVersion{},
		}
	}

	sendResponse(w, updateInfo)
}

// POST /pup/:pupId/check-pup-updates - Force check for pup updates
func (t api) checkPupUpdates(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/check-pup-updates")

	// Convert "all" to empty string (which means check all pups)
	if pupID == "all" {
		pupID = ""
	}

	// Trigger update check action
	jobID := t.dbx.AddAction(dogeboxd.CheckPupUpdates{
		PupID: pupID,
	})

	sendResponse(w, map[string]string{"jobId": jobID})
}

// UpgradePupRequest is the request body for the upgrade endpoint
type UpgradePupRequest struct {
	TargetVersion string `json:"targetVersion"`
}

// POST /pup/:pupId/upgrade - Trigger pup upgrade
func (t api) upgradePup(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/upgrade")

	// Parse request body
	var req UpgradePupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TargetVersion == "" {
		sendErrorResponse(w, http.StatusBadRequest, "targetVersion is required")
		return
	}

	// Get the pup to find its source
	pup, _, err := t.pups.GetPup(pupID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Pup not found")
		return
	}

	// Verify the target version is available
	updateInfo, ok := t.dbx.PupUpdateChecker.GetCachedUpdateInfo(pupID)
	if !ok {
		sendErrorResponse(w, http.StatusBadRequest, "No update information available. Check for updates first.")
		return
	}

	// Check that target version exists in available versions
	versionFound := false
	for _, v := range updateInfo.AvailableVersions {
		if v.Version == req.TargetVersion {
			versionFound = true
			break
		}
	}
	if !versionFound {
		sendErrorResponse(w, http.StatusBadRequest, "Target version not available")
		return
	}

	// Trigger upgrade action
	jobID := t.dbx.AddAction(dogeboxd.UpgradePup{
		PupID:         pupID,
		TargetVersion: req.TargetVersion,
		SourceId:      pup.Source.ID,
	})

	log.Printf("upgradePup: triggered upgrade for pup %s to version %s (jobId: %s)", pupID, req.TargetVersion, jobID)
	sendResponse(w, map[string]string{"jobId": jobID})
}

// POST /pup/:pupId/rollback - Rollback to previous version
func (t api) rollbackPup(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/rollback")

	// Verify pup exists
	_, _, err := t.pups.GetPup(pupID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Pup not found")
		return
	}

	// Trigger rollback action
	jobID := t.dbx.AddAction(dogeboxd.RollbackPupUpgrade{
		PupID: pupID,
	})

	log.Printf("rollbackPup: triggered rollback for pup %s (jobId: %s)", pupID, jobID)
	sendResponse(w, map[string]string{"jobId": jobID})
}

// GET /pup/:pupId/previous-version - Get previous version snapshot
func (t api) getPreviousVersion(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/previous-version")

	// Return null if no snapshot system available or no snapshot exists
	// For now, we don't have direct access to the snapshot manager from the API
	// So we'll return a simple response indicating if rollback might be available
	// The actual snapshot check happens in the rollback handler
	
	// Verify pup exists
	pup, _, err := t.pups.GetPup(pupID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Pup not found")
		return
	}

	// Check if pup is in broken state (which means rollback is likely needed)
	response := map[string]interface{}{
		"pupId":            pupID,
		"currentVersion":   pup.Version,
		"isBroken":         pup.Installation == dogeboxd.STATE_BROKEN,
		"brokenReason":     pup.BrokenReason,
		"rollbackPossible": pup.Installation == dogeboxd.STATE_BROKEN, // Simplified check
	}

	sendResponse(w, response)
}

// Backward compatibility - the old updatePup route now redirects to upgradePup
func (t api) updatePup(w http.ResponseWriter, r *http.Request) {
	// Redirect to the new upgrade endpoint
	t.upgradePup(w, r)
}

// GET /pup/skipped-updates - Get all skipped updates
func (t api) getAllSkippedUpdates(w http.ResponseWriter, r *http.Request) {
	skipped := t.dbx.SkippedUpdatesManager.GetAllSkipped()
	sendResponse(w, skipped)
}

// POST /pup/:pupId/skip-update - Skip updates for a specific pup
func (t api) skipPupUpdate(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/skip-update")

	// Get the pup to find its current version
	pup, _, err := t.pups.GetPup(pupID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Pup not found")
		return
	}

	// Get update info to find the latest version
	updateInfo, ok := t.dbx.PupUpdateChecker.GetCachedUpdateInfo(pupID)
	if !ok || !updateInfo.UpdateAvailable {
		sendErrorResponse(w, http.StatusBadRequest, "No update available to skip")
		return
	}

	// Skip the update
	err = t.dbx.SkippedUpdatesManager.SkipUpdate(pupID, pup.Version, updateInfo.LatestVersion)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to skip update")
		return
	}

	log.Printf("skipPupUpdate: skipped updates for pup %s up to version %s", pupID, updateInfo.LatestVersion)
	sendResponse(w, map[string]string{"status": "success"})
}

// DELETE /pup/:pupId/skip-update - Clear skip status for a specific pup
func (t api) clearSkippedUpdate(w http.ResponseWriter, r *http.Request) {
	pupID := strings.TrimPrefix(r.URL.Path, "/pup/")
	pupID = strings.TrimSuffix(pupID, "/skip-update")

	// Clear the skip status
	err := t.dbx.SkippedUpdatesManager.ClearSkipped(pupID)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to clear skip status")
		return
	}

	log.Printf("clearSkippedUpdate: cleared skip status for pup %s", pupID)
	sendResponse(w, map[string]string{"status": "success"})
}
