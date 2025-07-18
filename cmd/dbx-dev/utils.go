package dbxdev

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"
)

// getSocketPath returns the path to the DBX socket, checking environment variables
// and falling back to the default location.
func getSocketPath() string {
	socketPath := os.Getenv("DBX_SOCKET")
	if socketPath == "" {
		// Default to release information.
		socketPath = "/tmp/dbx-socket"
	}
	return socketPath
}

// getSocketClient returns an HTTP client configured to communicate over the DBX unix socket.
func getSocketClient() *http.Client {
	socketPath := getSocketPath()

	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

// getSocketConn returns a raw unix socket connection to the DBX socket.
// Useful for WebSocket connections that need direct socket access.
func getSocketConn() (net.Conn, error) {
	socketPath := getSocketPath()
	return net.Dial("unix", socketPath)
}
