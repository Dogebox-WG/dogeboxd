package network

import (
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	network_wifi "github.com/Dogebox-WG/dogeboxd/pkg/system/network/wifi"
)

func NewNetworkManager(nix dogeboxd.NixManager, sm dogeboxd.StateManager) dogeboxd.NetworkManager {
	// TODO: Do some system discovery and figure out how to init this properly.
	return NetworkManagerLinux{
		nix:     nix,
		sm:      sm,
		scanner: network_wifi.NewWifiScanner(),
	}
}
