package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

type CreateSourceRequest struct {
	Location string `json:"location"`
}

func (t api) createSource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req CreateSourceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error parsing payload")
		return
	}

	if _, err := t.sources.AddSource(req.Location); err != nil {
		if isPendingEligibleSourceLocation(req.Location) && internetOffline(t.dbx.NetworkManager) {
			log.Printf("Unable to verify source %q while offline, saving as pending: %v", req.Location, err)
			if err := t.sources.AddSourcePending(req.Location); err != nil {
				log.Printf("Error adding pending source: %v", err)
				sendErrorResponse(w, http.StatusInternalServerError, "Error adding source")
				return
			}

			sendResponse(w, map[string]any{
				"success": true,
				"pending": true,
			})
			return
		}

		log.Printf("Error adding source: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error adding source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}

func isPendingEligibleSourceLocation(location string) bool {
	return (strings.HasPrefix(location, "https://") && strings.HasSuffix(location, ".git")) ||
		strings.HasPrefix(location, "git@")
}

func internetOffline(networkManager dogeboxd.NetworkManager) bool {
	if networkManager == nil {
		return false
	}

	return !networkManager.HasInternetConnectivity()
}

func (t api) deleteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if id == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Missing source id")
		return
	}

	if err := t.sources.RemoveSource(id); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error deleting source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}
