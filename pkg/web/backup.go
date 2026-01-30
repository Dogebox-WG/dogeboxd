package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

type BackupRequest struct {
	Target          string `json:"target"`
	DestinationPath string `json:"destinationPath"`
}

type RestoreRequest struct {
	SourcePath string `json:"sourcePath"`
}

func (t api) getRemovableMounts(w http.ResponseWriter, r *http.Request) {
	mounts, err := system.GetRemovableMounts()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendResponse(w, map[string]any{
		"success": true,
		"mounts":  mounts,
	})
}

func (t api) startBackup(w http.ResponseWriter, r *http.Request) {
	var req BackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON: "+err.Error())
		return
	}

	if req.Target == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Target is required")
		return
	}

	target := dogeboxd.BackupTarget(req.Target)
	if target != dogeboxd.BackupTargetDownload && target != dogeboxd.BackupTargetRemovable {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid target")
		return
	}

	if target == dogeboxd.BackupTargetRemovable {
		if req.DestinationPath == "" {
			sendErrorResponse(w, http.StatusBadRequest, "Destination path is required")
			return
		}
		if err := system.ValidateRemovablePath(req.DestinationPath); err != nil {
			sendErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	id := t.dbx.AddAction(dogeboxd.BackupConfig{
		Target:          target,
		DestinationPath: req.DestinationPath,
	})

	response := map[string]any{
		"success": true,
		"id":      id,
	}
	if target == dogeboxd.BackupTargetDownload {
		response["downloadUrl"] = fmt.Sprintf("/system/backup/download/%s", id)
	}
	sendResponse(w, response)
}

func (t api) getBackupStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}
	if t.dbx.JobManager == nil {
		sendErrorResponse(w, http.StatusServiceUnavailable, "Job manager unavailable")
		return
	}

	job, err := t.dbx.JobManager.GetJob(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Job not found")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
		"job":     job,
	})
}

func (t api) downloadBackup(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}

	backupPath := filepath.Join(t.config.TmpDir, "backups", fmt.Sprintf("dogebox-backup-%s.tar.gz", jobID))
	if _, err := os.Stat(backupPath); err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Backup not found")
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=dogebox-backup-%s.tar.gz", jobID))
	http.ServeFile(w, r, backupPath)
}

func (t api) startRestore(w http.ResponseWriter, r *http.Request) {
	session, sessionOK := getSession(r, getBearerToken)
	if !sessionOK {
		sendErrorResponse(w, http.StatusUnauthorized, "Missing or invalid session")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		t.startRestoreUpload(w, r, session, sessionOK)
		return
	}

	var req RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON: "+err.Error())
		return
	}
	if req.SourcePath == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Source path is required")
		return
	}
	if err := system.ValidateRemovablePath(req.SourcePath); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	id := t.dbx.AddAction(dogeboxd.RestoreConfig{
		SourcePath:   req.SourcePath,
		SessionToken: session.DKM_TOKEN,
	})
	sendResponse(w, map[string]any{
		"success": true,
		"id":      id,
	})
}

func (t api) startRestoreUpload(w http.ResponseWriter, r *http.Request, session Session, sessionOK bool) {
	if !sessionOK {
		sendErrorResponse(w, http.StatusUnauthorized, "Missing or invalid session")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid multipart request")
		return
	}

	restoreDir := filepath.Join(t.config.TmpDir, "restore")
	if err := os.MkdirAll(restoreDir, 0755); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to create restore directory")
		return
	}

	var tempPath string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			sendErrorResponse(w, http.StatusBadRequest, "Failed to read upload")
			return
		}
		if part.FormName() != "backup" {
			continue
		}
		tempFile, err := os.CreateTemp(restoreDir, "backup-*.tar.gz")
		if err != nil {
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to create temp file")
			return
		}
		if _, err := io.Copy(tempFile, part); err != nil {
			tempFile.Close()
			os.Remove(tempFile.Name())
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to save upload")
			return
		}
		if err := tempFile.Close(); err != nil {
			os.Remove(tempFile.Name())
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to finalize upload")
			return
		}
		tempPath = tempFile.Name()
		break
	}

	if tempPath == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Backup file is required")
		return
	}

	id := t.dbx.AddAction(dogeboxd.RestoreConfig{
		SourcePath:   tempPath,
		SessionToken: session.DKM_TOKEN,
	})
	sendResponse(w, map[string]any{
		"success": true,
		"id":      id,
	})
}
