package dbxdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// templateFilesCmd walks through the pup directory and replaces pup_$template with the chosen pup name
func templateFilesCmd(pupName, templateName string) tea.Cmd {
	return func() tea.Msg {
		// Determine the dev directory
		var devDir string
		if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
			devDir = filepath.Join(dataDir, "dev")
		} else {
			homeDir, _ := os.UserHomeDir()
			devDir = filepath.Join(homeDir, "dev")
		}

		pupDir := filepath.Join(devDir, pupName)
		searchPattern := fmt.Sprintf("pup_%s", templateName)

		// Walk through all files in the directory
		err := filepath.Walk(pupDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories and .git folder
			if info.IsDir() || strings.Contains(path, ".git") {
				return nil
			}

			// Read file content
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// Replace all instances of pup_$template with pupName
			newContent := strings.ReplaceAll(string(content), searchPattern, pupName)

			// Only write if content changed
			if string(content) != newContent {
				if err := os.WriteFile(path, []byte(newContent), info.Mode()); err != nil {
					return err
				}
			}

			return nil
		})

		// Add synthetic delay
		time.Sleep(1 * time.Second)

		return templateCompleteMsg{err: err}
	}
}

// updateManifestHashCmd calculates SHA256 of pup.nix and updates manifest.json
func updateManifestHashCmd(pupName string) tea.Cmd {
	return func() tea.Msg {
		// Determine the dev directory
		var devDir string
		if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
			devDir = filepath.Join(dataDir, "dev")
		} else {
			homeDir, _ := os.UserHomeDir()
			devDir = filepath.Join(homeDir, "dev")
		}

		pupDir := filepath.Join(devDir, pupName)
		pupNixPath := filepath.Join(pupDir, "pup.nix")
		manifestPath := filepath.Join(pupDir, "manifest.json")

		// Calculate SHA256 of pup.nix
		file, err := os.Open(pupNixPath)
		if err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to open pup.nix: %w", err)}
		}
		defer file.Close()

		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to hash pup.nix: %w", err)}
		}

		hash := hex.EncodeToString(hasher.Sum(nil))

		// Read manifest.json
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to read manifest.json: %w", err)}
		}

		var manifest map[string]interface{}
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to parse manifest.json: %w", err)}
		}

		// Update the hash
		if container, ok := manifest["container"].(map[string]interface{}); ok {
			if build, ok := container["build"].(map[string]interface{}); ok {
				build["nixFileSha256"] = hash
			} else {
				return manifestUpdateMsg{err: fmt.Errorf("manifest.json missing container.build")}
			}
		} else {
			return manifestUpdateMsg{err: fmt.Errorf("manifest.json missing container")}
		}

		// Write back the updated manifest
		updatedData, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to marshal manifest.json: %w", err)}
		}

		if err := os.WriteFile(manifestPath, updatedData, 0644); err != nil {
			return manifestUpdateMsg{err: fmt.Errorf("failed to write manifest.json: %w", err)}
		}

		// Add synthetic delay
		time.Sleep(1 * time.Second)

		return manifestUpdateMsg{err: nil}
	}
}

// validatePupNameCmd checks if a pup name is valid and available
func validatePupNameCmd(pupName string, existingPups []pupInfo) tea.Cmd {
	return func() tea.Msg {
		// Check length
		if len(pupName) > 30 {
			return pupNameValidationMsg{err: fmt.Errorf("name must be 30 characters or less")}
		}

		// Check format (a-z0-9, underscores, dashes only)
		validName := regexp.MustCompile(`^[a-z0-9_-]+$`)
		if !validName.MatchString(pupName) {
			return pupNameValidationMsg{err: fmt.Errorf("name must contain only lowercase letters, numbers, underscores, and dashes (a-z, 0-9, _, -)")}
		}

		// Check if pup already exists
		for _, pup := range existingPups {
			if strings.EqualFold(pup.Name, pupName) {
				return pupNameValidationMsg{err: fmt.Errorf("a pup with name '%s' already exists", pupName)}
			}
		}

		// Check if directory already exists
		var devDir string
		if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
			devDir = filepath.Join(dataDir, "dev")
		} else {
			homeDir, _ := os.UserHomeDir()
			devDir = filepath.Join(homeDir, "dev")
		}

		targetDir := filepath.Join(devDir, pupName)
		if _, err := os.Stat(targetDir); err == nil {
			return pupNameValidationMsg{err: fmt.Errorf("directory '%s' already exists", targetDir)}
		}

		return pupNameValidationMsg{err: nil}
	}
}
