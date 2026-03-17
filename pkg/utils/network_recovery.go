package utils

import (
	"fmt"
	"net"
	"time"
)

const (
	networkRecoveryAttempts = 10
	networkRecoverySleep    = 2 * time.Second
)

// WaitForNetworkRecovery blocks until setup can pick a usable egress IP again
// after applying a system rebuild that restarts networking.
func WaitForNetworkRecovery(getLocalIP func() (net.IP, error)) error {
	_, err := waitForLocalIP(getLocalIP, networkRecoveryAttempts, networkRecoverySleep, time.Sleep)
	if err != nil {
		return fmt.Errorf("network did not recover after applying system configuration: %w", err)
	}

	return nil
}

func waitForLocalIP(
	getLocalIP func() (net.IP, error),
	attempts int,
	sleep time.Duration,
	sleeper func(time.Duration),
) (net.IP, error) {
	var localIP net.IP

	for i := 0; i < attempts; i++ {
		var err error
		localIP, err = getLocalIP()
		if err == nil {
			return localIP, nil
		}

		if i == attempts-1 {
			return nil, fmt.Errorf("after %d attempts, last error: %w", attempts, err)
		}

		sleeper(sleep)
	}

	return nil, fmt.Errorf("after %d attempts, local IP was never available", attempts)
}
