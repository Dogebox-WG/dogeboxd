package dbxdev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	githubRepo = "Dogebox-WG/pup-templates"
	githubAPI  = "https://api.github.com/repos/%s/contents"
)

// fetchTemplatesCmd retrieves top-level directories from the pup-templates repository
func fetchTemplatesCmd() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 10 * time.Second}

		url := fmt.Sprintf(githubAPI, githubRepo)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return templatesMsg{err: err}
		}

		// Add GitHub API headers
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			return templatesMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return templatesMsg{err: fmt.Errorf("GitHub API returned status %d", resp.StatusCode)}
		}

		var items []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Path string `json:"path"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			return templatesMsg{err: err}
		}

		// Filter for directories only
		var templates []templateInfo
		for _, item := range items {
			if item.Type == "dir" && !strings.HasPrefix(item.Name, ".") {
				templates = append(templates, templateInfo{
					Name: item.Name,
					Path: item.Path,
				})
			}
		}

		return templatesMsg{templates: templates}
	}
}

// cloneTemplateCmd clones only the selected template folder using sparse checkout
func cloneTemplateCmd(template templateInfo, pupName string) tea.Cmd {
	return func() tea.Msg {
		devDir, err := getDevDir()
		if err != nil {
			return cloneCompleteMsg{err: fmt.Errorf("failed to get dev directory: %w", err)}
		}

		targetDir := filepath.Join(devDir, pupName)

		// Create dev directory if it doesn't exist
		if err := os.MkdirAll(devDir, 0755); err != nil {
			return cloneCompleteMsg{err: fmt.Errorf("failed to create dev directory %s: %w", devDir, err)}
		}

		// Check if target already exists
		if _, err := os.Stat(targetDir); err == nil {
			return cloneCompleteMsg{err: fmt.Errorf("directory %s already exists", targetDir)}
		}

		// Create temporary directory for cloning
		tmpDir, err := os.MkdirTemp("", "pup-template-*")
		if err != nil {
			return cloneCompleteMsg{err: fmt.Errorf("failed to create temp dir: %w", err)}
		}
		defer os.RemoveAll(tmpDir)

		// Clone with sparse checkout
		cmds := [][]string{
			{"git", "clone", "--no-checkout", "--depth=1", fmt.Sprintf("https://github.com/%s.git", githubRepo), tmpDir},
			{"git", "-C", tmpDir, "sparse-checkout", "init", "--cone"},
			{"git", "-C", tmpDir, "sparse-checkout", "set", template.Path},
			{"git", "-C", tmpDir, "checkout"},
		}

		for _, cmd := range cmds {
			if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
				return cloneCompleteMsg{err: fmt.Errorf("git command failed: %w", err)}
			}
		}

		// Move the template folder to target location
		templateSrc := filepath.Join(tmpDir, template.Path)
		if err := os.Rename(templateSrc, targetDir); err != nil {
			// If rename fails (cross-device), try copying
			if err := copyDir(templateSrc, targetDir); err != nil {
				return cloneCompleteMsg{err: fmt.Errorf("failed to move template: %w", err)}
			}
		}

		// Change ownership to shibe:dogebox recursively
		if err := exec.Command("chown", "-R", "shibe:dogebox", targetDir).Run(); err != nil {
			return cloneCompleteMsg{err: fmt.Errorf("failed to change ownership to shibe:dogebox: %w", err)}
		}

		return cloneCompleteMsg{err: nil}
	}
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return exec.Command("cp", "-r", src, dst).Run()
}
