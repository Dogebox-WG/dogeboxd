package dbxsetup

import (
	"time"
)

// setupStep represents the current step in the setup process
type setupStep int

const (
	stepCheckingStatus setupStep = iota
	stepReady
	stepAlreadyConfigured
	stepDeviceName
	stepKeyboardLayout
	stepTimezone
	stepStorageDevice
	stepBinaryCache
	stepPassword
	stepPasswordConfirm
	stepGenerateKey
	stepDisplaySeed
	stepConfirmSeed
	stepSelectNetwork
	stepNetworkPassword
	stepFinalizing
	stepComplete
)

// setupModel holds all the configuration data
type setupModel struct {
	// Configuration data
	deviceName         string
	keyboardLayout     string
	timezone           string
	storageDevice      string
	binaryCacheOS      bool
	binaryCachePups    bool
	password           string
	passwordConfirm    string
	masterKeySeed      []string
	seedConfirmation   string
	seedWordIndex      int
	selectedNetwork    string
	selectedNetworkIdx int
	networkType        string // "wifi" or "ethernet"
	networkInterface   string
	networkPassword    string
	networkEncryption  string

	// Available options
	keyboardLayouts   []keyboardLayout
	timezones         []timezone
	storageDevices    []storageDevice
	availableNetworks []networkInfo

	// UI state
	currentStep        setupStep
	width, height      int
	err                error
	isProcessing       bool
	showPassword       bool
	setupStepsComplete []bool // Track which setup steps are complete

	// Connection
	socketPath string
	authToken  string
}

// keyboardLayout represents a keyboard layout option
type keyboardLayout struct {
	Code        string `json:"id"`
	Name        string `json:"label"`
	Description string `json:"description,omitempty"`
}

// timezone represents a timezone option
type timezone struct {
	Code        string `json:"id"`
	Name        string `json:"label"`
	Description string `json:"description,omitempty"`
}

// storageDevice represents a storage device option
type storageDevice struct {
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	SizePretty  string   `json:"sizePretty"`
	Path        string   `json:"path"`
	Label       string   `json:"label"`
	BootMedia   bool     `json:"bootMedia"`
	MountPoints []string `json:"mountPoints,omitempty"`
}

// networkInfo represents a network option
type networkInfo struct {
	Type      string `json:"type"` // "wifi" or "ethernet"
	Interface string `json:"interface"`
	SSID      string `json:"ssid,omitempty"`     // Only for WiFi
	Security  string `json:"security,omitempty"` // Only for WiFi
	Signal    int    `json:"signal,omitempty"`   // Only for WiFi
	Connected bool   `json:"connected"`
}

// Message types
type tickMsg time.Time
type bootstrapCheckMsg struct {
	configured bool
	err        error
}
type keyboardLayoutsMsg struct {
	layouts []keyboardLayout
	err     error
}
type timezonesMsg struct {
	timezones []timezone
	err       error
}
type storageDevicesMsg struct {
	devices []storageDevice
	err     error
}
type networksMsg struct {
	networks []networkInfo
	err      error
}
type seedGeneratedMsg struct {
	seed []string
	err  error
}
type setupCompleteMsg struct {
	err error
}
type errorMsg struct {
	err error
}

// setupProgressMsg represents progress during finalization
type setupProgressMsg struct {
	message string
	done    bool
}

// setupStepCompleteMsg indicates a specific step is complete
type setupStepCompleteMsg struct {
	step int
}
