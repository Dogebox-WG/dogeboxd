package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var nixRSDevMode bool
var nixRSSetRelease string

var rsCmd = &cobra.Command{
	Use:   "rs",
	Short: "Executes nixos-rebuild switch",
	Run: func(cmd *cobra.Command, args []string) {
		rebuildCommand, rebuildArgs, err := utils.GetRebuildCommand("switch", nixRSDevMode, nixRSSetRelease)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting rebuild command: %v\n", err)
			os.Exit(1)
		}

		execCmd := exec.Command(rebuildCommand, rebuildArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		err = execCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rsCmd.Flags().BoolVarP(&nixRSDevMode, "dev", "d", false, "dev mode")
	rsCmd.Flags().StringVarP(&nixRSSetRelease, "set-release", "s", "", "rebuild with specific release (used for upgrades)")
	nixCmd.AddCommand(rsCmd)
}
