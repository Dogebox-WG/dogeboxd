package network_connector

import (
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

var _ dogeboxd.NetworkConnector = &NetworkConnectorEthernet{}

type NetworkConnectorEthernet struct{}

func (t NetworkConnectorEthernet) Connect(network dogeboxd.SelectedNetwork) error {
	// TODO: just assume we can connect for now.
	return nil
}
