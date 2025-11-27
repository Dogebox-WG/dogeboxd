package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	// Ensure dogebox.nix includes custom.nix
	l.Logf("Ensuring dogebox.nix includes custom.nix...")
	if err := t.ensureCustomNixImport(l); err != nil {
		l.Errf("Failed to update dogebox.nix: %v", err)
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

// ensureCustomNixImport ensures dogebox.nix includes the custom.nix import
func (t SystemUpdater) ensureCustomNixImport(l dogeboxd.SubLogger) error {
	dogeboxNixPath := filepath.Join(t.config.NixDir, "dogebox.nix")

	content, err := os.ReadFile(dogeboxNixPath)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// Check if custom.nix import already exists
	if strings.Contains(contentStr, "custom.nix") {
		l.Logf("custom.nix import already exists in dogebox.nix")
		return nil
	}

	// The dogebox.nix format has imports ending with:
	//   ++ lib.optionals (...) [ ... ]
	//
	//   ;
	// }
	// We need to insert before the standalone ";" line

	customImport := "  ++ lib.optionals (builtins.pathExists ./custom.nix) [ ./custom.nix ]\n"

	// Look for the pattern of whitespace + semicolon + newline + }
	// This marks the end of the imports block
	patterns := []string{
		"\n  ;\n}", // Two space indent
		"\n\t;\n}", // Tab indent
		"\n;\n}",   // No indent
		"  ;\n}",   // Two space at start
	}

	replaced := false
	for _, pattern := range patterns {
		if strings.Contains(contentStr, pattern) {
			contentStr = strings.Replace(contentStr, pattern, "\n"+customImport+pattern[1:], 1)
			replaced = true
			break
		}
	}

	if !replaced {
		l.Errf("Could not find insertion point in dogebox.nix")
		// As a fallback, try to find just "; followed by newline and }"
		if idx := strings.LastIndex(contentStr, ";"); idx > 0 {
			contentStr = contentStr[:idx] + "\n" + customImport + contentStr[idx:]
			replaced = true
		}
	}

	if !replaced {
		l.Errf("Failed to modify dogebox.nix - no suitable insertion point found")
		return nil // Don't fail the whole operation
	}

	l.Logf("Adding custom.nix import to dogebox.nix")
	return os.WriteFile(dogeboxNixPath, []byte(contentStr), 0644)
}
