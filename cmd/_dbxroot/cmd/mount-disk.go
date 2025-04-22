package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var mountCmd = &cobra.Command{
	Use:   "mount-disk",
	Short: "Mount a disk device to a specified mount point",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		devicePath := args[0]
		mountPoint := args[1]

		mountCmd := exec.Command("sudo", "mount", devicePath, mountPoint)
		multiWriter := io.MultiWriter(os.Stdout)
		mountCmd.Stderr = multiWriter
		mountCmd.Stdout = multiWriter
		err := mountCmd.Run()
		if err != nil {
			fmt.Printf("Error mounting device %s to %s: %v\n", devicePath, mountPoint, err)
			os.Exit(1)
		}
		fmt.Printf("Successfully mounted %s to %s\n", devicePath, mountPoint)
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)
}
