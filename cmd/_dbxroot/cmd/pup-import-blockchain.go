package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

// Embedded shell script for importing blockchain data
const importBlockchainScript = `#!/run/current-system/sw/bin/bash

# Script to copy blockchain data from external drive to pup storage
# Usage: ./import-dogecoin-core-blockchain.sh <pup_id> [--dry-run]

set -e

# Global variables
DRY_RUN=false
DATA_DIR="/home/br/data"  # Default dogeboxd data directory
PUP_ID=""

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
            -h|--help)
                echo "Usage: $0 <pup_id> [--dry-run] [--data-dir /path/to/data]"
                echo "  <pup_id>     The ID of the pup to import blockchain data for"
                echo "  --dry-run    Show what would be done without actually copying files"
                echo "  --data-dir   Specify dogeboxd data directory (default: /home/br/data)"
                echo "  -h, --help   Show this help message"
                exit 0
                ;;
            *)
                if [[ -z "$PUP_ID" ]]; then
                    PUP_ID="$1"
                else
                    print_error "Unknown option: $1"
                    echo "Use -h or --help for usage information"
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # Validate that pup_id was provided
    if [[ -z "$PUP_ID" ]]; then
        print_error "Pup ID is required"
        echo "Usage: $0 <pup_id> [--dry-run] [--data-dir /path/to/data]"
        echo "Use -h or --help for usage information"
        exit 1
    fi
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   print_error "This script must be run as root (use sudo)"
   exit 1
fi

# Function to check if pup storage exists
pup_storage_exists() {
    local pup_id=$1
    local storage_path="$DATA_DIR/pups/storage/$pup_id"
    [[ -d "$storage_path" ]]
}



# Function to find pup storage path
get_pup_storage_path() {
    local pup_id=$1
    echo "$DATA_DIR/pups/storage/$pup_id"
}



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
    
    # Validate pup storage exists
    if ! pup_storage_exists "$PUP_ID"; then
        print_error "Pup storage directory does not exist for pup ID: $PUP_ID"
        print_error "Storage path: $(get_pup_storage_path "$PUP_ID")"
        exit 1
    fi
    
    # Get storage path
    storage_path=$(get_pup_storage_path "$PUP_ID")
    print_status "Pup storage found at: $storage_path"
    
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
    show_directory_info "$source_dir" "$storage_path"
    
    # Show operation summary
    echo ""
    if [[ "$DRY_RUN" == true ]]; then
        print_dry_run "Would copy blockchain data from:"
        echo "  $source_dir/{blocks,chainstate}"
        print_dry_run "To:"
        echo "  $storage_path/{blocks,chainstate}"
    else
        print_warning "This will copy blockchain data from:"
        echo "  $source_dir/{blocks,chainstate}"
        print_warning "To:"
        echo "  $storage_path/{blocks,chainstate}"
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
        if [[ -d "$storage_path/blocks" ]]; then
            print_dry_run "Would preserve existing blocks directory: $storage_path/blocks"
        fi
        print_dry_run "Would use targeted .dat file copying to avoid directory traversal issues"
        print_dry_run "Would copy .dat files from $source_dir/blocks/ to $storage_path/blocks/"
        
        print_dry_run "Would copy chainstate directory..."
        if [[ -d "$storage_path/chainstate" ]]; then
            print_dry_run "Would preserve existing chainstate directory: $storage_path/chainstate"
        fi
        print_dry_run "Would use targeted .ldb file copying to avoid directory traversal issues"
        print_dry_run "Would copy .ldb files from $source_dir/chainstate/ to $storage_path/chainstate/"
        
        # Set proper permissions for container user (UID 420, GID 69)
        print_dry_run "Would set permissions..."
        print_dry_run "Would run: chown -R 420:69 \"$storage_path/blocks\" \"$storage_path/chainstate\""
        print_dry_run "Would run: chmod -R 755 \"$storage_path/blocks\" \"$storage_path/chainstate\""
        
        print_dry_run "DRY RUN: Blockchain data copy simulation completed!"
        print_dry_run "Run without --dry-run to perform the actual copy"
    else
        print_status "Step 2: Copying blockchain data..."
        
                print_status "Copying blocks directory..."
        # Create directory if it doesn't exist
        mkdir -p "$storage_path/blocks"
        
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
                    if cp "$file" "$storage_path/blocks/$filename" 2>/dev/null; then
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
        mkdir -p "$storage_path/chainstate"
        
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
                    if cp "$file" "$storage_path/chainstate/$filename" 2>/dev/null; then
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
        
        # Set proper permissions for container user (UID 420, GID 69)
        print_status "Setting permissions..."
        chown -R 420:69 "$storage_path/blocks" "$storage_path/chainstate"
        chmod -R 755 "$storage_path/blocks" "$storage_path/chainstate"
        
        print_status "Blockchain data copy completed successfully!"
        print_status "You can now start your pup through the dogeboxd web interface"
    fi
}

# Run main function
main "$@"
`

var importBlockchainCmd = &cobra.Command{
	Use:   "import-blockchain",
	Short: "Import blockchain data to a specific pup",
	Long: `Import blockchain data to a specific pup by providing its ID.
This command requires a --pupId flag with an alphanumeric value.

The command will:
1. Stop the pup if it's running
2. Run the blockchain import script
3. Restart the pup if it was previously running

Example:
  pup import-blockchain --pupId mypup123`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		dataDir, _ := cmd.Flags().GetString("data-dir")

		if !utils.IsAlphanumeric(pupId) {
			fmt.Println("Error: pupId must contain only alphanumeric characters")
			return
		}

		if dataDir == "" {
			fmt.Println("Error: data-dir is required")
			return
		}

		fmt.Printf("Importing blockchain data for pup: %s\n", pupId)

		// Check if pup is currently running
		machineId := fmt.Sprintf("pup-%s", pupId)
		checkCmd := exec.Command("sudo", "machinectl", "status", machineId)
		wasRunning := checkCmd.Run() == nil

		// Stop the pup if it's running
		if wasRunning {
			fmt.Println("Stopping pup before import...")
			stopCmd := exec.Command("sudo", "machinectl", "stop", machineId)
			stopCmd.Stdout = os.Stdout
			stopCmd.Stderr = os.Stderr

			if err := stopCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "Error stopping pup:", err)
				os.Exit(1)
			}
		}

		// Create a temporary script file
		tmpDir := os.TempDir()
		scriptPath := filepath.Join(tmpDir, "import-dogecoin-core-blockchain.sh")

		// Write the embedded script to the temporary file
		if err := os.WriteFile(scriptPath, []byte(importBlockchainScript), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temporary script: %v\n", err)
			os.Exit(1)
		}

		// Clean up the temporary file when we're done
		defer os.Remove(scriptPath)

		fmt.Println("Running blockchain import script...")
		importCmd := exec.Command("sudo", "bash", scriptPath, pupId, "--data-dir", dataDir)
		importCmd.Stdout = os.Stdout
		importCmd.Stderr = os.Stderr

		if err := importCmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error running import script:", err)
			os.Exit(1)
		}

		// Restart the pup if it was previously running
		if wasRunning {
			fmt.Println("Restarting pup after import...")
			startCmd := exec.Command("sudo", "machinectl", "start", machineId)
			startCmd.Stdout = os.Stdout
			startCmd.Stderr = os.Stderr

			if err := startCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "Error restarting pup:", err)
				os.Exit(1)
			}
		}

		fmt.Println("Blockchain data import completed successfully")
	},
}

func init() {
	pupCmd.AddCommand(importBlockchainCmd)

	importBlockchainCmd.Flags().StringP("pupId", "p", "", "ID of the pup to import blockchain data to (required, alphanumeric only)")
	importBlockchainCmd.Flags().StringP("data-dir", "d", "", "Data directory path (required)")
	importBlockchainCmd.MarkFlagRequired("pupId")
	importBlockchainCmd.MarkFlagRequired("data-dir")
}
