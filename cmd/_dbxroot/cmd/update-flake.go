package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var updateFlakeCmd = &cobra.Command{
	Use:   "update-flake",
	Short: "Executes nix update-flake on /etc/nixos",
	Run: func(cmd *cobra.Command, args []string) {
		//impure, _ := cmd.Flags().GetString("impure")
		// TODO : Do we want to support flakes other than /etc/nixos?

		updateFlakeCmd := exec.Command("nix", "flake", "update", "--flake", "/etc/nixos")
		updateFlakeCmd.Stdout = os.Stdout
		updateFlakeCmd.Stderr = os.Stderr
		err := updateFlakeCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing : %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	nixCmd.AddCommand(updateFlakeCmd)
}
