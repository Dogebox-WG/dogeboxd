package cmd

import (
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/cmd/dbx/utils"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	source "github.com/dogeorg/dogeboxd/pkg/sources"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var canPupStartCmd = &cobra.Command{
	Use:   "can-pup-start",
	Short: "Check if a pup can start.",
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			log.Println("Failed to get dataDir flag.")
			utils.ExitBad(true)
			return
		}

		systemd, err := cmd.Flags().GetBool("systemd")
		if err != nil {
			log.Println("Failed to get systemd flag.")
			utils.ExitBad(true)
			return
		}

		pupId, err := cmd.Flags().GetString("pup-id")
		if err != nil {
			log.Println("Failed to get pup-id flag.")
			utils.ExitBad(true)
			return
		}

		store, err := dogeboxd.NewStoreManager(fmt.Sprintf("%s/dogebox.db", dataDir))
		if err != nil {
			log.Println("couldn't open store-manager db", err)
			utils.ExitBad(systemd)
		}
		sm := system.NewStateManager(store)

		isInRecoveryMode := system.IsRecoveryMode(dataDir, sm)

		if isInRecoveryMode {
			log.Println("Can start: false")
			utils.ExitBad(systemd)
			return
		}

		config := dogeboxd.ServerConfig{
			DataDir: dataDir,
			TmpDir:  "/tmp",
		}

		// Ideally we wouldn't have to init all these things.
		systemMonitor := system.NewSystemMonitor(config)

		pupManager, err := pup.NewPupManager(config, systemMonitor)
		if err != nil {
			log.Println("Failed to load PupManager: ", err)
			utils.ExitBad(systemd)
			return
		}

		sourceManager := source.NewSourceManager(config, sm, pupManager)
		pupManager.SetSourceManager(sourceManager)

		canStart, err := pupManager.CanPupStart(pupId)
		if err != nil {
			log.Println("Failed to check if pup can start: ", err)
			utils.ExitBad(systemd)
			return
		}

		if canStart {
			log.Println("Can start: true")
			os.Exit(0)
		}

		log.Println("Can start: false")
		utils.ExitBad(systemd)
	},
}

func init() {
	canPupStartCmd.Flags().StringP("pup-id", "p", "", "id of pup to check")
	canPupStartCmd.Flags().StringP("data-dir", "d", "/opt/dogebox", "dogebox data dir")
	canPupStartCmd.Flags().BoolP("systemd", "", false, "Exits with 255 instead of 1 if in recovery mode.")
	canPupStartCmd.MarkFlagRequired("data-dir")
	canPupStartCmd.MarkFlagRequired("pup-id")
	rootCmd.AddCommand(canPupStartCmd)
}
