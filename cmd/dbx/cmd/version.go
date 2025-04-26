package cmd

import (
	"fmt"

	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get dogebox version information",
	Run: func(cmd *cobra.Command, args []string) {
		version := version.GetDBXRelease()

		fmt.Printf("Dogebox Release: %s\n", version.Release)

		fmt.Printf("Packages:\n")
		for pkg, tuple := range version.Packages {
			fmt.Printf(" - %s: %s (%s)\n", pkg, tuple.Rev, tuple.Hash)
		}

		if len(version.Packages) == 0 {
			fmt.Printf("No packages found, if you're not actively developing, this is unexpected, please raise an issue.\n")
		}

		fmt.Printf("Git: %s\n", version.Git.Commit)
		fmt.Printf("Dirty: %t\n", version.Git.Dirty)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
