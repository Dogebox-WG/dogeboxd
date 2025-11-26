package web

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type SetTailscaleStateRequest struct {
	Enabled bool `json:"enabled"`
}

type SetTailscaleConfigRequest struct {
	AuthKey         string `json:"authKey"`
	Hostname        string `json:"hostname"`
	AdvertiseRoutes string `json:"advertiseRoutes"`
	Tags            string `json:"tags"`
	ListenPort      int    `json:"listenPort"`
}

func (t api) getTailscaleState(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox

	// Determine effective hostname (what will actually be used)
	effectiveHostname := dbxState.Tailscale.Hostname
	if effectiveHostname == "" {
		effectiveHostname = dbxState.Hostname // Use Dogebox system hostname
	}
	if effectiveHostname == "" {
		effectiveHostname = "dogebox" // Final fallback
	}

	// Return the full tailscale config but mask the auth key
	response := map[string]any{
		"enabled":           dbxState.Tailscale.Enabled,
		"hostname":          dbxState.Tailscale.Hostname,
		"effectiveHostname": effectiveHostname,
		"advertiseRoutes":   dbxState.Tailscale.AdvertiseRoutes,
		"tags":              dbxState.Tailscale.Tags,
		"listenPort":        dbxState.Tailscale.ListenPort,
		"hasAuthKey":        dbxState.Tailscale.AuthKey != "",
	}

	sendResponse(w, response)
}

func (t api) setTailscaleState(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req SetTailscaleStateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	var action dogeboxd.Action
	if req.Enabled {
		action = dogeboxd.EnableTailscale{}
	} else {
		action = dogeboxd.DisableTailscale{}
	}

	id := t.dbx.AddAction(action)
	sendResponse(w, map[string]string{"id": id})
}

func (t api) setTailscaleConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req SetTailscaleConfigRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	action := dogeboxd.SetTailscaleConfig{
		AuthKey:         req.AuthKey,
		Hostname:        req.Hostname,
		AdvertiseRoutes: req.AdvertiseRoutes,
		Tags:            req.Tags,
		ListenPort:      req.ListenPort,
	}

	id := t.dbx.AddAction(action)
	sendResponse(w, map[string]string{"id": id})
}

// TailscaleStatus represents the runtime status of Tailscale
type TailscaleStatus struct {
	Running      bool   `json:"running"`
	BackendState string `json:"backendState"`
	TailscaleIP  string `json:"tailscaleIP"`
	Hostname     string `json:"hostname"`
	Online       bool   `json:"online"`
	Error        string `json:"error,omitempty"`
}

func (t api) getTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	status := TailscaleStatus{
		Running: false,
	}

	// Check if tailscale is enabled in config first
	dbxState := t.sm.Get().Dogebox
	if !dbxState.Tailscale.Enabled {
		status.BackendState = "Disabled"
		sendResponse(w, status)
		return
	}

	// Run tailscale status --json to get actual status
	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Tailscale might not be running or not installed
		status.BackendState = "NotRunning"
		status.Error = "Tailscale service not responding"
		sendResponse(w, status)
		return
	}

	// Parse the JSON output
	var tsStatus struct {
		BackendState string `json:"BackendState"`
		Self         struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			Online       bool     `json:"Online"`
			HostName     string   `json:"HostName"`
		} `json:"Self"`
	}

	if err := json.Unmarshal(output, &tsStatus); err != nil {
		status.Error = "Failed to parse Tailscale status"
		sendResponse(w, status)
		return
	}

	status.Running = true
	status.BackendState = tsStatus.BackendState
	status.Online = tsStatus.Self.Online

	// Get the hostname (remove trailing dot from DNS name)
	status.Hostname = strings.TrimSuffix(tsStatus.Self.DNSName, ".")
	if status.Hostname == "" {
		status.Hostname = tsStatus.Self.HostName
	}

	// Get the first Tailscale IP
	if len(tsStatus.Self.TailscaleIPs) > 0 {
		status.TailscaleIP = tsStatus.Self.TailscaleIPs[0]
	}

	sendResponse(w, status)
}
