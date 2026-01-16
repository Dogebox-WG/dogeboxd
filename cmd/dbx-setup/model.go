package dbxsetup

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Init initializes the model and returns initial commands
func (m setupModel) Init() tea.Cmd {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Start by checking if system is already configured
	return tea.Batch(
		checkBootstrapCmd(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

// Update handles messages and updates the model
func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Leave room for headers/help; clamp to sensible minimums
		listHeight := msg.Height - 8
		if listHeight < 3 {
			listHeight = 3
		}
		listWidth := msg.Width - 4
		if listWidth < 20 {
			listWidth = 20
		}

		m.keyboardVP.Width = listWidth
		m.keyboardVP.Height = listHeight
		m.timezoneVP.Width = listWidth
		m.timezoneVP.Height = listHeight

		m.refreshKeyboardViewport()
		m.refreshTimezoneViewport()
		return m, nil

	case tea.KeyMsg:
		// Global quit handling
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// Handle input based on current step
		switch m.currentStep {
		case stepCheckingStatus:
			// No input during status check
			return m, nil
		case stepReady:
			if msg.String() == "enter" {
				m.currentStep = stepDeviceName
				m.err = nil
			}
			return m, nil
		case stepAlreadyConfigured:
			if msg.String() == "enter" || msg.String() == "q" {
				return m, tea.Quit
			}
			return m, nil
		case stepDeviceName:
			return m.handleDeviceNameInput(msg)
		case stepKeyboardLayout:
			return m.handleKeyboardLayoutInput(msg)
		case stepTimezone:
			return m.handleTimezoneInput(msg)
		case stepStorageDevice:
			return m.handleStorageDeviceInput(msg)
		case stepBinaryCache:
			return m.handleBinaryCacheInput(msg)
		case stepPassword:
			return m.handlePasswordInput(msg)
		case stepPasswordConfirm:
			return m.handlePasswordConfirmInput(msg)
		case stepDisplaySeed:
			return m.handleSeedDisplayInput(msg)
		case stepConfirmSeed:
			return m.handleSeedConfirmInput(msg)
		case stepSelectNetwork:
			return m.handleNetworkInput(msg)
		case stepNetworkPassword:
			return m.handleNetworkPasswordInput(msg)
		case stepComplete:
			if msg.String() == "enter" || msg.String() == "q" {
				return m, tea.Quit
			}
		}

	case bootstrapCheckMsg:
		if msg.err != nil {
			m.err = msg.err
			m.currentStep = stepAlreadyConfigured
		} else if msg.configured {
			// System is already configured
			m.currentStep = stepAlreadyConfigured
		} else {
			// System needs configuration
			m.currentStep = stepReady
			// TODO : check here
			// Fetch keyboard layouts in preparation
			return m, fetchKeyboardLayoutsCmd()
		}
		return m, nil

	case keyboardLayoutsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.keyboardLayouts = msg.layouts
			// Set default to first option
			if len(m.keyboardLayouts) > 0 {
				m.keyboardLayout = m.keyboardLayouts[0].Code
			}
			m.refreshKeyboardViewport()
		}
		return m, nil

	case timezonesMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.timezones = msg.timezones
			// Set default to first option
			if len(m.timezones) > 0 {
				m.timezone = m.timezones[0].Code
			}
			m.refreshTimezoneViewport()
		}
		return m, nil

	case storageDevicesMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.storageDevices = msg.devices
			// Set default to boot media if available, otherwise first device
			if len(m.storageDevices) > 0 {
				// Look for boot media first
				bootMediaFound := false
				for _, device := range m.storageDevices {
					if device.BootMedia {
						m.storageDevice = device.Name
						bootMediaFound = true
						break
					}
				}
				// If no boot media, use first device
				if !bootMediaFound {
					m.storageDevice = m.storageDevices[0].Name
				}
			}
		}
		return m, nil

	case networksMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.availableNetworks = msg.networks
		}
		return m, nil

	case seedGeneratedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.masterKeySeed = msg.seed
			m.currentStep = stepDisplaySeed
		}
		return m, nil

	case setupCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.currentStep = stepComplete
		}
		m.isProcessing = false
		return m, nil

	case setupStepCompleteMsg:
		// Mark the step as complete
		if msg.step >= 0 && msg.step < len(m.setupStepsComplete) {
			m.setupStepsComplete[msg.step] = true
		}
		return m, nil

	case setupProgressMsg:
		// Update progress message during finalization
		// This could be displayed in the UI
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.isProcessing = false
		return m, nil

	case tickMsg:
		// Keep ticking during finalization for spinner animation
		if m.currentStep == stepFinalizing && m.isProcessing {
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		return m, nil
	}

	return m, nil
}

// View renders the UI
func (m setupModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Build the view based on current step
	var content string

	switch m.currentStep {
	case stepCheckingStatus:
		content = m.renderCheckingStatusStep()
	case stepReady:
		content = m.renderReadyStep()
	case stepAlreadyConfigured:
		content = m.renderAlreadyConfiguredStep()
	case stepDeviceName:
		content = m.renderDeviceNameStep()
	case stepKeyboardLayout:
		content = m.renderKeyboardLayoutStep()
	case stepTimezone:
		content = m.renderTimezoneStep()
	case stepStorageDevice:
		content = m.renderStorageDeviceStep()
	case stepBinaryCache:
		content = m.renderBinaryCacheStep()
	case stepPassword:
		content = m.renderPasswordStep()
	case stepPasswordConfirm:
		content = m.renderPasswordConfirmStep()
	case stepGenerateKey:
		content = m.renderGeneratingKeyStep()
	case stepDisplaySeed:
		content = m.renderSeedDisplayStep()
	case stepConfirmSeed:
		content = m.renderSeedConfirmStep()
	case stepSelectNetwork:
		content = m.renderNetworkSelectStep()
	case stepNetworkPassword:
		content = m.renderNetworkPasswordStep()
	case stepFinalizing:
		content = m.renderFinalizingStep()
	case stepComplete:
		content = m.renderCompleteStep()
	}

	// Add error display if needed
	if m.err != nil {
		content += fmt.Sprintf("\n\nError: %v", m.err)
	}

	return content
}

// Input handlers for each step
func (m setupModel) handleDeviceNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.deviceName != "" {
			m.currentStep = stepKeyboardLayout
			m.err = nil
		} else {
			m.err = fmt.Errorf("device name cannot be empty")
		}
	case "backspace":
		if len(m.deviceName) > 0 {
			m.deviceName = m.deviceName[:len(m.deviceName)-1]
		}
	default:
		if len(msg.String()) == 1 && len(m.deviceName) < 30 {
			m.deviceName += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handleKeyboardLayoutInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	updated := false

	switch msg.String() {
	case "enter":
		m.currentStep = stepTimezone
		m.err = nil
		// Fetch timezones when moving to that step
		return m, fetchTimezonesCmd()
	case "up", "k":
		if len(m.keyboardLayouts) > 0 {
			for i, layout := range m.keyboardLayouts {
				if layout.Code == m.keyboardLayout && i > 0 {
					m.keyboardLayout = m.keyboardLayouts[i-1].Code
					updated = true
					break
				}
			}
		}
	case "down", "j":
		if len(m.keyboardLayouts) > 0 {
			for i, layout := range m.keyboardLayouts {
				if layout.Code == m.keyboardLayout && i < len(m.keyboardLayouts)-1 {
					m.keyboardLayout = m.keyboardLayouts[i+1].Code
					updated = true
					break
				}
			}
		}
	case "left", "esc":
		m.currentStep = stepDeviceName
	}

	if updated {
		m.refreshKeyboardViewport()
	}

	return m, nil
}

func (m setupModel) handleTimezoneInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	updated := false

	switch msg.String() {
	case "enter":
		if m.timezone != "" {
			m.currentStep = stepStorageDevice
			m.err = nil
			// Fetch storage devices when moving to that step
			return m, fetchStorageDevicesCmd()
		}
		m.err = fmt.Errorf("please select a timezone")
	case "up", "k":
		if len(m.timezones) > 0 {
			for i, tz := range m.timezones {
				if tz.Code == m.timezone && i > 0 {
					m.timezone = m.timezones[i-1].Code
					updated = true
					break
				}
			}
		}
	case "down", "j":
		if len(m.timezones) > 0 {
			for i, tz := range m.timezones {
				if tz.Code == m.timezone && i < len(m.timezones)-1 {
					m.timezone = m.timezones[i+1].Code
					updated = true
					break
				}
			}
		}
	case "left", "esc":
		m.currentStep = stepKeyboardLayout
	}

	if updated {
		m.refreshTimezoneViewport()
	}

	return m, nil
}

func (m setupModel) handleStorageDeviceInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.storageDevice != "" {
			m.currentStep = stepBinaryCache
			m.err = nil
		} else {
			m.err = fmt.Errorf("please select a storage device")
		}
	case "up", "k":
		if len(m.storageDevices) > 0 {
			for i, device := range m.storageDevices {
				if device.Name == m.storageDevice && i > 0 {
					m.storageDevice = m.storageDevices[i-1].Name
					break
				}
			}
		}
	case "down", "j":
		if len(m.storageDevices) > 0 {
			for i, device := range m.storageDevices {
				if device.Name == m.storageDevice && i < len(m.storageDevices)-1 {
					m.storageDevice = m.storageDevices[i+1].Name
					break
				}
			}
		}
	case "left", "esc":
		m.currentStep = stepTimezone
	}
	return m, nil
}

func (m setupModel) handleBinaryCacheInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.currentStep = stepPassword
		m.err = nil
	case "tab":
		// Toggle between OS and Pups selection
		// This is handled in the view, we track which is selected
	case "space":
		// Toggle the currently selected option
		// For simplicity, we'll toggle both with space
		m.binaryCacheOS = !m.binaryCacheOS
	case "1":
		m.binaryCacheOS = !m.binaryCacheOS
	case "2":
		m.binaryCachePups = !m.binaryCachePups
	case "left", "esc":
		m.currentStep = stepStorageDevice
	}
	return m, nil
}

func (m setupModel) handlePasswordInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.password) >= 8 {
			m.currentStep = stepPasswordConfirm
			m.err = nil
		} else {
			m.err = fmt.Errorf("password must be at least 8 characters")
		}
	case "backspace":
		if len(m.password) > 0 {
			m.password = m.password[:len(m.password)-1]
		}
	case "tab":
		m.showPassword = !m.showPassword
	case "left", "esc":
		m.currentStep = stepBinaryCache
		m.password = ""
		m.showPassword = false
	default:
		if len(msg.String()) == 1 {
			m.password += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handlePasswordConfirmInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.passwordConfirm == m.password {
			m.currentStep = stepGenerateKey
			m.err = nil
			// Generate the master key
			return m, generateMasterKeyCmd(m.password)
		} else {
			m.err = fmt.Errorf("passwords do not match")
		}
	case "backspace":
		if len(m.passwordConfirm) > 0 {
			m.passwordConfirm = m.passwordConfirm[:len(m.passwordConfirm)-1]
		}
	case "tab":
		m.showPassword = !m.showPassword
	case "left", "esc":
		m.currentStep = stepPassword
		m.passwordConfirm = ""
		m.showPassword = false
	default:
		if len(msg.String()) == 1 {
			m.passwordConfirm += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handleSeedDisplayInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.currentStep = stepConfirmSeed
		// Generate random word index (1-based for user display)
		fixedSeedIndex := os.Getenv("DEV_SEED_WORD_INDEX")
		if fixedSeedIndex != "" {
			m.seedWordIndex, _ = strconv.Atoi(fixedSeedIndex)
		} else {
			m.seedWordIndex = rand.Intn(len(m.masterKeySeed)) + 1
		}
		m.seedConfirmation = ""
		m.err = nil
	case "left", "esc":
		// Can't go back from seed display
		m.err = fmt.Errorf("seed phrase has been generated, please continue")
	}
	return m, nil
}

func (m setupModel) handleSeedConfirmInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Verify the single word (seedWordIndex is 1-based)
		expectedWord := m.masterKeySeed[m.seedWordIndex-1]
		if strings.TrimSpace(m.seedConfirmation) == expectedWord {
			m.currentStep = stepSelectNetwork
			m.err = nil
			// Fetch available networks
			return m, fetchNetworksCmd()
		} else {
			m.err = fmt.Errorf("incorrect word - please check your seed phrase")
		}
	case "backspace":
		if len(m.seedConfirmation) > 0 {
			m.seedConfirmation = m.seedConfirmation[:len(m.seedConfirmation)-1]
		}
	case "left", "esc":
		m.currentStep = stepDisplaySeed
		m.seedConfirmation = ""
	default:
		// Only allow lowercase letters and hyphens (common in seed words)
		if len(msg.String()) == 1 && (msg.String()[0] >= 'a' && msg.String()[0] <= 'z' || msg.String()[0] == '-') {
			m.seedConfirmation += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handleNetworkInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.selectedNetworkIdx >= 0 && m.selectedNetworkIdx < len(m.availableNetworks) {
			selected := m.availableNetworks[m.selectedNetworkIdx]
			m.selectedNetwork = selected.SSID
			m.networkType = selected.Type
			m.networkInterface = selected.Interface
			m.networkEncryption = selected.Security

			// If WiFi with security, ask for password
			if selected.Type == "wifi" && selected.Security != "" && selected.Security != "open" {
				m.currentStep = stepNetworkPassword
				m.networkPassword = ""
				m.err = nil
			} else {
				// Ethernet or open WiFi - proceed to finalization
				m.currentStep = stepFinalizing
				m.isProcessing = true
				m.setupStepsComplete = make([]bool, 8) // 8 steps in finalization
				m.err = nil
				return m, tea.Batch(
					finalizeSetupCmd(m),
					tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
				)
			}
		}
	case "up", "k":
		if m.selectedNetworkIdx > 0 {
			m.selectedNetworkIdx--
		}
	case "down", "j":
		if m.selectedNetworkIdx < len(m.availableNetworks)-1 {
			m.selectedNetworkIdx++
		}
	case "s":
		// Skip network selection
		m.selectedNetwork = ""
		m.networkType = ""
		m.currentStep = stepFinalizing
		m.isProcessing = true
		m.setupStepsComplete = make([]bool, 8) // 8 steps in finalization
		return m, tea.Batch(
			finalizeSetupCmd(m),
			tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
		)
	case "left", "esc":
		m.currentStep = stepConfirmSeed
	}
	return m, nil
}

func (m setupModel) handleNetworkPasswordInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.networkPassword != "" {
			// Proceed to finalization with password
			m.currentStep = stepFinalizing
			m.isProcessing = true
			m.setupStepsComplete = make([]bool, 8) // 8 steps in finalization
			m.err = nil
			return m, tea.Batch(
				finalizeSetupCmd(m),
				tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
			)
		} else {
			m.err = fmt.Errorf("password cannot be empty")
		}
	case "backspace":
		if len(m.networkPassword) > 0 {
			m.networkPassword = m.networkPassword[:len(m.networkPassword)-1]
		}
	case "left", "esc":
		m.currentStep = stepSelectNetwork
		m.networkPassword = ""
	default:
		if len(msg.String()) == 1 {
			m.networkPassword += msg.String()
		}
	}
	return m, nil
}

// refreshKeyboardViewport rebuilds the keyboard list view and keeps the
// selected layout in view.
func (m *setupModel) refreshKeyboardViewport() {
	m.keyboardVP.SetContent(m.keyboardOptionsContent())
	m.ensureKeyboardSelectionVisible()
}

// refreshTimezoneViewport rebuilds the timezone list view and keeps the
// selected timezone in view.
func (m *setupModel) refreshTimezoneViewport() {
	m.timezoneVP.SetContent(m.timezoneOptionsContent())
	m.ensureTimezoneSelectionVisible()
}

// ensureKeyboardSelectionVisible clamps the keyboard viewport offset so the
// current selection stays on-screen.
func (m *setupModel) ensureKeyboardSelectionVisible() {
	selectedIdx := 0
	for i, layout := range m.keyboardLayouts {
		if layout.Code == m.keyboardLayout {
			selectedIdx = i
			break
		}
	}
	m.adjustViewportOffset(&m.keyboardVP, selectedIdx, len(m.keyboardLayouts))
}

// ensureTimezoneSelectionVisible clamps the timezone viewport offset so the
// current selection stays on-screen.
func (m *setupModel) ensureTimezoneSelectionVisible() {
	selectedIdx := 0
	for i, tz := range m.timezones {
		if tz.Code == m.timezone {
			selectedIdx = i
			break
		}
	}
	m.adjustViewportOffset(&m.timezoneVP, selectedIdx, len(m.timezones))
}

// adjustViewportOffset scrolls the viewport so the selected index is visible.
func (m *setupModel) adjustViewportOffset(vp *viewport.Model, selected, total int) {
	if vp.Height <= 0 || total == 0 {
		vp.SetYOffset(0)
		return
	}

	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}

	maxYOffset := total - vp.Height
	if maxYOffset < 0 {
		maxYOffset = 0
	}

	// Scroll up if selection moved above current view
	if selected < vp.YOffset {
		vp.SetYOffset(selected)
		return
	}

	// Scroll down if selection moved below current view
	if selected >= vp.YOffset+vp.Height {
		vp.SetYOffset(minInt(selected-vp.Height+1, maxYOffset))
		return
	}

	// Clamp if window shrunk
	if vp.YOffset > maxYOffset {
		vp.SetYOffset(maxYOffset)
	}
}

// keyboardOptionsContent builds the content string for the keyboard viewport.
func (m setupModel) keyboardOptionsContent() string {
	if len(m.keyboardLayouts) == 0 {
		return normalStyle.Render("  No keyboard layouts found")
	}

	var options []string
	for _, layout := range m.keyboardLayouts {
		line := fmt.Sprintf("  %s - %s", layout.Code, layout.Name)
		if layout.Code == m.keyboardLayout {
			line = selectedStyle.Render("▸ " + line[2:])
		} else {
			line = normalStyle.Render(line)
		}
		options = append(options, line)
	}
	return strings.Join(options, "\n")
}

// timezoneOptionsContent builds the content string for the timezone viewport.
func (m setupModel) timezoneOptionsContent() string {
	if len(m.timezones) == 0 {
		return normalStyle.Render("  No timezones found")
	}

	var options []string
	for _, tz := range m.timezones {
		line := fmt.Sprintf("  %s - %s", tz.Code, tz.Name)
		if tz.Code == m.timezone {
			line = selectedStyle.Render("▸ " + line[2:])
		} else {
			line = normalStyle.Render(line)
		}
		options = append(options, line)
	}
	return strings.Join(options, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
