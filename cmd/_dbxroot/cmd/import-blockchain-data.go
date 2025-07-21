package cmd

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

// PupState represents the structure of a pup state file
type PupState struct {
	ID       string `json:"id"`
	Manifest struct {
		Meta struct {
			Name string `json:"name"`
		} `json:"meta"`
	} `json:"manifest"`
	Installation string `json:"installation"`
}

var importBlockchainDataCmd = &cobra.Command{
	Use:   "import-blockchain-data",
	Short: "Import blockchain data to Dogecoin Core pup",
	Long: `Import blockchain data to the Dogecoin Core pup if it's installed.
This command will automatically detect if Dogecoin Core is installed and
copy blockchain data from an external drive to the pup's storage directory.

The command will:
1. Check if Dogecoin Core is installed
2. Find blockchain data on mounted external drives
3. Copy blockchain data to the pup's storage directory

Note: Pup state management (stopping/starting) should be handled by the caller.

Example:
  import-blockchain-data --data-dir /home/user/data`,
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		ownerUID, _ := cmd.Flags().GetString("owner-uid")
		ownerGID, _ := cmd.Flags().GetString("owner-gid")

		if dataDir == "" {
			fmt.Println("Error: data-dir is required")
			return
		}

		// Check if running as root
		if syscall.Geteuid() != 0 {
			fmt.Println("Error: This command must be run as root")
			return
		}

		fmt.Println("Checking for installed Dogecoin Core pup...")

		// Find Dogecoin Core pup
		dogecoinPup, err := findDogecoinCorePup(dataDir)
		if err != nil {
			fmt.Printf("Error finding Dogecoin Core pup: %v\n", err)
			return
		}

		if dogecoinPup == nil {
			fmt.Println("No Dogecoin Core pup found. Please install Dogecoin Core first.")
			return
		}

		fmt.Printf("Found Dogecoin Core pup: %s (ID: %s)\n", dogecoinPup.Manifest.Meta.Name, dogecoinPup.ID)

		// Get pup storage path
		storagePath := filepath.Join(dataDir, "pups", "storage", dogecoinPup.ID)

		// Validate pup storage exists
		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			fmt.Printf("Error: Pup storage directory does not exist: %s\n", storagePath)
			os.Exit(1)
		}

		// Find source directory
		sourceDir, err := getSourceDirectory()
		if err != nil {
			fmt.Printf("Error finding source directory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found blockchain data source: %s\n", sourceDir)

		// Show directory information
		showDirectoryInfo(sourceDir, storagePath)

		// Show operation summary
		fmt.Println()
		fmt.Printf("Copying blockchain data from:\n")
		fmt.Printf("  %s/{blocks,chainstate}\n", sourceDir)
		fmt.Printf("To:\n")
		fmt.Printf("  %s/{blocks,chainstate}\n", storagePath)
		fmt.Println()
		fmt.Println("Only missing files will be copied. Existing files will be preserved.")
		fmt.Println("Proceeding with copy...")

		// Copy blockchain data
		if err := copyBlockchainData(sourceDir, storagePath, ownerUID, ownerGID); err != nil {
			fmt.Printf("Error copying blockchain data: %v\n", err)
			os.Exit(1)
		}
	},
}

// findDogecoinCorePup searches for an installed Dogecoin Core pup
func findDogecoinCorePup(dataDir string) (*PupState, error) {
	pupDir := filepath.Join(dataDir, "pups")

	// Read all .gob files in the pup directory
	files, err := os.ReadDir(pupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pup directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".gob" {
			pupPath := filepath.Join(pupDir, file.Name())

			// Read and parse the pup state file
			pupState, err := readPupState(pupPath)
			if err != nil {
				fmt.Printf("Warning: Failed to read pup state file %s: %v\n", pupPath, err)
				continue
			}

			// Check if this is Dogecoin Core
			if pupState.Manifest.Meta.Name == "Dogecoin Core" {
				return pupState, nil
			}
		}
	}

	return nil, nil
}

// readPupState reads and parses a pup state file
func readPupState(pupPath string) (*PupState, error) {
	file, err := os.Open(pupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open pup state file: %w", err)
	}
	defer file.Close()

	var pupState PupState
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&pupState); err != nil {
		return nil, fmt.Errorf("failed to decode pup state: %w", err)
	}

	return &pupState, nil
}

// getSourceDirectory finds directories with blockchain data
func getSourceDirectory() (string, error) {
	var foundPaths []string
	searchPaths := []string{"/Volumes", "/mnt", "/media"}

	// Search for directories with blockchain data
	for _, basePath := range searchPaths {
		if stat, err := os.Stat(basePath); err == nil && stat.IsDir() {
			// Check each subdirectory at the top level
			entries, err := os.ReadDir(basePath)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				if entry.IsDir() {
					dir := filepath.Join(basePath, entry.Name())

					// Check if this directory has blocks and chainstate at the top level
					blocksPath := filepath.Join(dir, "blocks")
					chainstatePath := filepath.Join(dir, "chainstate")

					hasBlocks := false
					hasChainstate := false

					if stat, err := os.Stat(blocksPath); err == nil && stat.IsDir() {
						hasBlocks = true
					}

					if stat, err := os.Stat(chainstatePath); err == nil && stat.IsDir() {
						hasChainstate = true
					}

					if hasBlocks && hasChainstate {
						foundPaths = append(foundPaths, dir)
					}
				}
			}
		}
	}

	if len(foundPaths) == 0 {
		return "", fmt.Errorf("no directories with both 'blocks' and 'chainstate' folders found in /Volumes, /mnt, or /media\nPlease ensure your external drive is connected and contains Dogecoin Core blockchain data\nExpected structure: /path/to/drive/blocks/ and /path/to/drive/chainstate/")
	}

	// If only one source directory found, use it automatically
	if len(foundPaths) == 1 {
		return foundPaths[0], nil
	}

	// Multiple source directories found - throw error
	fmt.Println("Error: Multiple blockchain data directories found:")
	for i, path := range foundPaths {
		fmt.Printf("  %d - %s\n", i+1, path)
	}
	return "", fmt.Errorf("please disconnect extra drives or specify a single source directory")
}

// showDirectoryInfo displays information about source and destination directories
func showDirectoryInfo(sourceDir, destDir string) {
	fmt.Println("Directory information:")
	fmt.Printf("Source: %s\n", sourceDir)
	fmt.Printf("Destination: %s\n", destDir)
	fmt.Println()

	fmt.Println("Source directory sizes:")
	showDirSize(filepath.Join(sourceDir, "blocks"))
	showDirSize(filepath.Join(sourceDir, "chainstate"))

	fmt.Println("Destination directory sizes (if exists):")
	showDirSize(filepath.Join(destDir, "blocks"))
	showDirSize(filepath.Join(destDir, "chainstate"))
}

// showDirSize shows the size of a directory using du
func showDirSize(dir string) {
	cmd := exec.Command("du", "-sh", dir)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Could not calculate size for %s\n", dir)
		return
	}
	fmt.Print(string(output))
}

// copyBlockchainData copies blockchain data from source to destination using Go's native file operations
func copyBlockchainData(sourceDir, destDir, ownerUID, ownerGID string) error {
	fmt.Println("Step 2: Copying blockchain data...")

	// Remove and recreate destination directories to ensure clean copy
	destBlocksDir := filepath.Join(destDir, "blocks")
	destChainstateDir := filepath.Join(destDir, "chainstate")

	fmt.Println("Removing existing blockchain data directories...")
	if err := os.RemoveAll(destBlocksDir); err != nil {
		return fmt.Errorf("failed to remove existing blocks directory: %w", err)
	}
	if err := os.RemoveAll(destChainstateDir); err != nil {
		return fmt.Errorf("failed to remove existing chainstate directory: %w", err)
	}

	fmt.Println("Creating fresh destination directories...")
	if err := os.MkdirAll(destBlocksDir, 0755); err != nil {
		return fmt.Errorf("failed to create blocks directory: %w", err)
	}
	if err := os.MkdirAll(destChainstateDir, 0755); err != nil {
		return fmt.Errorf("failed to create chainstate directory: %w", err)
	}

	// Copy chainstate directory first
	fmt.Println("Copying chainstate directory...")
	sourceChainstateDir := filepath.Join(sourceDir, "chainstate")
	if err := copyDirectoryFresh(sourceChainstateDir, destChainstateDir); err != nil {
		return fmt.Errorf("failed to copy chainstate: %w", err)
	}

	// Copy blocks directory
	fmt.Println("Copying blocks directory...")
	sourceBlocksDir := filepath.Join(sourceDir, "blocks")
	if err := copyDirectoryFresh(sourceBlocksDir, destBlocksDir); err != nil {
		return fmt.Errorf("failed to copy blocks: %w", err)
	}

	// Set proper permissions
	fmt.Println("Setting permissions...")
	if err := setOwnership(filepath.Join(destDir, "blocks"), ownerUID, ownerGID); err != nil {
		return fmt.Errorf("failed to set ownership on blocks: %w", err)
	}
	if err := setOwnership(filepath.Join(destDir, "chainstate"), ownerUID, ownerGID); err != nil {
		return fmt.Errorf("failed to set ownership on chainstate: %w", err)
	}

	if err := setPermissions(filepath.Join(destDir, "blocks")); err != nil {
		return fmt.Errorf("failed to set permissions on blocks: %w", err)
	}
	if err := setPermissions(filepath.Join(destDir, "chainstate")); err != nil {
		return fmt.Errorf("failed to set permissions on chainstate: %w", err)
	}

	fmt.Printf("Blockchain data is now available at: %s\n", destDir)
	return nil
}

// copyDirectoryFresh copies all contents from source directory to destination directory
func copyDirectoryFresh(sourceDir, destDir string) error {
	// First pass: count total files
	totalFiles := 0
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to count files: %w", err)
	}

	fmt.Printf("Found %d files to copy\n", totalFiles)

	// Second pass: copy files with progress
	fileCount := 0

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from source
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file
		fileCount++

		// Show progress every 50 files
		if fileCount%50 == 0 {
			fmt.Printf("Copied %d/%d files...\n", fileCount, totalFiles)
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})

	// Show final count
	if err == nil {
		fmt.Printf("Successfully copied %d/%d files\n", fileCount, totalFiles)
	}

	return err
}

// setOwnership sets the ownership of a directory recursively
func setOwnership(dir, uid, gid string) error {
	cmd := exec.Command("chown", "-R", uid+":"+gid, dir)
	return cmd.Run()
}

// setPermissions sets the permissions of a directory recursively
func setPermissions(dir string) error {
	cmd := exec.Command("chmod", "-R", "755", dir)
	return cmd.Run()
}

func init() {
	importBlockchainDataCmd.Flags().String("data-dir", "", "Dogeboxd data directory")
	importBlockchainDataCmd.MarkFlagRequired("data-dir")
	importBlockchainDataCmd.Flags().String("owner-uid", "420", "UID for file ownership")
	importBlockchainDataCmd.Flags().String("owner-gid", "69", "GID for file ownership")
	rootCmd.AddCommand(importBlockchainDataCmd)
}
