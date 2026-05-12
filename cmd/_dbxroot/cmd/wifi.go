package cmd

import (
	"github.com/spf13/cobra"
)

var wifiCmd = &cobra.Command{
	Use:   "wifi",
	Short: "Interact with wifi module",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(wifiCmd)
}
