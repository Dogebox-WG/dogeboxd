package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mdlayher/wifi"
	"github.com/spf13/cobra"
)

type ScanResult struct {
	SSID       string  `json:"ssid"`
	BSSID      string  `json:"bssid"`
	Encryption string  `json:"encryption"`
	Quality    float32 `json:"quality"`
	Signal     string  `json:"signal"`
}

var wifiScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for wireless networks",
	Long:  `Scan for wireless networks for a given device.`,
	Run: func(cmd *cobra.Command, args []string) {
		ifaceName, _ := cmd.Flags().GetString("interface")
		wifiClient, err := wifi.New()
		if err != nil {
			log.Fatalln("Could not init a wifi interface client, skipping:", err)
		} else {
			defer wifiClient.Close()

			ifis, err := wifiClient.Interfaces()
			if err != nil {
				log.Fatalln("Could not find any wireless interfaces:", err)
			}

			map_result := make(map[string][]ScanResult)
			for _, ifi := range ifis {
				if ifi.Name != ifaceName || ifi.Type != wifi.InterfaceTypeStation {
					continue
				}

				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
				defer cancel()

				err = wifiClient.Scan(ctx, ifi)
				if err != nil {
					log.Fatalf("failed to scan access points for device %s: %v", ifi.Name, err)
				}

				aps, err := wifiClient.AccessPoints(ifi)
				if err != nil {
					log.Fatalf("failed to retrieve access points for device %s: %v", ifi.Name, err)
				}

				networks := []ScanResult{}
				for _, bss := range aps {
					scan := ScanResult{
						BSSID:      bss.BSSID.String(),
						SSID:       bss.SSID,
						Signal:     strconv.Itoa(int(bss.Signal) / 100),
						Quality:    float32(bss.SignalUnspecified),
						Encryption: mapRSNEncryption(bss.RSN),
					}
					networks = append(networks, scan)
				}
				map_result[ifi.Name] = networks
			}
			scan_result, _ := json.Marshal(map_result)
			fmt.Println(string(scan_result))
		}
	},
}

func mapRSNEncryption(rsn wifi.RSNInfo) string {
	return rsn.String()
}

func init() {
	wifiCmd.AddCommand(wifiScanCmd)

	wifiScanCmd.Flags().StringP("interface", "i", "", "Wireless interface name (required)")
	wifiScanCmd.MarkFlagRequired("interface")
}
