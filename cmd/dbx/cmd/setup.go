package cmd

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	dbxsetup "github.com/dogeorg/dogeboxd/cmd/dbx-setup"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Start the initial system setup wizard",
	Long:  `Configure your Dogebox system with an interactive setup wizard.`,
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(dbxsetup.NewModel(), dbxsetup.ProgramOptions()...)
		dbxsetup.SetProgram(p)
		if _, err := p.Run(); err != nil {
			log.Fatalf("failed to run setup TUI: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
