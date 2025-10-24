package web

import (
	"log"
	"net/http"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// GET /pup/updates - Get all available pup updates
func (t api) getAllPupUpdates(w http.ResponseWriter, r *http.Request) {
	updates := t.dbx.PupUpdateChecker.GetAllCachedUpdates()

	log.Printf("getAllPupUpdates: returning %d cached update(s)", len(updates))
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

	log.Printf("getPupUpdates: returning update info for pup %s (updateAvailable: %v)", pupID, updateInfo.UpdateAvailable)
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

// NOTE: The following endpoints are stubs for update execution functionality.
// Current scope focuses on update detection and notification only.
// These will be fully implemented in a future phase when we add actual update execution.

// POST /pup/:pupId/update - Trigger pup update (NOT YET IMPLEMENTED)
func (t api) updatePup(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement update execution in future phase
	log.Printf("updatePup endpoint called but not yet implemented - update execution deferred to future phase")
	sendErrorResponse(w, http.StatusNotImplemented, "Pup update execution not yet implemented - currently only supports update detection and notification")
}

// POST /pup/:pupId/rollback - Rollback to previous version (NOT YET IMPLEMENTED)
func (t api) rollbackPup(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement rollback in future phase
	log.Printf("rollbackPup endpoint called but not yet implemented - rollback functionality deferred to future phase")
	sendErrorResponse(w, http.StatusNotImplemented, "Pup rollback not yet implemented - currently only supports update detection and notification")
}

// GET /pup/:pupId/previous-version - Get previous version snapshot (NOT YET IMPLEMENTED)
func (t api) getPreviousVersion(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement version history in future phase
	log.Printf("getPreviousVersion endpoint called but not yet implemented - version history deferred to future phase")
	sendErrorResponse(w, http.StatusNotImplemented, "Version history not yet implemented - currently only supports update detection and notification")
}
