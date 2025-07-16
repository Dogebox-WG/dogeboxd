package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/wish"
	wishbubble "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	dbxdev "github.com/dogeorg/dogeboxd/cmd/dbx-dev"
)

func main() {
	var dataDir string
	flag.StringVar(&dataDir, "data-dir", "", "Directory for storing SSH host key")
	flag.Parse()

	// Determine host key path
	var hostKeyPath string
	if dataDir != "" {
		hostKeyPath = filepath.Join(dataDir, "dbx_ssh_host_key")
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("failed to get home directory: %v", err)
		}
		hostKeyPath = filepath.Join(homeDir, ".ssh", "dbx_ssh_dev_host_key")

		// Ensure .ssh directory exists
		sshDir := filepath.Join(homeDir, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			log.Fatalf("failed to create .ssh directory: %v", err)
		}
	}

	const addr = "0.0.0.0:42069"

	srv, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			wishbubble.Middleware(dbxdev.WishHandler),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("failed to create SSH server: %v", err)
	}

	log.Printf("dbx-ssh listening on %s (host key: %s)", addr, hostKeyPath)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("error starting SSH server: %v", err)
	}
}
