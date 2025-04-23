package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var nixRBDevMode bool

var rbCmd = &cobra.Command{
	Use:   "rb",
	Short: "Executes nixos-rebuild boot",
	Run: func(cmd *cobra.Command, args []string) {
		rebuildCommand, rebuildArgs, err := utils.GetRebuildCommand("boot", nixRBDevMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting rebuild command: %v\n", err)
			os.Exit(1)
		}

		execCmd := exec.Command(rebuildCommand, rebuildArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		err = execCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild boot: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rbCmd.Flags().BoolVarP(&nixRBDevMode, "dev", "d", false, "dev mode")
	nixCmd.AddCommand(rbCmd)
}
