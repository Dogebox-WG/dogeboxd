package dbxdev

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pupActionCmd returns a command that POSTs /pup/{id}/{action} via unix socket
// and then triggers a refresh of pup list.
func pupActionCmd(id, action string) tea.Cmd {
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
		client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

		url := "http://dogeboxd/pup/" + id + "/" + action
		req, err := http.NewRequest(http.MethodPost, url, nil)
		if err == nil {
			_, _ = client.Do(req) // ignore response errors for now
		}
		// After attempting action, refresh list
		return nil
	}
}
