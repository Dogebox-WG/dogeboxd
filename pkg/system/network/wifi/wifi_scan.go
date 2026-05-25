package network_wifi

import (
	"bytes"
	"encoding/json"
	"os/exec"
)

var _ WifiScanner = &IWScanner{}

type IWScanner struct{}

func (s IWScanner) Scan(interfaceName string) ([]ScannedWifiNetwork, error) {
	cmd := exec.Command("sudo", "_dbxroot", "wifi", "scan", "-i", interfaceName)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var output map[string][]ScannedWifiNetwork
	err = json.Unmarshal(out.Bytes(), &output)
	if err != nil {
		return nil, err
	}
	return output[interfaceName], nil
}
