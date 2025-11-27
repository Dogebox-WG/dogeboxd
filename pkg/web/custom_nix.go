package web

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

//go:embed custom.nix.default
var defaultCustomNix []byte

type GetCustomNixResponse struct {
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

type SaveCustomNixRequest struct {
	Content string `json:"content"`
}

type ValidateCustomNixRequest struct {
	Content string `json:"content"`
}

type ValidateCustomNixResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func (t api) getCustomNix(w http.ResponseWriter, r *http.Request) {
	customNixPath := filepath.Join(t.config.NixDir, "custom.nix")

	data, err := os.ReadFile(customNixPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return default template
			sendResponse(w, GetCustomNixResponse{
				Content: string(defaultCustomNix),
				Exists:  false,
			})
			return
		}
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to read custom.nix")
		return
	}

	sendResponse(w, GetCustomNixResponse{
		Content: string(data),
		Exists:  true,
	})
}

func (t api) saveCustomNix(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req SaveCustomNixRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	// Queue the save action
	action := dogeboxd.SaveCustomNix{Content: req.Content}
	id := t.dbx.AddAction(action)
	sendResponse(w, map[string]string{"id": id})
}

func (t api) validateCustomNix(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req ValidateCustomNixRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	// Validate the nix content directly (synchronous, no job needed)
	validationErr := validateNixContent(req.Content, t.config.TmpDir)

	if validationErr != nil {
		sendResponse(w, ValidateCustomNixResponse{
			Valid: false,
			Error: validationErr.Error(),
		})
		return
	}

	sendResponse(w, ValidateCustomNixResponse{
		Valid: true,
	})
}

// validateNixContent validates nix content using nix-instantiate --parse
func validateNixContent(content string, tmpDir string) error {
	// Create a temporary file to validate
	tmpFile, err := os.CreateTemp(tmpDir, "custom-nix-validate-*.nix")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Run nix-instantiate --parse to validate syntax
	cmd := exec.Command("nix-instantiate", "--parse", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &NixValidationError{Output: string(output)}
	}

	return nil
}

// NixValidationError represents a nix validation error
type NixValidationError struct {
	Output string
}

func (e *NixValidationError) Error() string {
	return e.Output
}

