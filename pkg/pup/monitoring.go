package pup

import (
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// called when we expect a pup to be changing state,
// this will rapidly poll for a few seconds and update
// the frontend with status.
func (t PupManager) FastPollPup(id string) {
	serviceName := fmt.Sprintf("container@pup-%s.service", id)
	log.Printf("[DEBUG] FastPollPup: sending fast poll request for %s (service: %s)", id, serviceName)
	t.monitor.GetFastMonChannel() <- serviceName
	log.Printf("[DEBUG] FastPollPup: fast poll request sent for %s", id)
}

/* Set the list of monitored services on the SystemMonitor */
func (t PupManager) updateMonitoredPups() {
	serviceNames := []string{}
	for _, p := range t.state {
		if p.Installation == dogeboxd.STATE_READY {
			serviceNames = append(serviceNames, fmt.Sprintf("container@pup-%s.service", p.ID))
		}
	}
	t.monitor.GetMonChannel() <- serviceNames
}
