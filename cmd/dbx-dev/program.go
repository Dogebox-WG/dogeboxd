package dbxdev

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
)

// Global program instance for async messaging
var program *tea.Program

// SetProgram stores the program instance for async messaging
func SetProgram(p *tea.Program) {
	program = p
}

// ProgramOptions returns default program options.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithAltScreen()}
}

// NewModel creates a new TUI model instance.
func NewModel() tea.Model {
	return model{}
}

// WishHandler exposes the TUI over an SSH session.
func WishHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	// We need to capture the program instance after it starts
	// This is typically done in the main function that calls Start()
	return NewModel(), ProgramOptions()
}
