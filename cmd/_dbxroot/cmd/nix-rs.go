package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var rsCmd = &cobra.Command{
	Use:   "rs",
	Short: "Executes nixos-rebuild switch",
	Run: func(cmd *cobra.Command, args []string) {
		rebuildCommand, err := utils.GetRebuildCommand("switch")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting rebuild command: %v\n", err)
			os.Exit(1)
		}

		err = exec.Command(rebuildCommand).Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	nixCmd.AddCommand(rsCmd)
}
