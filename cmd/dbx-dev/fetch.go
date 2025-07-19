package dbxdev

import (
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchPupsCmd retrieves pup information via the unix socket.
func fetchPupsCmd() tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		resp, err := client.Get("http://dogeboxd/system/bootstrap")
		if err != nil {
			return pupsMsg{err: err}
		}
		defer resp.Body.Close()

		var payload struct {
			States map[string]json.RawMessage `json:"states"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return pupsMsg{err: err}
		}

		out := make([]pupInfo, 0, len(payload.States))
		for id, raw := range payload.States {
			var s struct {
				Manifest struct {
					Meta struct {
						Name string `json:"name"`
					} `json:"meta"`
				} `json:"manifest"`
				Installation     string   `json:"installation"`
				Enabled          bool     `json:"enabled"`
				BrokenReason     string   `json:"brokenReason"`
				IsDevModeEnabled bool     `json:"isDevModeEnabled"`
				DevModeServices  []string `json:"devModeServices"`
			}
			if err := json.Unmarshal(raw, &s); err != nil {
				continue
			}
			out = append(out, pupInfo{
				ID:           id,
				Name:         s.Manifest.Meta.Name,
				State:        s.Installation,
				Enabled:      s.Enabled,
				Error:        s.BrokenReason,
				DevEnabled:   s.IsDevModeEnabled,
				DevAvailable: len(s.DevModeServices) > 0,
			})
		}
		return pupsMsg{list: out}
	}
}

// fetchSourcesCmd retrieves source information via the unix socket.
func fetchSourcesCmd() tea.Cmd {
	return func() tea.Msg {
		client := getSocketClient()

		resp, err := client.Get("http://dogeboxd/sources")
		if err != nil {
			return sourcesMsg{err: err}
		}
		defer resp.Body.Close()

		var payload struct {
			Sources []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Location    string `json:"location"`
				Type        string `json:"type"`
			} `json:"sources"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return sourcesMsg{err: err}
		}

		sources := make([]sourceInfo, len(payload.Sources))
		for i, s := range payload.Sources {
			sources[i] = sourceInfo{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				Location:    s.Location,
				Type:        s.Type,
			}
		}
		return sourcesMsg{sources: sources}
	}
}
