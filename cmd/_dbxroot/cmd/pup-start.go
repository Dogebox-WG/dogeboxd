package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a specific pup",
	Long: `Start a specific pup by providing its ID.
This command requires a --pupId flag with an alphanumeric value.

Example:
  pup start --pupId mypup123`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		if !utils.IsAlphanumeric(pupId) {
			fmt.Println("Error: pupId must contain only alphanumeric characters")
			return
		}

		fmt.Printf("Starting container with ID: %s\n", pupId)

		// We enforce the pup- prefix here to make sure that no bad-actor
		// can start a non-pup container that is running on the system.
		serviceName := fmt.Sprintf("container@pup-%s.service", pupId)

		// Use restart instead of start to handle containers that were previously
		// stopped and may be in a failed state. Restart cleans up any stale state.
		systemctlCmd := exec.Command("sudo", "systemctl", "restart", serviceName)
		systemctlCmd.Stdout = os.Stdout
		systemctlCmd.Stderr = os.Stderr

		if err := systemctlCmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error executing systemctl restart:", err)
			os.Exit(1)
		}

		fmt.Printf("Container %s started successfully\n", pupId)
	},
}

func init() {
	pupCmd.AddCommand(startCmd)

	startCmd.Flags().StringP("pupId", "p", "", "ID of the pup to start (required, alphanumeric only)")
	startCmd.MarkFlagRequired("pupId")
}
