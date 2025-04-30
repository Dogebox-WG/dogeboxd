package system

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/go-resty/resty/v2"
)

type ReflectorFileData struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

func SaveReflectorTokenForReboot(config dogeboxd.ServerConfig, host, token string) error {
	data := ReflectorFileData{
		Host:  host,
		Token: token,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal reflector data: %w", err)
	}

	filePath := filepath.Join(config.DataDir, "reflector.json")
	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write reflector data: %w", err)
	}

	return nil
}

func CheckAndSubmitReflectorData(config dogeboxd.ServerConfig, networkManager dogeboxd.NetworkManager) error {
	if config.DisableReflector {
		log.Println("Reflector disabled, skipping checking")
		return nil
	}

	filePath := filepath.Join(config.DataDir, "reflector.json")
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read reflector data file: %w", err)
	}

	var data ReflectorFileData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		log.Println("invalid reflector data: host or token is empty")
		return nil
	}

	host := data.Host
	token := data.Token

	if host == "" || token == "" {
		log.Println("invalid reflector data: host or token is empty")
		return nil
	}

	// Try, sleep, retry for a bit so we can wait for the network to catch up
	var localIP net.IP
	err = retry(10, 2*time.Second, func() (err error) {
		localIP, err = networkManager.GetLocalIP()
		return err
	})
	if err != nil {
		log.Printf("Could not determine local IP address for reflector submission: %s", err)
		return nil
	}

	log.Printf("Submitting reflector data to %s w/ token %s, ip %s", host, token, localIP.String())

	client := resty.New()
	client.SetBaseURL(host)
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	resp, err := client.R().
		SetBody(map[string]string{"token": token, "ip": localIP.String()}).
		Post("/")

	if err != nil {
		log.Printf("Failed to submit to reflector: %s", err)
		return err
	}

	if resp.StatusCode() != http.StatusCreated {
		log.Printf("Failed to submit to reflector: %s", resp.String())
		return fmt.Errorf("failed to submit to reflector: %s", resp.String())
	}

	return nil
}

func retry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; ; i++ {
		err = f()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		log.Println("retrying after error:", err)
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}
