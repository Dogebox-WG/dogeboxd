package system

import (
	"os"
	"os/exec"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// ValidateNix validates nix content using nix-instantiate --parse.
// Returns nil if valid, otherwise returns the error message.
func (t SystemUpdater) ValidateNix(content string) error {
	// Create a temporary file to validate
	tmpFile, err := os.CreateTemp(t.config.TmpDir, "custom-nix-validate-*.nix")
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

// SaveCustomNix validates and saves the custom.nix content,
// then triggers a system rebuild.
func (t SystemUpdater) SaveCustomNix(content string, l dogeboxd.SubLogger) error {
	l.Logf("Validating custom nix configuration...")

	// Validate first
	if err := t.ValidateNix(content); err != nil {
		l.Errf("Validation failed: %v", err)
		return err
	}

	l.Logf("Validation passed, saving configuration...")

	// Save the file
	customNixPath := filepath.Join(t.config.NixDir, "custom.nix")
	if err := os.WriteFile(customNixPath, []byte(content), 0644); err != nil {
		l.Errf("Failed to write custom.nix: %v", err)
		return err
	}

	l.Logf("Triggering system rebuild...")

	// Trigger rebuild
	if err := t.nix.Rebuild(l); err != nil {
		l.Errf("Rebuild failed: %v", err)
		return err
	}

	l.Logf("Custom configuration applied successfully")
	return nil
}
