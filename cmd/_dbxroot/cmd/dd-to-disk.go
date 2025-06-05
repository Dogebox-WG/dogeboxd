package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var ddToDiskCmd = &cobra.Command{
	Use:   "dd-to-disk",
	Short: "Install Dogebox to a disk.",
	Long: `Install Dogebox to a disk.
This command requires --target-disk and --dbx-secret flags.

Example:
  _dbxroot dd-to-disk --target-disk /dev/sdb --dbx-secret ?`,
	Run: func(cmd *cobra.Command, args []string) {
		targetDisk, _ := cmd.Flags().GetString("target-disk")
		dbxSecret, _ := cmd.Flags().GetString("dbx-secret")

		if dbxSecret != system.DBXRootSecret {
			log.Printf("Invalid dbx secret")
			os.Exit(1)
		}

		defer func() {
			if r := recover(); r != nil {
				log.Printf("Failed to install to disk: %v", r)
				os.Exit(1)
			}
		}()

		disks, err := system.GetSystemDisks()
		if err != nil {
			log.Printf("Failed to get system disks: %v", err)
			os.Exit(1)
		}

		// Ensure target disk exists in disks
		var targetDiskExists bool
		var bootMediaDisk dogeboxd.SystemDisk
		for _, disk := range disks {
			if disk.Name == targetDisk {
				targetDiskExists = true
			}
			if disk.BootMedia && bootMediaDisk.Name == "" {
				bootMediaDisk = disk
			}
		}

		if !targetDiskExists {
			log.Printf("Target disk %s not found in system disks", targetDisk)
			os.Exit(1)
		}

		if bootMediaDisk.Name == "" {
			log.Printf("No boot media disk found")
			os.Exit(1)
		}

		if bootMediaDisk.Name == targetDisk {
			log.Printf("Source and target disks are the same: %s", targetDisk)
			os.Exit(1)
		}

		log.Printf("Using %s as source boot media", bootMediaDisk)
		log.Printf("Installing to target disk: %s", targetDisk)

		// Create partition table
		utils.RunParted(disk, "mklabel", "gpt")

		utils.RunParted(disk, "mkpart", "uboot", "16384s", "24575s")
		utils.RunParted(disk, "type", "1", "F808D051-1602-4DCD-9452-F9637FEFC49A")

		utils.RunParted(disk, "mkpart", "misc", "24576s", "32767s")
		utils.RunParted(disk, "type", "2", "C6D08308-E418-4124-8890-F8411E3D8D87")

		utils.RunParted(disk, "mkpart", "dtbo", "32768s", "40959s")
		utils.RunParted(disk, "type", "3", "2A583E58-486A-4BD4-ACE4-8D5454E97F5C")

		utils.RunParted(disk, "mkpart", "resource", "40960s", "73727s")
		utils.RunParted(distk, "type", "4", "6115F139-4F47-4BAF-8D23-B6957EAEE4B3")

		utils.RunParted(disk, "mkpart", "kernel", "73728s", "155647s")
		utils.RunParted(disk, "type", "5", "A83FBA16-D354-45C5-8B44-3EC50832D363")

		utils.RunParted(disk, "mkpart", "boot", "155648s", "221183s")
		utils.RunParted(disk, "type", "6", "500E2214-B72D-4CC3-D7C1-8419260130F5")

		utils.RunParted(disk, "mkpart", "recovery", "221184s", "286719s")
		utils.RunParted(disk, "type", "7", "E099DA71-5450-44EA-AA9F-1B771C582805")

		utils.RunParted(disk, "mkpart", "rootfs")
		utils.RunParted(disk, "type", "8", "AF12D156-5D5B-4EE3-B415-8D492CA12EA9")
		utils.RunParted(disk, "set", "8", "boot", "on")
		utils.RunParted(disk, "set", "8", "legacy_boot", "on")

		utils.RunCommand("dd", "if="+bootMediaDisk.Name, "of="+targetDisk, "skip=64", "seek=64", "bs=100k", "count=4", "status=progress")

		hasPartitionPrefix := strings.HasPrefix(disk, "/dev/nvme") || strings.HasPrefix(disk, "/dev/mmcblk")
		partitionPrefix := ""

		if hasPartitionPrefix {
			partitionPrefix = "p"
		}

		rootPartition := fmt.Sprintf("%s%s8", disk, partitionPrefix)

		utils.RunCommand("mkfs.ext4", "-L", "nixos", rootPartition)

		// Create /mnt if it doesn't exist, -p will prevent error if it already exists.
		utils.RunCommand("sudo", "mkdir", "-p", "/mnt")

		utils.RunCommand("sudo", "mount", rootPartition, "/mnt")

		// Copy our NixOS configuration over
		utils.RunCommand("mkdir", "-p", "/mnt/etc/nixos/")
		copyFiles("/etc/nixos/", "/mnt/etc/nixos/")

		// Generate hardware-configuration.nix
		utils.RunCommand("nixos-generate-config", "--root", "/mnt")

		// Set an installed flag so we know not to try again.
		utils.RunCommand("mkdir", "-p", "/mnt/opt/")
		utils.RunCommand("sudo", "touch", "/mnt/opt/dbx-installed")
		utils.RunCommand("sudo", "chown", "dogeboxd:dogebox", "/mnt/opt/dbx-installed")

		flakePath, err := utils.FlakePath()
		if err != nil {
			log.Printf("Failed to get flake path: %v", err)
			os.Exit(1)
		}

		// Install
		utils.RunCommand("nixos-install", "--flake", flakePath, "--no-root-passwd", "--root", "/mnt")
		utils.RunCommand("sudo", "umount", "/mnt")

		log.Println("Finished installing. Please remove installation media and reboot.")
	},
}

func init() {
	rootCmd.AddCommand(ddToDiskCmd)

	ddToDiskCmd.Flags().StringP("target-disk", "d", "", "Disk to install to (required)")
	ddToDiskCmd.MarkFlagRequired("target-disk")

	ddToDiskCmd.Flags().StringP("dbx-secret", "s", "", "?")
	ddToDiskCmd.MarkFlagRequired("dbx-secret")
}

//func getWrittenRootPartition(disk string) (string, error) {
//	cmd := exec.Command("lsblk", disk, "-o", "name,label", "--json")
//	output, err := cmd.Output()
//	if err != nil {
//		return "", fmt.Errorf("failed to run lsblk command: %w", err)
//	}
//
//	var result struct {
//		Blockdevices []struct {
//			Name     string `json:"name"`
//			Label    string `json:"label"`
//			Children []struct {
//				Name  string `json:"name"`
//				Label string `json:"label"`
//			} `json:"children,omitempty"`
//		} `json:"blockdevices"`
//	}
//
//	if err := json.Unmarshal(output, &result); err != nil {
//		return "", fmt.Errorf("failed to parse lsblk output: %w", err)
//	}
//
//	for _, device := range result.Blockdevices {
//		if device.Label == "nixos" {
//			return "/dev/" + device.Name, nil
//		}
//		for _, child := range device.Children {
//			if child.Label == "nixos" {
//				return "/dev/" + child.Name, nil
//			}
//		}
//	}
//
//	return "", fmt.Errorf("no partition with label 'nixos' found")
//}

func copyFiles(source string, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destination, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(destPath, info.Mode())
	})

	return err
}
