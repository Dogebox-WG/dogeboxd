package web

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Dogebox-WG/dogeboxd/pkg/system"
)

type InstallToDiskRequest struct {
	Disk   string `json:"disk"`
	Secret string `json:"secret"`
}

func (t api) getSystemDisks(w http.ResponseWriter, r *http.Request) {
	log.Printf("GET /system/disks: remote=%s authHeaderPresent=%t", r.RemoteAddr, r.Header.Get("Authorization") != "")
	disks, err := system.GetSystemDisks()
	if err != nil {
		log.Printf("GET /system/disks: failed to enumerate disks: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("GET /system/disks: returning %d disks", len(disks))
	sendResponse(w, disks)
}

func (t api) getInstallDisks(w http.ResponseWriter, r *http.Request) {
	log.Printf("GET /system/install-disks: remote=%s authHeaderPresent=%t", r.RemoteAddr, r.Header.Get("Authorization") != "")
	disks, err := system.GetInstallTargetDisks()
	if err != nil {
		log.Printf("GET /system/install-disks: failed to enumerate install disks: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("GET /system/install-disks: returning %d install disks", len(disks))
	sendResponse(w, disks)
}

func (t api) installToDisk(w http.ResponseWriter, r *http.Request) {
	var req InstallToDiskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON: "+err.Error())
		return
	}

	if req.Disk == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Disk is required")
		return
	}

	if req.Secret == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Secret is required")
		return
	}

	if req.Secret != system.DBXRootSecret {
		sendErrorResponse(w, http.StatusForbidden, "Invalid secret")
		return
	}

	dbxState := t.sm.Get().Dogebox

	if err := system.InstallToDisk(t.dbx, t.config, dbxState, req.Disk); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error installing to disk: "+err.Error())
		return
	}

	sendResponse(w, map[string]string{"status": "ok"})
}
