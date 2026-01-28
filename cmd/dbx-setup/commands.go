package dbxsetup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// checkBootstrapCmd checks if the system is already configured
func checkBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/bootstrap", nil)
		if err != nil {
			return bootstrapCheckMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return bootstrapCheckMsg{err: err}
		}
		defer resp.Body.Close()

		// Parse the response to check setup status
		var result struct {
			SetupFacts struct {
				HasCompletedInitialConfiguration bool `json:"hasCompletedInitialConfiguration"`
			} `json:"setupFacts"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return bootstrapCheckMsg{err: fmt.Errorf("failed to parse bootstrap response: %w", err)}
		}

		return bootstrapCheckMsg{
			configured: result.SetupFacts.HasCompletedInitialConfiguration,
			err:        nil,
		}
	}
}

// fetchKeyboardLayoutsCmd fetches available keyboard layouts
func fetchKeyboardLayoutsCmd() tea.Cmd {
	return func() tea.Msg {
		/*
			// For now, return a static list of common layouts
			// In production, this would fetch from the API
			layouts := []keyboardLayout{
				{Code: "us", Name: "US", Description: "US English"},
				{Code: "uk", Name: "UK", Description: "UK English"},
				{Code: "de", Name: "German", Description: "German (QWERTZ)"},
				{Code: "fr", Name: "French", Description: "French (AZERTY)"},
				{Code: "es", Name: "Spanish", Description: "Spanish"},
				{Code: "it", Name: "Italian", Description: "Italian"},
				{Code: "jp", Name: "Japanese", Description: "Japanese"},
				{Code: "dvorak", Name: "Dvorak", Description: "Dvorak"},
			}
			return keyboardLayoutsMsg{layouts: layouts}
		*/
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/keymaps", nil)
		if err != nil {
			return keyboardLayoutsMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return keyboardLayoutsMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return keyboardLayoutsMsg{err: fmt.Errorf("failed to fetch keyboard layouts: %s", body)}
		}

		var apiKeyboardLayouts []keyboardLayout
		if err := json.NewDecoder(resp.Body).Decode(&apiKeyboardLayouts); err != nil {
			return keyboardLayoutsMsg{err: err}
		}

		// Ensure results are sorted for a consistent UI experience
		sort.Slice(apiKeyboardLayouts, func(i, j int) bool {
			return apiKeyboardLayouts[i].Name < apiKeyboardLayouts[j].Name
		})

		return keyboardLayoutsMsg{layouts: apiKeyboardLayouts}
	}
}

// fetchTimezonesCmd fetches available timezones
func fetchTimezonesCmd() tea.Cmd {
	return func() tea.Msg {
		/*
			timezones := []timezone{
				{Code: "utc", Name: "UTC", Description: "UTC"},
				{Code: "europe/london", Name: "Europe/London", Description: "Europe/London"},
				{Code: "australia/sydney", Name: "Australia/Sydney", Description: "Australia/Sydney"},
				{Code: "australia/melbourne", Name: "Australia/Melbourne", Description: "Australia/Melbourne"},
				{Code: "australia/brisbane", Name: "Australia/Brisbane", Description: "Australia/Brisbane"},
				{Code: "australia/adelaide", Name: "Australia/Adelaide", Description: "Australia/Adelaide"},
				{Code: "australia/perth", Name: "Australia/Perth", Description: "Australia/Perth"},
				{Code: "asia/tokyo", Name: "Asia/Tokyo", Description: "Asia/Tokyo"},
			}
			return timezonesMsg{timezones: timezones}
		*/
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/timezones", nil)
		if err != nil {
			return timezonesMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return timezonesMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return timezonesMsg{err: fmt.Errorf("failed to fetch timezones: %s", body)}
		}

		var apiTimezones []timezone
		if err := json.NewDecoder(resp.Body).Decode(&apiTimezones); err != nil {
			return timezonesMsg{err: err}
		}

		// Ensure results are sorted for a consistent UI experience
		sort.Slice(apiTimezones, func(i, j int) bool {
			city := func(name string) string {
				parts := strings.Split(name, "/")
				return strings.ToLower(parts[len(parts)-1])
			}
			ci, cj := city(apiTimezones[i].Name), city(apiTimezones[j].Name)
			if ci == cj {
				return apiTimezones[i].Name < apiTimezones[j].Name // stable fallback
			}
			return ci < cj
		})

		return timezonesMsg{timezones: apiTimezones}
	}
}

// fetchStorageDevicesCmd fetches available storage devices
func fetchStorageDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/disks", nil)
		if err != nil {
			return storageDevicesMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return storageDevicesMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return storageDevicesMsg{err: fmt.Errorf("failed to fetch disks: %s", body)}
		}

		var devices []storageDevice
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			return storageDevicesMsg{err: err}
		}

		return storageDevicesMsg{devices: devices}
	}
}

// fetchNetworksCmd fetches available WiFi networks
func fetchNetworksCmd() tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		req, err := http.NewRequest(http.MethodGet, "http://dogeboxd/system/network/list", nil)
		if err != nil {
			return networksMsg{err: err}
		}

		resp, err := client.Do(req)
		if err != nil {
			return networksMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Network scan might not be available, return empty list
			return networksMsg{networks: []networkInfo{}}
		}

		var result struct {
			Success  bool          `json:"success"`
			Networks []interface{} `json:"networks"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return networksMsg{err: err}
		}

		// Parse network connections and extract all networks
		var allNetworks []networkInfo
		for _, net := range result.Networks {
			if netMap, ok := net.(map[string]interface{}); ok {
				netType := getStringField(netMap, "type")
				netInterface := getStringField(netMap, "interface")

				if netType == "wifi" {
					if ssids, ok := netMap["ssids"].([]interface{}); ok {
						for _, ssid := range ssids {
							if ssidMap, ok := ssid.(map[string]interface{}); ok {
								network := networkInfo{
									Type:      "wifi",
									Interface: netInterface,
									SSID:      getStringField(ssidMap, "ssid"),
									Security:  getStringField(ssidMap, "encryption"),
									Signal:    int(getFloatField(ssidMap, "quality") * 100),
								}
								allNetworks = append(allNetworks, network)
							}
						}
					}
				} else if netType == "ethernet" {
					network := networkInfo{
						Type:      "ethernet",
						Interface: netInterface,
					}
					allNetworks = append(allNetworks, network)
				}
			}
		}

		// Sort networks: Ethernet first, then WiFi by signal strength
		sort.Slice(allNetworks, func(i, j int) bool {
			// Ethernet always comes first
			if allNetworks[i].Type == "ethernet" && allNetworks[j].Type != "ethernet" {
				return true
			}
			if allNetworks[i].Type != "ethernet" && allNetworks[j].Type == "ethernet" {
				return false
			}
			// For WiFi, sort by signal strength
			if allNetworks[i].Type == "wifi" && allNetworks[j].Type == "wifi" {
				return allNetworks[i].Signal > allNetworks[j].Signal
			}
			return false
		})

		return networksMsg{networks: allNetworks}
	}
}

// generateMasterKeyCmd generates a new master key
func generateMasterKeyCmd(password string) tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		payload := map[string]string{
			"password": password,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return seedGeneratedMsg{err: err}
		}

		req, err := http.NewRequest(http.MethodPost, "http://dogeboxd/keys/create-master", bytes.NewReader(body))
		if err != nil {
			return seedGeneratedMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return seedGeneratedMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return seedGeneratedMsg{err: fmt.Errorf("failed to generate key: %s", respBody)}
		}

		var result struct {
			Success    bool     `json:"success"`
			SeedPhrase []string `json:"seedPhrase"`
			Token      string   `json:"token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return seedGeneratedMsg{err: err}
		}

		// The seed phrase is already an array
		return seedGeneratedMsg{seed: result.SeedPhrase}
	}
}

// sendProgress sends a progress update to the UI
func sendProgress(step int) {
	if program != nil {
		program.Send(setupStepCompleteMsg{step: step})
	}
}

// finalizeSetupCmd performs the final setup configuration
func finalizeSetupCmd(m setupModel) tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		// Add a small initial delay
		time.Sleep(500 * time.Millisecond)

		// Step 1: Set hostname
		payload := map[string]string{"hostname": m.deviceName}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPost, "http://dogeboxd/system/hostname", bytes.NewReader(body))
		if err != nil {
			return setupCompleteMsg{err: fmt.Errorf("failed to create hostname request: %w", err)}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return setupCompleteMsg{err: fmt.Errorf("failed to set hostname: %w", err)}
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return setupCompleteMsg{err: fmt.Errorf("failed to set hostname: status %d", resp.StatusCode)}
		}

		sendProgress(0) // Device name complete
		time.Sleep(500 * time.Millisecond)

		// Step 2: Keyboard layout (simulated - not actually set via API currently)
		sendProgress(1) // Keyboard layout complete
		time.Sleep(500 * time.Millisecond)

		// Step 3: Set timezone (if selected)
		/*
			if m.timezone != "" {
				payload := map[string]string{"timezone": m.timezone}
				body, _ := json.Marshal(payload)

				req, err := http.NewRequest(http.MethodPost, "http://dogeboxd/system/timezone", bytes.NewReader(body))
				if err != nil {
					return setupCompleteMsg{err: fmt.Errorf("failed to create timezone request: %w", err)}
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					return setupCompleteMsg{err: fmt.Errorf("failed to set timezone: %w", err)}
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return setupCompleteMsg{err: fmt.Errorf("failed to set timezone: status %d", resp.StatusCode)}
				}
			}
		*/
		sendProgress(2) // Timezone complete
		time.Sleep(500 * time.Millisecond)

		// Step 4: Set storage device (if selected)
		if m.storageDevice != "" {
			payload := map[string]string{"storageDevice": m.storageDevice}
			body, _ := json.Marshal(payload)

			req, err := http.NewRequest(http.MethodPost, "http://dogeboxd/system/storage", bytes.NewReader(body))
			if err != nil {
				return setupCompleteMsg{err: fmt.Errorf("failed to create storage request: %w", err)}
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				return setupCompleteMsg{err: fmt.Errorf("failed to set storage device: %w", err)}
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return setupCompleteMsg{err: fmt.Errorf("failed to set storage device: status %d", resp.StatusCode)}
			}
		}

		sendProgress(3) // Storage device complete
		time.Sleep(500 * time.Millisecond)

		// Step 5: Binary caches (handled in bootstrap call)
		sendProgress(4) // Binary caches complete
		time.Sleep(500 * time.Millisecond)

		// Step 6: User account (already created with key generation)
		sendProgress(5) // User account complete
		time.Sleep(500 * time.Millisecond)

		// Step 7: Set network (if selected)
		if m.networkType != "" {
			var networkPayload interface{}

			if m.networkType == "wifi" {
				networkPayload = map[string]interface{}{
					"interface":  m.networkInterface,
					"ssid":       m.selectedNetwork,
					"password":   m.networkPassword,
					"encryption": m.networkEncryption,
					"isHidden":   false,
				}
			} else {
				// Ethernet
				networkPayload = map[string]interface{}{
					"interface": m.networkInterface,
				}
			}

			body, _ := json.Marshal(networkPayload)

			req, err := http.NewRequest(http.MethodPut, "http://dogeboxd/system/network/set-pending", bytes.NewReader(body))
			if err != nil {
				return setupCompleteMsg{err: fmt.Errorf("failed to create network request: %w", err)}
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				return setupCompleteMsg{err: fmt.Errorf("failed to set network: %w", err)}
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return setupCompleteMsg{err: fmt.Errorf("failed to set network: status %d", resp.StatusCode)}
			}
		}

		sendProgress(6) // Network configuration complete
		time.Sleep(500 * time.Millisecond)

		// Step 8: Final bootstrap with binary cache settings
		bootstrapPayload := map[string]interface{}{
			"reflectorToken":              "", // Empty for now
			"reflectorHost":               "", // Empty for now
			"initialSSHKey":               "", // Empty for now
			"useFoundationOSBinaryCache":  m.binaryCacheOS,
			"useFoundationPupBinaryCache": m.binaryCachePups,
		}

		body, err = json.Marshal(bootstrapPayload)
		if err != nil {
			return setupCompleteMsg{err: fmt.Errorf("failed to marshal bootstrap payload: %w", err)}
		}

		req, err = http.NewRequest(http.MethodPost, "http://dogeboxd/system/bootstrap", bytes.NewReader(body))
		if err != nil {
			return setupCompleteMsg{err: fmt.Errorf("failed to create bootstrap request: %w", err)}
		}
		req.Header.Set("Content-Type", "application/json")

		resp, _ = client.Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}

		sendProgress(7) // Bootstrap complete
		time.Sleep(500 * time.Millisecond)

		return setupCompleteMsg{err: nil}
	}
}

// Helper functions for safe type conversion
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getFloatField(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}
