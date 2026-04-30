package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
)

type CommenceUpdateRequest struct {
	Package string `json:"package"`
	Version string `json:"version"`
	OSRef   string `json:"osRef,omitempty"`
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

func buildPackageInfo(currentVersion string, releases []system.UpgradableRelease) PackageInfo {
	updates := make([]UpgradableRelease, len(releases))
	for i, release := range releases {
		updates[i] = UpgradableRelease{
			Version:    release.Version,
			ReleaseURL: release.ReleaseURL,
			Summary:    release.Summary,
		}
	}

	return PackageInfo{
		Name:           "Dogebox",
		CurrentVersion: currentVersion,
		LatestUpdate:   releases[0].Version,
		Updates:        updates,
	}
}

func buildSyntheticOSRefUpdate(currentVersion string, osRef string) PackageInfo {
	shortRef := osRef
	if len(shortRef) > 12 {
		shortRef = shortRef[:12]
	}
	shortRef = strings.NewReplacer("/", "-", " ", "-").Replace(shortRef)
	syntheticVersion := fmt.Sprintf("%s-osref.%s", currentVersion, shortRef)

	return PackageInfo{
		Name:           "Dogebox",
		CurrentVersion: currentVersion,
		LatestUpdate:   syntheticVersion,
		Updates: []UpgradableRelease{
			{
				Version:    syntheticVersion,
				ReleaseURL: fmt.Sprintf("https://github.com/dogebox-wg/os/commit/%s", osRef),
				Summary:    fmt.Sprintf("Developer OS upgrade from ref %s", osRef),
			},
		},
	}
}

func (t api) checkForUpdates(w http.ResponseWriter, r *http.Request) {
	// Parse query parameter for including pre-releases
	includePreReleases := r.URL.Query().Get("includePreReleases") == "true"
	osRef := strings.TrimSpace(r.URL.Query().Get("osRef"))
	currentVersion := version.GetDBXRelease().Release

	packages := make(map[string]PackageInfo)

	if osRef != "" {
		packages["dogebox"] = buildSyntheticOSRefUpdate(currentVersion, osRef)
		sendResponse(w, UpdatesResponse{Packages: packages})
		return
	}

	releases, err := system.GetUpgradableReleases(includePreReleases)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error checking for updates")
		return
	}

	if len(releases) > 0 {
		packages["dogebox"] = buildPackageInfo(currentVersion, releases)
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

	id := t.dbx.AddAction(dogeboxd.SystemUpdate{
		Package: packageName,
		Version: req.Version,
		OSRef:   req.OSRef,
	})

	sendResponse(w, map[string]any{
		"success": true,
		"id":      id,
	})
}
