package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dogeorg/dogeboxd/pkg/version"
)

func IsAlphanumeric(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func IsAbsolutePath(path string) bool {
	return len(path) > 0 && path[0] == '/'
}

func RunParted(device string, args ...string) {
	args = append([]string{"parted", "-s", device, "--"}, args...)
	RunCommand(args...)
}

func RunCommand(args ...string) string {
	log.Printf("----------------------------------------")
	log.Printf("Running command: %+v", args)
	cmd := exec.Command(args[0], args[1:]...)
	output := &strings.Builder{}
	cmd.Stdout = io.MultiWriter(os.Stdout, output)
	cmd.Stderr = io.MultiWriter(os.Stderr, output)
	if err := cmd.Run(); err != nil {
		log.Printf("Error running command: %v", err.Error())
		panic(err)
	}

	log.Printf("----------------------------------------")

	return output.String()
}

func GetLoopDeviceBackingFile(loopDevice string) (string, error) {
	cmd := exec.Command("losetup", "-O", "NAME,BACK-FILE", loopDevice)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get loop device backing file: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, loopDevice) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("loop device %s not found", loopDevice)
}

func GetFlakePath() (string, error) {
	// Get system architecture
	archOutput := RunCommand("uname", "-m")
	architecture := strings.TrimSpace(archOutput)

	// Get build type
	buildTypeBytes, err := os.ReadFile("/opt/build-type")
	if err != nil {
		log.Printf("Failed to read build type: %v", err)
		os.Exit(1)
	}
	buildType := strings.TrimSpace(string(buildTypeBytes))

	flakeName := fmt.Sprintf("dogeboxos-%s-%s", buildType, architecture)
	flakePath := fmt.Sprintf("/etc/nixos#%s", flakeName)

	return flakePath, nil
}

func GetRebuildCommand(action string, isDev bool) (string, []string, error) {
	if isDev {
		// Assume the user is not running in a flake environment when running in dev mode.
		return "nixos-rebuild", []string{action}, nil
	}

	// Action is allowed to be "boot" or "switch". Throw an error if it's not.
	if action != "boot" && action != "switch" {
		return "", nil, fmt.Errorf("invalid action: %s", action)
	}

	flakePath, err := GetFlakePath()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get flake path: %w", err)
	}

	commandArgs := []string{action, "--flake", flakePath, "--impure"}

	versionInformation := version.GetDBXRelease()

	for pkg, tuple := range versionInformation.Packages {
		// Only support dogebox-wg thing for now.
		repo := fmt.Sprintf("github:dogebox-wg/%s/%s", pkg, tuple.Rev)
		commandArgs = append(commandArgs, "--override-input", pkg, repo)
	}

	return "nixos-rebuild", commandArgs, nil
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
