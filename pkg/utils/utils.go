package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func ImageBytesToWebBase64(imgBytes []byte, filename string) (string, error) {
	logoData64 := base64.StdEncoding.EncodeToString(imgBytes)
	contentType := ""

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}

	return "data:" + contentType + ";base64," + logoData64, nil
}

func PrettyPrintDiskSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func CopyFiles(source string, destination string) error {
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

func GetPupNixAttributes(config dogeboxd.ServerConfig, diskSourcePath string, pupID string, pupManifestBuild dogeboxd.PupManifestBuild) ([]string, error) {
	nixFile := filepath.Join(diskSourcePath, pupManifestBuild.NixFile)

	log.Println("sourceDirectory", diskSourcePath)
	log.Println("nixFile", nixFile)

	// quick sanity check
	if _, err := os.Stat(nixFile); err != nil {
		return nil, fmt.Errorf("nix file %q missing: %w", nixFile, err)
	}

	expr := fmt.Sprintf(`builtins.attrNames (import %q {})`, nixFile)

	cmd := exec.Command("nix", "eval", "--json", "--expr", expr, "--impure")
	cmd.Dir = diskSourcePath
	out, err := cmd.Output()
	if err != nil {
		// wrap to keep the call-site stack tidy
		return nil, fmt.Errorf("nix eval: %w", err)
	}

	var attrs []string
	if err := json.Unmarshal(out, &attrs); err != nil {
		return nil, fmt.Errorf("decoding nix eval output: %w", err)
	}

	return attrs, nil
}

func GetPupNixDevelopmentModeServices(config dogeboxd.ServerConfig, diskSourcePath string, pupID string, manifest dogeboxd.PupManifest) ([]string, error) {
	pupNixAttributes, err := GetPupNixAttributes(config, diskSourcePath, pupID, manifest.Container.Build)
	if err != nil {
		return nil, err
	}

	devServices := []string{}

	// If any of the services have a "-dev" variant, then development mode is available
	for _, service := range manifest.Container.Services {
		devServiceName := service.Name + "-dev"
		if slices.Contains(pupNixAttributes, devServiceName) {
			devServices = append(devServices, service.Name)
		}
	}

	return devServices, nil
}
