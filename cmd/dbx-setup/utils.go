package dbxsetup

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// getSocketClient returns an HTTP client configured for unix socket
func getSocketPath() string {
	socketPath := os.Getenv("DBX_SOCKET")
	if socketPath == "" {
		// Default to release information.
		socketPath = "/tmp/dbx-socket"
	}
	return socketPath
}

func getSocketClient() *http.Client {
	socketPath := getSocketPath()

	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

// formatBytes formats bytes to human readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
