package web

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/version"
)

type CommenceUpdateRequest struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type UpgradableRelease struct {
	Version    string `json:"version"`
	ReleaseURL string `json:"releaseURL"`
	Summary    string `json:"summary"`
}

type PackageInfo struct {
	Name           string              `json:"name"`
	CurrentVersion string              `json:"currentVersion"`
	LatestUpdate   string              `json:"latestUpdate"`
	Updates        []UpgradableRelease `json:"updates"`
}

type UpdatesResponse struct {
	Packages map[string]PackageInfo `json:"packages"`
}

func (t api) checkForUpdates(w http.ResponseWriter, r *http.Request) {
	releases, err := system.GetUpgradableReleases()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error checking for updates")
		return
	}

	// Convert to the expected format
	packages := make(map[string]PackageInfo)

	if len(releases) > 0 {
		// Get current version from the system
		currentVersion := version.GetDBXRelease().Release
		latestVersion := releases[0].Version

		// Convert system.UpgradableRelease to our local UpgradableRelease
		updates := make([]UpgradableRelease, len(releases))
		for i, release := range releases {
			updates[i] = UpgradableRelease{
				Version:    release.Version,
				ReleaseURL: release.ReleaseURL,
				Summary:    release.Summary,
			}
		}

		packages["dogebox"] = PackageInfo{
			Name:           "Dogebox",
			CurrentVersion: currentVersion,
			LatestUpdate:   latestVersion,
			Updates:        updates,
		}
	}

	response := UpdatesResponse{
		Packages: packages,
	}

	sendResponse(w, response)
}

func (t api) commenceUpdate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req CommenceUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	if req.Package == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Package is required")
		return
	}

	if req.Version == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Version is required")
		return
	}

	// Convert "dogebox" package to "os" as expected by the underlying function
	packageName := req.Package
	if packageName == "dogebox" {
		packageName = "os"
	}

	// Execute the system update
	err = system.DoSystemUpdate(packageName, req.Version)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error executing update: "+err.Error())
		return
	}

	sendResponse(w, map[string]bool{"success": true})
}
