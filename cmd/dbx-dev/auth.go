package dbxdev

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

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

		// We don't need to parse the response, just check if we can connect
		return bootstrapCheckMsg{socketPath: socketPath, err: nil}
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
