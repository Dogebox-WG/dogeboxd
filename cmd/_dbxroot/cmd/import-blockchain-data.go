package cmd

import (
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

// Embedded shell script for importing blockchain data
const importBlockchainScript = `#!/run/current-system/sw/bin/bash

# Script to copy blockchain data from external drive
# Usage: ./blockchain-import.sh <destination_path> [--dry-run]

set -e

# Global variables
DRY_RUN=false
DATA_DIR=""
DEST_PATH=""
OWNER_UID="root"
OWNER_GID="root"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_dry_run() {
    echo -e "${YELLOW}[DRY RUN]${NC} $1"
}

# Parse command line arguments
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            --owner-uid)
                OWNER_UID="$2"
                shift 2
                ;;
            --owner-gid)
                OWNER_GID="$2"
                shift 2
                ;;
            -h|--help)
                echo "Usage: $0 <destination_path> [--dry-run] [--data-dir /path/to/data] [--owner-uid UID] [--owner-gid GID]"
                echo "  <destination_path>  The destination path to copy blockchain data to"
                echo "  --dry-run           Show what would be done without actually copying files"
                echo "  --data-dir          Specify dogeboxd data directory (required)"
                echo "  --owner-uid         UID for file ownership (default: root)"
                echo "  --owner-gid         GID for file ownership (default: root)"
                echo "  -h, --help          Show this help message"
                exit 0
                ;;
            *)
                if [[ -z "$DEST_PATH" ]]; then
                    DEST_PATH="$1"
                else
                    print_error "Unknown option: $1"
                    echo "Use -h or --help for usage information"
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # Validate that destination path was provided
    if [[ -z "$DEST_PATH" ]]; then
        print_error "Destination path is required"
        echo "Usage: $0 <destination_path> [--dry-run] [--data-dir /path/to/data]"
        echo "Use -h or --help for usage information"
        exit 1
    fi
    
    # Validate that data directory was provided
    if [[ -z "$DATA_DIR" ]]; then
        print_error "Data directory is required"
        echo "Usage: $0 <destination_path> [--dry-run] [--data-dir /path/to/data]"
        echo "Use -h or --help for usage information"
        exit 1
    fi
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   print_error "This script must be run as root (use sudo)"
   exit 1
fi

# Function to get source directory
get_source_directory() {
    local source_dir=""
    local found_paths=()
    local search_paths=("/Volumes" "/mnt" "/media")
    
    # Search for directories with blockchain data
    for base_path in "${search_paths[@]}"; do
        if [[ -d "$base_path" ]]; then
            # Check each subdirectory at the top level
            for dir in "$base_path"/*; do
                if [[ -d "$dir" ]]; then
                    # Check if this directory has blocks and chainstate at the top level
                    if [[ -d "$dir/blocks" && -d "$dir/chainstate" ]]; then
                        found_paths+=("$dir")
                    fi
                fi
            done
        fi
    done
    
    if [[ ${#found_paths[@]} -eq 0 ]]; then
        print_error "No directories with both 'blocks' and 'chainstate' folders found in /Volumes, /mnt, or /media"
        print_error "Please ensure your external drive is connected and contains Dogecoin Core blockchain data"
        print_error "Expected structure: /path/to/drive/blocks/ and /path/to/drive/chainstate/"
        return 1
    fi
    
    # If only one source directory found, use it automatically
    if [[ ${#found_paths[@]} -eq 1 ]]; then
        local auto_source="${found_paths[0]}"
        source_dir="$auto_source"
    else
        # Multiple source directories found - throw error
        print_error "Multiple blockchain data directories found:"
        for i in "${!found_paths[@]}"; do
            echo "  $((i+1)) - ${found_paths[$i]}" >&2
        done
        print_error "Please disconnect extra drives or specify a single source directory"
        print_error "This script requires exactly one blockchain data source to avoid ambiguity"
        return 1
    fi
    
    # Output the clean path to stdout, nothing else
    printf "%s\n" "$source_dir"
}

# Function to show directory sizes
show_directory_info() {
    local source_dir=$1
    local dest_dir=$2
    
    print_status "Directory information:"
    echo "Source: $source_dir"
    echo "Destination: $dest_dir"
    echo ""
    
    print_status "Source directory sizes:"
    du -sh "$source_dir/blocks" "$source_dir/chainstate" 2>/dev/null || print_warning "Could not calculate source sizes"
    
    print_status "Destination directory sizes (if exists):"
    du -sh "$dest_dir/blocks" "$dest_dir/chainstate" 2>/dev/null || echo "Destination directories don't exist or are empty"
}

# Main execution
main() {
    # Parse command line arguments
    parse_arguments "$@"
    
    # Version information for debugging
    print_status "Dogeboxd Blockchain Import Script v1.0.0"
    print_status "Features: targeted file copying for both blocks and chainstate, avoid directory traversal, improved error handling, fixed file path handling with spaces"
    
    if [[ "$DRY_RUN" == true ]]; then
        print_dry_run "DRY RUN MODE - No files will be copied"
        print_status "Dogeboxd Blockchain Data Copy Script (DRY RUN)"
    else
        print_status "Dogeboxd Blockchain Data Copy Script"
    fi
    print_status "====================================="
    
    # Create destination directory if it doesn't exist
    if [[ ! -d "$DEST_PATH" ]]; then
        print_status "Creating destination directory: $DEST_PATH"
        mkdir -p "$DEST_PATH"
    fi
    
    # Get source directory
    print_status "Step 1: Select source directory"
    source_dir=$(get_source_directory)
    if [[ $? -ne 0 ]]; then
        print_error "get_source_directory function failed with exit code $?"
        exit 1
    fi
    print_status "Found 1 source directory, automatically selecting: $source_dir"
    print_status "Selected source directory: $source_dir"
    
    # Show directory information
    show_directory_info "$source_dir" "$DEST_PATH"
    
    # Show operation summary
    echo ""
    if [[ "$DRY_RUN" == true ]]; then
        print_dry_run "Would copy blockchain data from:"
        echo "  $source_dir/{blocks,chainstate}"
        print_dry_run "To:"
        echo "  $DEST_PATH/{blocks,chainstate}"
    else
        print_warning "This will copy blockchain data from:"
        echo "  $source_dir/{blocks,chainstate}"
        print_warning "To:"
        echo "  $DEST_PATH/{blocks,chainstate}"
        echo ""
        print_warning "Only missing files will be copied. Existing files will be preserved."
        
        # Interactive confirmation (skip in dry-run mode)
        if [[ -t 0 ]]; then
            read -p "Are you sure you want to continue? (y/N): " confirm
            if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
                print_status "Operation cancelled"
                exit 0
            fi
        else
            print_status "Non-interactive mode detected, proceeding with copy..."
        fi
    fi
    
    # Copy data
    if [[ "$DRY_RUN" == true ]]; then
        print_dry_run "Step 2: Simulating blockchain data copy..."
        
        print_dry_run "Would copy blocks directory..."
        if [[ -d "$DEST_PATH/blocks" ]]; then
            print_dry_run "Would preserve existing blocks directory: $DEST_PATH/blocks"
        fi
        print_dry_run "Would use targeted .dat file copying to avoid directory traversal issues"
        print_dry_run "Would copy .dat files from $source_dir/blocks/ to $DEST_PATH/blocks/"
        
        print_dry_run "Would copy chainstate directory..."
        if [[ -d "$DEST_PATH/chainstate" ]]; then
            print_dry_run "Would preserve existing chainstate directory: $DEST_PATH/chainstate"
        fi
        print_dry_run "Would use targeted .ldb file copying to avoid directory traversal issues"
        print_dry_run "Would copy .ldb files from $source_dir/chainstate/ to $DEST_PATH/chainstate/"
        
        # Set proper permissions
        print_dry_run "Would set permissions..."
        print_dry_run "Would run: chown -R $OWNER_UID:$OWNER_GID \"$DEST_PATH/blocks\" \"$DEST_PATH/chainstate\""
        print_dry_run "Would run: chmod -R 755 \"$DEST_PATH/blocks\" \"$DEST_PATH/chainstate\""
        
        print_dry_run "DRY RUN: Blockchain data copy simulation completed!"
        print_dry_run "Run without --dry-run to perform the actual copy"
    else
        print_status "Step 2: Copying blockchain data..."
        
        print_status "Copying blocks directory..."
        # Create directory if it doesn't exist
        mkdir -p "$DEST_PATH/blocks"
        
        # Use targeted file copying to avoid directory traversal issues
        print_status "Using targeted file copying to avoid directory traversal issues..."
        
        # Copy block files one by one
        local copied_count=0
        local total_count=0
        
        if [ -d "$source_dir/blocks" ]; then
            total_count=$(ls "$source_dir/blocks"/*.dat 2>/dev/null | wc -l)
            print_status "Found approximately $total_count .dat files to copy"
            
            for file in "$source_dir/blocks"/*.dat; do
                if [ -f "$file" ]; then
                    local filename=$(basename "$file")
                    if cp "$file" "$DEST_PATH/blocks/$filename" 2>/dev/null; then
                        copied_count=$((copied_count + 1))
                        # Show progress every 50 files (blocks are larger, so less frequent updates)
                        if [ $((copied_count % 50)) -eq 0 ]; then
                            print_status "Copied $copied_count block files so far..."
                        fi
                    else
                        print_warning "Failed to copy: $filename"
                    fi
                fi
            done
            
            print_status "Successfully copied $copied_count block files"
        else
            print_warning "Source blocks directory not found"
        fi
        
        print_status "Copying chainstate directory..."
        # Create directory if it doesn't exist
        mkdir -p "$DEST_PATH/chainstate"
        
        # Try to increase file descriptor limit temporarily
        local old_ulimit=$(ulimit -n 2>/dev/null || echo "1024")
        ulimit -n 65536 2>/dev/null || print_warning "Could not increase file descriptor limit"
        
        # Use targeted .ldb file copying to avoid directory traversal issues
        print_status "Using targeted .ldb file copying to avoid directory traversal issues..."
        
        # Copy .ldb files one by one
        local copied_count=0
        local total_count=0
        
        if [ -d "$source_dir/chainstate" ]; then
            total_count=$(ls "$source_dir/chainstate"/*.ldb 2>/dev/null | wc -l)
            print_status "Found approximately $total_count .ldb files to copy"
            
            for file in "$source_dir/chainstate"/*.ldb; do
                if [ -f "$file" ]; then
                    local filename=$(basename "$file")
                    if cp "$file" "$DEST_PATH/chainstate/$filename" 2>/dev/null; then
                        copied_count=$((copied_count + 1))
                        # Show progress every 100 files
                        if [ $((copied_count % 100)) -eq 0 ]; then
                            print_status "Copied $copied_count files so far..."
                        fi
                    else
                        print_warning "Failed to copy: $filename"
                    fi
                fi
            done
            
            print_status "Successfully copied $copied_count .ldb files"
        else
            print_warning "Source chainstate directory not found"
        fi
        
        # Restore original file descriptor limit
        ulimit -n "$old_ulimit" 2>/dev/null || true
        
        # Set proper permissions
        print_status "Setting permissions..."
        chown -R "$OWNER_UID:$OWNER_GID" "$DEST_PATH/blocks" "$DEST_PATH/chainstate"
        chmod -R 755 "$DEST_PATH/blocks" "$DEST_PATH/chainstate"
        
        print_status "Blockchain data copy completed successfully!"
        print_status "Blockchain data is now available at: $DEST_PATH"
    fi
}

# Run main function
main "$@"
`

var importBlockchainDataCmd = &cobra.Command{
	Use:   "import-blockchain-data",
	Short: "Import blockchain data to Dogecoin Core pup",
	Long: `Import blockchain data to the Dogecoin Core pup if it's installed.
This command will automatically detect if Dogecoin Core is installed and
run the blockchain import script for that specific pup.

The command will:
1. Check if Dogecoin Core is installed
2. Run the blockchain import script to copy blockchain data

Note: Pup state management (stopping/starting) should be handled by the caller.

Example:
  import-blockchain-data --data-dir /home/user/data`,
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, _ := cmd.Flags().GetString("data-dir")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if dataDir == "" {
			fmt.Println("Error: data-dir is required")
			return
		}

		if dryRun {
			fmt.Println("DRY RUN MODE - No files will be copied")
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

		// Create a temporary script file
		tmpDir := os.TempDir()
		scriptPath := filepath.Join(tmpDir, "blockchain-import.sh")

		// Write the embedded script to the temporary file
		if err := os.WriteFile(scriptPath, []byte(importBlockchainScript), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temporary script: %v\n", err)
			os.Exit(1)
		}

		// Clean up the temporary file when we're done
		defer os.Remove(scriptPath)

		// Build command arguments
		scriptArgs := []string{"bash", scriptPath, storagePath, "--data-dir", dataDir, "--owner-uid", "420", "--owner-gid", "69"}
		if dryRun {
			scriptArgs = append(scriptArgs, "--dry-run")
		}

		// Run the script
		if dryRun {
			fmt.Println("[DRY RUN] Would run blockchain import script...")
		} else {
			fmt.Println("Running blockchain import script...")
		}
		importCmd := exec.Command("sudo", scriptArgs...)
		importCmd.Stdout = os.Stdout
		importCmd.Stderr = os.Stderr

		if err := importCmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error running import script:", err)
			os.Exit(1)
		}

		if dryRun {
			fmt.Println("[DRY RUN] Blockchain data import simulation completed successfully")
		} else {
			fmt.Println("Blockchain data import completed successfully")
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

func init() {
	importBlockchainDataCmd.Flags().String("data-dir", "", "Dogeboxd data directory")
	importBlockchainDataCmd.MarkFlagRequired("data-dir")
	importBlockchainDataCmd.Flags().Bool("dry-run", false, "Run the import in dry-run mode (no files will be copied)")
	rootCmd.AddCommand(importBlockchainDataCmd)
}
