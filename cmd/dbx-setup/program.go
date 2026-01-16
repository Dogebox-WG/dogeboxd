package dbxsetup

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// NewModel creates a new setup model instance.
func NewModel() tea.Model {
	return setupModel{
		socketPath:      getSocketPath(),
		currentStep:     stepCheckingStatus,
		binaryCacheOS:   true, // Default to using OS binary cache
		binaryCachePups: true, // Default to using Pups binary cache
		keyboardVP:      viewport.New(0, 0),
		timezoneVP:      viewport.New(0, 0),
	}
}
