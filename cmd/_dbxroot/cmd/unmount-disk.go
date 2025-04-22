package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var unmountCmd = &cobra.Command{
	Use:   "unmount-disk",
	Short: "Unmount a disk from a specified mount point",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mountPoint := args[0]

		unmountCmd := exec.Command("sudo", "umount", mountPoint)
		multiWriter := io.MultiWriter(os.Stdout)
		unmountCmd.Stderr = multiWriter
		unmountCmd.Stdout = multiWriter
		err := unmountCmd.Run()
		if err != nil {
			fmt.Printf("Error unmounting %s: %v\n", mountPoint, err)
			os.Exit(1)
		}
		fmt.Printf("Successfully unmounted %s\n", mountPoint)
	},
}

func init() {
	rootCmd.AddCommand(unmountCmd)
}
