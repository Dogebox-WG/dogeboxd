package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var wifiTestCmd = &cobra.Command{
	Use:   "wifi-test",
	Short: "wifi-test",
	Run: func(cmd *cobra.Command, args []string) {
		iface, _ := cmd.Flags().GetString("interface")
		ssid, _ := cmd.Flags().GetString("ssid")
		password, _ := cmd.Flags().GetString("password")

		err := testWifiConnect(iface, ssid, password)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(wifiTestCmd)

	wifiTestCmd.Flags().StringP("interface", "i", "", "Wireless interface name (required)")
	wifiTestCmd.MarkFlagRequired("interface")

	wifiTestCmd.Flags().StringP("ssid", "s", "", "Wifi SSID Name (required)")
	wifiTestCmd.MarkFlagRequired("ssid")

	wifiTestCmd.Flags().StringP("password", "p", "", "Wifi Password (required)")
	wifiTestCmd.MarkFlagRequired("password")
}

func testWifiConnect(iface string, ssid string, password string) error {
	cmd := exec.Command("wpa_supplicant",
		"-i", iface,
		"-C", "/var/run/wpa_supplicant",
		"-B",
		"-f", "/var/log/wpa_supplicant.log",
		"-D", "nl80211,wext",
	)

	// Start wpa_supplicant
	err := cmd.Start()
	if err != nil {
		log.Printf("failed to start wpa_supplicant for interface %s, %+v", iface, err)
		return err
	}

	log.Printf("Started wpa_supplicant for interface: %s", iface)

	// Use wpa_cli to add and connect to the network
	addNetworkCmd := exec.Command("wpa_cli", "-i", iface, "add_network")
	networkID, err := addNetworkCmd.CombinedOutput()
	if err != nil {
		log.Printf("failed to add network: %+v", err)
		log.Print(string(networkID))
		return err
	}

	id := string(networkID)

	setSSIDCmd := exec.Command("wpa_cli", "-i", iface, "set_network", id, "ssid", fmt.Sprintf("\"%s\"", ssid))
	err = setSSIDCmd.Run()
	if err != nil {
		log.Printf("failed to set SSID: %v", err)
		return err
	}

	setPSKCmd := exec.Command("wpa_cli", "-i", iface, "set_network", id, "psk", fmt.Sprintf("\"%s\"", password))
	err = setPSKCmd.Run()
	if err != nil {
		log.Printf("failed to set PSK: %v", err)
		return err
	}

	enableNetworkCmd := exec.Command("wpa_cli", "-i", iface, "enable_network", id)
	err = enableNetworkCmd.Run()
	if err != nil {
		log.Printf("failed to enable network: %v", err)
		return err
	}

	log.Printf("Attempting to connect to WiFi network: %s", ssid)

	time.Sleep(20 * time.Second)

	// Check connection status
	statusCmd := exec.Command("wpa_cli", "-i", iface, "status")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		log.Printf("Failed to get connection status: %v", err)
		return err
	}

	status := string(statusOutput)

	if strings.Contains(status, "wpa_state=COMPLETED") {
		log.Printf("Successfully connected to WiFi network: %s", ssid)
	} else {
		log.Printf("Failed to connect to WiFi network: %s. Current status: %s", ssid, status)
	}

	return nil
}
