package network_wifi

type ScannedWifiNetwork struct {
	SSID       string  `json:"ssid"`
	BSSID      string  `json:"bssid"`
	Encryption string  `json:"encryption"`
	Quality    float32 `json:"quality"`
	Signal     string  `json:"signal"`
}

type WifiScanner interface {
	Scan(networkInterface string) ([]ScannedWifiNetwork, error)
}

func NewWifiScanner() WifiScanner {
	// TODO: Do some system discovery and figure out how to init this properly.
	return IWScanner{}
}
