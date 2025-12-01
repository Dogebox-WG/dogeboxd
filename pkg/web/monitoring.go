package web

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemStats represents system-level resource usage
type SystemStats struct {
	CPU  SystemStatMetric `json:"cpu"`
	RAM  SystemStatMetric `json:"ram"`
	Disk SystemStatMetric `json:"disk"`
}

type SystemStatMetric struct {
	Label   string    `json:"label"`
	Type    string    `json:"type"`
	Values  []float64 `json:"values"`
	Current float64   `json:"current"`
	Total   uint64    `json:"total,omitempty"` // For RAM/Disk in MB
	Used    uint64    `json:"used,omitempty"`  // For RAM/Disk in MB
}

// ServicesResponse represents the list of available external services
type ServicesResponse struct {
	Available []ServiceInfo `json:"available"`
}

// ServiceInfo represents an external service's status
type ServiceInfo struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Configured bool          `json:"configured"`
	Status     ServiceStatus `json:"status,omitempty"`
}

// ServiceStatus holds service-specific status information
type ServiceStatus struct {
	IP        string `json:"ip,omitempty"`
	Connected bool   `json:"connected"`
	Hostname  string `json:"hostname,omitempty"`
}

// TailscaleStatus represents the JSON output from `tailscale status --json`
type TailscaleStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		Online       bool     `json:"Online"`
		HostName     string   `json:"HostName"`
	} `json:"Self"`
}

// getSystemStats returns current system resource usage
func (t api) getSystemStats(w http.ResponseWriter, r *http.Request) {
	stats := SystemStats{}

	// Get CPU usage
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		stats.CPU = SystemStatMetric{
			Label:   "CPU Usage",
			Type:    "float",
			Values:  []float64{cpuPercent[0]}, // Single current value, frontend maintains history
			Current: cpuPercent[0],
		}
	}

	// Get Memory usage
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		stats.RAM = SystemStatMetric{
			Label:   "Memory Usage",
			Type:    "float",
			Values:  []float64{memInfo.UsedPercent},
			Current: memInfo.UsedPercent,
			Total:   memInfo.Total / 1024 / 1024, // Convert to MB
			Used:    memInfo.Used / 1024 / 1024,  // Convert to MB
		}
	}

	// Get Disk usage (root partition)
	diskInfo, err := disk.Usage("/")
	if err == nil {
		stats.Disk = SystemStatMetric{
			Label:   "Disk Usage",
			Type:    "float",
			Values:  []float64{diskInfo.UsedPercent},
			Current: diskInfo.UsedPercent,
			Total:   diskInfo.Total / 1024 / 1024, // Convert to MB
			Used:    diskInfo.Used / 1024 / 1024,  // Convert to MB
		}
	}

	sendResponse(w, stats)
}

// getSystemServices returns available external services and their status
func (t api) getSystemServices(w http.ResponseWriter, r *http.Request) {
	response := ServicesResponse{
		Available: []ServiceInfo{},
	}

	// Check for Tailscale
	tailscaleInfo := checkTailscale()
	if tailscaleInfo != nil {
		response.Available = append(response.Available, *tailscaleInfo)
	}

	// Future services can be added here:
	// - OpenVPN: checkOpenVPN()
	// - WireGuard: checkWireGuard()

	sendResponse(w, response)
}

// checkTailscale checks if Tailscale is installed and gets its status
func checkTailscale() *ServiceInfo {
	// Check if tailscale binary exists
	_, err := exec.LookPath("tailscale")
	if err != nil {
		// Tailscale not installed
		return nil
	}

	info := &ServiceInfo{
		ID:         "tailscale",
		Name:       "Tailscale",
		Configured: false,
		Status:     ServiceStatus{},
	}

	// Try to get Tailscale status
	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Tailscale installed but not configured/running
		info.Configured = false
		return info
	}

	var status TailscaleStatus
	if err := json.Unmarshal(output, &status); err != nil {
		info.Configured = false
		return info
	}

	// Check if Tailscale is running and connected
	info.Configured = status.BackendState == "Running"
	info.Status.Connected = status.Self.Online && status.BackendState == "Running"

	// Get the first Tailscale IP
	if len(status.Self.TailscaleIPs) > 0 {
		info.Status.IP = status.Self.TailscaleIPs[0]
	}

	// Get hostname (remove trailing dot from DNS name)
	if status.Self.DNSName != "" {
		info.Status.Hostname = strings.TrimSuffix(status.Self.DNSName, ".")
	} else {
		info.Status.Hostname = status.Self.HostName
	}

	return info
}
