package dbxdev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// checkBootstrapCmd verifies connection to dogeboxd by calling the bootstrap endpoint
func checkBootstrapCmd(socketPath string) tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/bootstrap", nil)
		if err != nil {
			return bootstrapCheckMsg{socketPath: socketPath, err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return bootstrapCheckMsg{socketPath: socketPath, err: err}
		}
		defer resp.Body.Close()

		// Parse the response to check setup status
		var result struct {
			SetupFacts struct {
				HasCompletedInitialConfiguration bool `json:"hasCompletedInitialConfiguration"`
			} `json:"setupFacts"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return bootstrapCheckMsg{socketPath: socketPath, err: fmt.Errorf("failed to parse bootstrap response: %w", err)}
		}

		// Check if initial configuration is complete
		if !result.SetupFacts.HasCompletedInitialConfiguration {
			return bootstrapCheckMsg{
				socketPath:            socketPath,
				err:                   nil,
				configurationComplete: false,
			}
		}

		return bootstrapCheckMsg{socketPath: socketPath, err: nil, configurationComplete: true}
	}
}

// authenticateCmd calls the dogeboxd authenticate endpoint
func authenticateCmd(password string) tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		payload := map[string]string{"password": password}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPost, "http://dogeboxd/authenticate", bytes.NewReader(body))
		if err != nil {
			return authMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return authMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusForbidden {
				return authMsg{err: fmt.Errorf("invalid password")}
			}
			return authMsg{err: fmt.Errorf("authentication failed: %d", resp.StatusCode)}
		}

		var result struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return authMsg{err: err}
		}

		return authMsg{token: result.Token}
	}
}

// addSourceCmd adds a new source to the system
func addSourceCmd(location, token string) tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		payload := map[string]string{"location": location}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPut, "http://dogeboxd/source", bytes.NewReader(body))
		if err != nil {
			return sourceAddedMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return sourceAddedMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return sourceAddedMsg{err: fmt.Errorf("failed to add source: %d", resp.StatusCode)}
		}

		// Get the list of sources to find the ID of our newly added source
		sourceId, err := findSourceId(location, token)
		if err != nil {
			return sourceAddedMsg{err: err}
		}

		return sourceAddedMsg{sourceId: sourceId}
	}
}

// findSourceId gets the list of sources and finds the ID for our location
func findSourceId(location, token string) (string, error) {
	client := getSocketClient()

	req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/sources", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Sources []struct {
			ID       string `json:"id"`
			Location string `json:"location"`
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, source := range result.Sources {
		if source.Location == location {
			return source.ID, nil
		}
	}

	return "", fmt.Errorf("source not found")
}

// readManifestVersion reads the version from manifest.json in the pup directory
func readManifestVersion(pupDir string) (string, error) {
	manifestPath := filepath.Join(pupDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest struct {
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	if manifest.Meta.Version == "" {
		return "", fmt.Errorf("no version found in manifest.json")
	}

	return manifest.Meta.Version, nil
}

// installPupCmd triggers pup installation
func installPupCmd(sourceId, pupName, token string) tea.Cmd {
	return func() tea.Msg {
		devDir, err := getDevDir()
		if err != nil {
			return pupInstalledMsg{err: err}
		}

		pupDir := filepath.Join(devDir, pupName)

		// Read version from manifest.json
		version, err := readManifestVersion(pupDir)
		if err != nil {
			return pupInstalledMsg{err: err}
		}

		client := getSocketClient()

		payload := map[string]interface{}{
			"pupName":                 pupName,
			"pupVersion":              version,
			"sourceId":                sourceId,
			"autoInstallDependencies": false,
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPut, "http://dogeboxd/pup", bytes.NewReader(body))
		if err != nil {
			return pupInstalledMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return pupInstalledMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return pupInstalledMsg{err: fmt.Errorf("failed to install pup: %d", resp.StatusCode)}
		}

		// Read the response to get the job ID
		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return pupInstalledMsg{err: fmt.Errorf("failed to decode response: %w", err)}
		}

		jobID := result["id"]
		return pupInstalledMsg{jobID: jobID, err: nil}
	}
}

// createSourceCmd creates a new source
func createSourceCmd(location string) tea.Cmd {
	return func() tea.Msg {
		socketPath := getSocketPath()

		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}
		client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

		payload := map[string]interface{}{
			"location": location,
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPut, "http://dogeboxd/source", bytes.NewReader(body))
		if err != nil {
			return sourceCreatedMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return sourceCreatedMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return sourceCreatedMsg{err: fmt.Errorf("failed to create source: %d", resp.StatusCode)}
		}

		return sourceCreatedMsg{err: nil}
	}
}

// deleteSourceCmd deletes a source
func deleteSourceCmd(sourceID string) tea.Cmd {
	return func() tea.Msg {
		socketPath := getSocketPath()

		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}
		client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://dogeboxd/source/%s", sourceID), nil)
		if err != nil {
			return sourceDeletedMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return sourceDeletedMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return sourceDeletedMsg{err: fmt.Errorf("failed to delete source: %d", resp.StatusCode)}
		}

		return sourceDeletedMsg{err: nil}
	}
}
