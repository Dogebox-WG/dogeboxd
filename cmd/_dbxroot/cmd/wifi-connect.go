package cmd

import (
	"log"
	"os"

	"github.com/mdlayher/wifi"
	"github.com/spf13/cobra"
)

var wifiConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "connect to a wifi network",
	Run: func(cmd *cobra.Command, args []string) {
		iface, _ := cmd.Flags().GetString("interface")
		ssid, _ := cmd.Flags().GetString("ssid")
		password, _ := cmd.Flags().GetString("password")

		err := connect(iface, ssid, password)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	wifiCmd.AddCommand(wifiConnectCmd)

	wifiConnectCmd.Flags().StringP("interface", "i", "", "Wireless interface name (required)")
	wifiConnectCmd.MarkFlagRequired("interface")

	wifiConnectCmd.Flags().StringP("ssid", "s", "", "Wifi SSID Name (required)")
	wifiConnectCmd.MarkFlagRequired("ssid")

	wifiConnectCmd.Flags().StringP("password", "p", "", "Wifi Password (optional)")
}

func connect(iface string, ssid string, password string) error {
	wifiClient, err := wifi.New()
	if err != nil {
		log.Fatalln("Could not init a wifi interface client, skipping:", err)
	} else {
		defer wifiClient.Close()

		ifis, err := wifiClient.Interfaces()
		if err != nil {
			log.Fatalln("Could not find any wireless interfaces:", err)
		}

		for _, ifi := range ifis {
			if ifi.Name != iface || ifi.Type != wifi.InterfaceTypeStation {
				continue
			}

			if password == "" {
				err = wifiClient.Connect(ifi, ssid)
				if err != nil {
					log.Fatalf("Failed to connect to network: %s on %s.\nerr: %v", ssid, ifi.Name, err)
				}
			} else {
				err = wifiClient.ConnectWPAPSK(ifi, ssid, password)
				if err != nil {
					log.Fatalf("Failed to connect to network: %s on %s.\nerr: %v", ssid, ifi.Name, err)
				}
			}
			break
		}
	}
	return nil
}
