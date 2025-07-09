package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t api) updateConfig(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(dogeboxd.UpdatePupConfig{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) getPupProviders(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")

	deps, err := t.pups.CalculateDeps(pupid)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Cannof find pup")
		return
	}
	sendResponse(w, deps)
}

func (t api) updateProviders(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(dogeboxd.UpdatePupProviders{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

type InstallPupRequest struct {
	PupName                 string `json:"pupName"`
	PupVersion              string `json:"pupVersion"`
	SourceId                string `json:"sourceId"`
	SessionToken            string
	AutoInstallDependencies bool `json:"autoInstallDependencies"`
}

// calculateDependencies creates a temporary pup state and calculates its dependencies
func (t api) calculateDependencies(sourceId, pupName, pupVersion string) (*dogeboxd.PupState, []dogeboxd.PupDependencyReport, error) {
	// Create a temporary pup - this will be used to calculate dependencies
	manifest, source, err := t.sources.GetSourceManifest(sourceId, pupName, pupVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't get manifest: %s", err)
	}

	// Create a temporary pup state to calculate dependencies
	tempPup := &dogeboxd.PupState{
		ID:       fmt.Sprintf("%s-%s", manifest.Meta.Name, manifest.Meta.Version),
		Manifest: manifest,
		Source:   source.Config(),
	}

	// Get all dependencies that need to be installed
	deps, err := t.pups.CalculateDeps(tempPup.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't calculate dependencies: %s", err)
	}

	return tempPup, deps, nil
}

func (t api) installPup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req InstallPupRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	session, sessionOK := getSession(r, getBearerToken)
	if !sessionOK {
		sendErrorResponse(w, http.StatusBadRequest, "Failed to fetch session")
		return
	}
	req.SessionToken = session.DKM_TOKEN

	// If auto-install is enabled, determine dependencies
	if req.AutoInstallDependencies {
		_, deps, err := t.calculateDependencies(req.SourceId, req.PupName, req.PupVersion)
		if err != nil {
			sendErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		// Add the batch installation action
		id := t.dbx.AddAction(dogeboxd.InstallPup{
			PupName:      req.PupName,
			PupVersion:   req.PupVersion,
			SourceId:     req.SourceId,
			SessionToken: req.SessionToken,
		})

		// Add installation actions for dependencies
		for _, dep := range deps {
			if dep.CurrentProvider == "" {
				// Use the first available provider for each dependency
				if len(dep.InstallableProviders) > 0 {
					provider := dep.InstallableProviders[0]
					// Use the same source ID as the main pup for dependencies
					t.dbx.AddAction(dogeboxd.InstallPup{
						PupName:      provider.PupName,
						PupVersion:   provider.PupVersion,
						SourceId:     req.SourceId, // Use the same source ID as the main pup
						SessionToken: req.SessionToken,
					})
				}
			}
		}

		sendResponse(w, map[string]string{"id": id})
		return
	}

	// If auto-install is disabled, just install the main pup
	id := t.dbx.AddAction(dogeboxd.InstallPup{
		PupName:      req.PupName,
		PupVersion:   req.PupVersion,
		SourceId:     req.SourceId,
		SessionToken: req.SessionToken,
	})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) pupAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ID")
	action := r.PathValue("action")

	if action == "install" {
		sendErrorResponse(w, http.StatusBadRequest, "Must use PUT /pup to install")
		return
	}

	var a dogeboxd.Action
	switch action {
	case "uninstall":
		a = dogeboxd.UninstallPup{PupID: id}
	case "purge":
		a = dogeboxd.PurgePup{PupID: id}
	case "enable":
		a = dogeboxd.EnablePup{PupID: id}
	case "disable":
		a = dogeboxd.DisablePup{PupID: id}
	case "import-blockchain":
		a = dogeboxd.ImportBlockchainData{PupID: id}
	default:
		sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("No pup action %s", action))
		return
	}

	sendResponse(w, map[string]string{"id": t.dbx.AddAction(a)})
}

func (t api) updateHooks(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := []dogeboxd.PupHook{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(dogeboxd.UpdatePupHooks{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

// Returns all missing dependencies and all potential providers for each dependency for a given pup
func (t api) getMissingDeps(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	deps, err := t.pups.CalculateDeps(pupid)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Cannot find pup")
		return
	}
	// Only include dependencies that are not currently satisfied (no installed provider)
	missing := []dogeboxd.PupDependencyReport{}
	for _, dep := range deps {
		if dep.CurrentProvider == "" && !dep.Optional {
			// Create a new struct without the PupLogoBase64 field
			installableProviders := []dogeboxd.PupManifestDependencySource{}
			for _, provider := range dep.InstallableProviders {
				installableProviders = append(installableProviders, dogeboxd.PupManifestDependencySource{
					SourceLocation: provider.SourceLocation,
					PupName:        provider.PupName,
					PupVersion:     provider.PupVersion,
				})
			}
			dep.InstallableProviders = installableProviders
			missing = append(missing, dep)
		}
	}
	sendResponse(w, missing)
}

type InstallPupsRequest struct {
	Pups []InstallPupRequest `json:"pups"`
}

func (t api) installPups(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req InstallPupsRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	session, sessionOK := getSession(r, getBearerToken)
	if !sessionOK {
		sendErrorResponse(w, http.StatusBadRequest, "Failed to fetch session")
		return
	}

	// Create batch installation requests
	installRequests := make([]dogeboxd.InstallPup, 0)

	// Process each pup and its dependencies if auto-install is enabled
	for _, pup := range req.Pups {
		pup.SessionToken = session.DKM_TOKEN

		if pup.AutoInstallDependencies {
			_, deps, err := t.calculateDependencies(pup.SourceId, pup.PupName, pup.PupVersion)
			if err != nil {
				sendErrorResponse(w, http.StatusBadRequest, err.Error())
				return
			}

			// Add the main pup
			installRequests = append(installRequests, dogeboxd.InstallPup{
				PupName:      pup.PupName,
				PupVersion:   pup.PupVersion,
				SourceId:     pup.SourceId,
				SessionToken: pup.SessionToken,
			})

			// Add all required dependencies
			for _, dep := range deps {
				if dep.CurrentProvider == "" {
					// Use the first available provider for each dependency
					if len(dep.InstallableProviders) > 0 {
						provider := dep.InstallableProviders[0]
						// Use the same source ID as the main pup for dependencies
						installRequests = append(installRequests, dogeboxd.InstallPup{
							PupName:      provider.PupName,
							PupVersion:   provider.PupVersion,
							SourceId:     pup.SourceId, // Use same source for dependencies
							SessionToken: pup.SessionToken,
						})
					}
				}
			}
		} else {
			// Add just the main pup if auto-install is disabled
			installRequests = append(installRequests, dogeboxd.InstallPup{
				PupName:      pup.PupName,
				PupVersion:   pup.PupVersion,
				SourceId:     pup.SourceId,
				SessionToken: pup.SessionToken,
			})
		}
	}

	id := t.dbx.AddAction(dogeboxd.InstallPups(installRequests))
	sendResponse(w, map[string]string{"id": id})
}
