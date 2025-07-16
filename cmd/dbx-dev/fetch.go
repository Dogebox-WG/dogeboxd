package dbxdev

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchPupsCmd retrieves pup information via the unix socket.
func fetchPupsCmd() tea.Cmd {
	return func() tea.Msg {
		socketPath := os.Getenv("DBX_SOCKET")
		if socketPath == "" {
			socketPath = filepath.Join(os.Getenv("HOME"), "data", "dbx-socket")
		}

		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		}
		client := &http.Client{Transport: tr, Timeout: 2 * time.Second}

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
