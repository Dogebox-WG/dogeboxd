package cmd

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	dbxdev "github.com/dogeorg/dogeboxd/cmd/dbx-dev"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the developer TUI",
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(dbxdev.NewModel(), dbxdev.ProgramOptions()...)
		dbxdev.SetProgram(p)
		if _, err := p.Run(); err != nil {
			log.Fatalf("failed to run TUI: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
}
