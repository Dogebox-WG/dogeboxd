package system

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type lsblkDevice struct {
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	Mountpoints []string        `json:"mountpoints"`
	RM          json.RawMessage `json:"rm"`
	Type        string          `json:"type"`
	Children    []lsblkDevice   `json:"children,omitempty"`
	Label       string          `json:"label"`
}

type lsblkOutput struct {
	Blockdevices []lsblkDevice `json:"blockdevices"`
}

func GetRemovableMounts() ([]dogeboxd.RemovableMount, error) {
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,PATH,MOUNTPOINTS,RM,TYPE,LABEL")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	var result lsblkOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	mounts := map[string]dogeboxd.RemovableMount{}
	var walk func(device lsblkDevice)
	walk = func(device lsblkDevice) {
		if isRemovable(device.RM) && len(device.Mountpoints) > 0 {
			for _, mount := range device.Mountpoints {
				if mount == "" {
					continue
				}
				mounts[mount] = dogeboxd.RemovableMount{
					Path:   mount,
					Label:  device.Label,
					Device: device.Path,
				}
			}
		}
		for _, child := range device.Children {
			walk(child)
		}
	}

	for _, device := range result.Blockdevices {
		walk(device)
	}

	paths := make([]string, 0, len(mounts))
	for path := range mounts {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	out := make([]dogeboxd.RemovableMount, 0, len(paths))
	for _, path := range paths {
		out = append(out, mounts[path])
	}
	return out, nil
}

func ValidateRemovablePath(path string) error {
	clean := filepath.Clean(path)
	mounts, err := GetRemovableMounts()
	if err != nil {
		return err
	}
	for _, mount := range mounts {
		if isPathWithin(clean, mount.Path) {
			return nil
		}
	}
	return fmt.Errorf("path is not on a removable mount")
}

func isRemovable(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	text := strings.Trim(string(raw), `"`)
	return text == "1" || text == "true"
}
