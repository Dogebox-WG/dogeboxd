package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/Dogebox-WG/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var nixRSSetRelease string
var nixRSFlakeDir string
var nixRSSystemdRun bool
var nixRSSystemdUnit string

var rsCmd = &cobra.Command{
	Use:   "rs",
	Short: "Executes nixos-rebuild switch",
	Run: func(cmd *cobra.Command, args []string) {
		if nixRSSystemdRun {
			unitName := nixRSSystemdUnit
			if unitName == "" {
				unitName = "dogebox-system-update"
			}

			systemdArgs := []string{
				"--unit", unitName,
				"--collect",
				"--wait",
				"--pipe",
				"--setenv=PATH=/run/current-system/sw/bin:/run/wrappers/bin",
				"/run/wrappers/bin/_dbxroot",
				"nix",
				"rs",
			}
			if nixRSFlakeDir != "" {
				systemdArgs = append(systemdArgs, "--flake-dir", nixRSFlakeDir)
			}
			if nixRSSetRelease != "" {
				systemdArgs = append(systemdArgs, "--set-release", nixRSSetRelease)
			}

			execCmd := exec.Command("/run/current-system/sw/bin/systemd-run", systemdArgs...)
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			if err := execCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch in transient unit: %v\n", err)
				os.Exit(1)
			}
			return
		}

		rebuildCommand, rebuildArgs, err := utils.GetRebuildCommand("switch", nixRSSetRelease, nixRSFlakeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting rebuild command: %v\n", err)
			os.Exit(1)
		}

		execCmd := exec.Command(rebuildCommand, rebuildArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if nixRSFlakeDir != "" {
			execCmd.Env = append(os.Environ(), "DBX_UPGRADE_FLAKE_DIR="+nixRSFlakeDir)
		}

		err = execCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rsCmd.Flags().StringVarP(&nixRSSetRelease, "set-release", "s", "", "rebuild with specific release (used for upgrades)")
	rsCmd.Flags().StringVar(&nixRSFlakeDir, "flake-dir", "", "rebuild from a specific flake directory")
	rsCmd.Flags().BoolVar(&nixRSSystemdRun, "systemd-run", false, "run rebuild inside a transient systemd unit")
	rsCmd.Flags().StringVar(&nixRSSystemdUnit, "systemd-unit", "", "transient systemd unit name")
	nixCmd.AddCommand(rsCmd)
}
