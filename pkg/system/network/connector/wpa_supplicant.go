package network_connector

import (
	"bytes"
	"errors"
	"os/exec"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.NetworkConnector = &NetworkConnectorWPASupplicant{}

type NetworkConnectorWPASupplicant struct{}

func (t NetworkConnectorWPASupplicant) Connect(network dogeboxd.SelectedNetwork) error {
	switch network.(type) {
	case dogeboxd.SelectedNetworkEthernet:
		{
			return errors.New("instantiated NetworkConnectorWPASupplicant for an ethernet network, aborting")
		}
	}

	n := network.(dogeboxd.SelectedNetworkWifi)

	iface := n.Interface
	ssid := n.Ssid
	password := n.Password

	// Prepare wpa_supplicant command with network information
	cmd := exec.Command("sudo", "_dbxroot", "wifi-test", "--interface", iface, "--ssid", ssid, "--password", password)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	return err
}
