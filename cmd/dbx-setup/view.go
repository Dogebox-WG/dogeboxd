package dbxsetup

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("86")).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	seedWordStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1).
			Margin(0, 1)

	progressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

func (m setupModel) renderCheckingStatusStep() string {
	title := titleStyle.Render("Checking System Status")
	subtitle := progressStyle.Render("Please wait while we check if your system needs configuration...")

	spinner := progressStyle.Render("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		spinner,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderReadyStep() string {
	title := titleStyle.Render("Welcome to Dogebox Setup")
	subtitle := subtitleStyle.Render("Your system needs initial configuration")

	description := normalStyle.Render(
		"This setup wizard will guide you through configuring your Dogebox.\n" +
			"You'll need to:\n\n" +
			"  • Choose a device name\n" +
			"  • Select your keyboard layout\n" +
			"  • Select your timezone\n" +
			"  • Configure storage\n" +
			"  • Create a password\n" +
			"  • Save your recovery seed phrase\n" +
			"  • Configure network (optional)")

	prompt := successStyle.Render("Ready to begin?")

	help := helpStyle.Render("Enter: Start Setup • Ctrl+C: Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		description,
		"",
		prompt,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderAlreadyConfiguredStep() string {
	// Create a prominent error style
	errorBoxStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")). // Bright red
		Padding(1, 3)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")) // Red

	title := titleStyle.Render("System Already Configured")

	warning := errorBoxStyle.Render("⚠️  ERROR  ⚠️")

	message := normalStyle.Render(
		"This Dogebox system has already been configured.\n\n" +
			"Running setup again could damage your existing configuration\n" +
			"and potentially cause data loss.\n\n" +
			"If you need to reconfigure your system, please use the\n" +
			"recovery mode or contact support.")

	help := helpStyle.Render("Enter/Q: Exit • Ctrl+C: Exit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		warning,
		"",
		title,
		"",
		message,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderDeviceNameStep() string {
	title := titleStyle.Render("Device Name")
	subtitle := subtitleStyle.Render("Choose a name for your Dogebox device")

	input := inputStyle.Width(40).Render(m.deviceName)

	help := helpStyle.Render("Enter: Continue • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		input,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderKeyboardLayoutStep() string {
	title := titleStyle.Render("Keyboard Layout")
	subtitle := subtitleStyle.Render("Select your keyboard layout")

	body := m.keyboardVP.View()
	if body == "" {
		body = normalStyle.Render("  No keyboard layouts found")
	}

	help := helpStyle.Render("↑/↓: Navigate • Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		body,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderTimezoneStep() string {
	title := titleStyle.Render("Timezone")
	subtitle := subtitleStyle.Render("Select your timezone")

	body := m.timezoneVP.View()
	if body == "" {
		body = normalStyle.Render("  No timezones found")
	}

	help := helpStyle.Render("↑/↓: Navigate • Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		body,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderStorageDeviceStep() string {
	title := titleStyle.Render("Mass Storage Device")
	subtitle := subtitleStyle.Render("Select the storage device for Dogebox data")

	var options []string
	for _, device := range m.storageDevices {
		var displayName string

		// Show boot media indicator as prefix
		if device.BootMedia {
			displayName = "[BOOT] "
		}

		if device.Label != "" {
			displayName += fmt.Sprintf("%s (%s)", device.Name, device.Label)
		} else {
			displayName += device.Name
		}

		line := fmt.Sprintf("  %s - %s", displayName, device.SizePretty)
		if device.Name == m.storageDevice {
			line = selectedStyle.Render("▸ " + line[2:])
		} else {
			line = normalStyle.Render(line)
		}
		options = append(options, line)
	}

	if len(options) == 0 {
		options = append(options, normalStyle.Render("  No storage devices found"))
	}

	help := helpStyle.Render("↑/↓: Navigate • Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		strings.Join(options, "\n"),
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderBinaryCacheStep() string {
	title := titleStyle.Render("Binary Cache Configuration")
	subtitle := subtitleStyle.Render("Select which binary caches to enable for faster installation")

	// Checkbox style
	checkedBox := successStyle.Render("[✓]")
	uncheckedBox := normalStyle.Render("[ ]")

	// System packages option
	osBox := uncheckedBox
	if m.binaryCacheOS {
		osBox = checkedBox
	}
	osOption := fmt.Sprintf("%s System Packages", osBox)
	osDesc := subtitleStyle.Render("    Pre-built NixOS packages for system components")

	// Pups option
	pupsBox := uncheckedBox
	if m.binaryCachePups {
		pupsBox = checkedBox
	}
	pupsOption := fmt.Sprintf("%s Pups", pupsBox)
	pupsDesc := subtitleStyle.Render("    Pre-built packages for Dogebox applications")

	explanation := normalStyle.Render(
		"\nBinary caches speed up installation by downloading\n" +
			"pre-built packages instead of compiling from source.\n" +
			"Both options are recommended for most users.")

	help := helpStyle.Render("1: Toggle System • 2: Toggle Pups • Enter: Continue • Esc: Back")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		osOption,
		osDesc,
		"",
		pupsOption,
		pupsDesc,
		explanation,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderPasswordStep() string {
	title := titleStyle.Render("User Password")
	subtitle := subtitleStyle.Render("Create a password for your Dogebox")

	var display string
	if m.showPassword {
		display = m.password
	} else {
		display = strings.Repeat("•", len(m.password))
	}

	input := inputStyle.Width(40).Render(display)

	help := helpStyle.Render("Tab: Show/Hide • Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		"Password must be at least 8 characters",
		"",
		input,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderPasswordConfirmStep() string {
	title := titleStyle.Render("Confirm Password")
	subtitle := subtitleStyle.Render("Re-enter your password to confirm")

	var display string
	if m.showPassword {
		display = m.passwordConfirm
	} else {
		display = strings.Repeat("•", len(m.passwordConfirm))
	}

	input := inputStyle.Width(40).Render(display)

	help := helpStyle.Render("Tab: Show/Hide • Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		input,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderGeneratingKeyStep() string {
	title := titleStyle.Render("Generating Master Key")
	subtitle := progressStyle.Render("Please wait while we generate your secure master key...")

	spinner := progressStyle.Render("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		spinner,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderSeedDisplayStep() string {
	title := titleStyle.Render("Master Key Seed Phrase")
	subtitle := errorStyle.Render("⚠️  IMPORTANT: Write down these words in order. This is your only backup!")

	// Display seed words in a grid
	var seedDisplay []string
	for i := 0; i < len(m.masterKeySeed); i += 4 {
		var row []string
		for j := 0; j < 4 && i+j < len(m.masterKeySeed); j++ {
			word := fmt.Sprintf("%2d. %s", i+j+1, m.masterKeySeed[i+j])
			row = append(row, seedWordStyle.Render(word))
		}
		seedDisplay = append(seedDisplay, lipgloss.JoinHorizontal(lipgloss.Left, row...))
	}

	warning := errorStyle.Render(
		"Never share this seed phrase with anyone!\n" +
			"Anyone with these words can access your Dogebox.")

	help := helpStyle.Render("Enter: Continue (after writing down) • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		strings.Join(seedDisplay, "\n"),
		"",
		warning,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderSeedConfirmStep() string {
	title := titleStyle.Render("Confirm Seed Phrase")

	ordinal := getOrdinal(m.seedWordIndex)
	question := titleStyle.Copy().
		Foreground(lipgloss.Color("220")).
		Render(fmt.Sprintf("What is the %s word in your seed phrase?", ordinal))

	subtitle := subtitleStyle.Render("This confirms you've saved your seed phrase correctly")

	input := inputStyle.Width(30).Render(m.seedConfirmation)

	help := helpStyle.Render("Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		question,
		subtitle,
		"",
		input,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

// getOrdinal returns the ordinal form of a number (1st, 2nd, 3rd, etc.)
func getOrdinal(n int) string {
	suffix := "th"
	switch n % 10 {
	case 1:
		if n%100 != 11 {
			suffix = "st"
		}
	case 2:
		if n%100 != 12 {
			suffix = "nd"
		}
	case 3:
		if n%100 != 13 {
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func (m setupModel) renderNetworkSelectStep() string {
	title := titleStyle.Render("Network Configuration")
	subtitle := subtitleStyle.Render("Select a network connection (optional)")

	var options []string
	for i, network := range m.availableNetworks {
		var line string

		if network.Type == "ethernet" {
			if network.Interface != "" {
				line = fmt.Sprintf("  🔌 Ethernet (%s)", network.Interface)
			} else {
				line = "  🔌 Ethernet"
			}
		} else {
			// WiFi network
			signal := ""
			if network.Signal > 75 {
				signal = "▂▄▆█"
			} else if network.Signal > 50 {
				signal = "▂▄▆_"
			} else if network.Signal > 25 {
				signal = "▂▄__"
			} else {
				signal = "▂___"
			}

			line = fmt.Sprintf("  %s  %s", signal, network.SSID)
			if network.Security != "" && network.Security != "open" {
				line += fmt.Sprintf(" 🔒 (%s)", network.Security)
			}
		}

		if i == m.selectedNetworkIdx {
			line = selectedStyle.Render("▸ " + line[2:])
		} else {
			line = normalStyle.Render(line)
		}
		options = append(options, line)
	}

	if len(options) == 0 {
		options = append(options, normalStyle.Render("  No networks found"))
	}

	help := helpStyle.Render("↑/↓: Navigate • Enter: Select • S: Skip • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		strings.Join(options, "\n"),
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderNetworkPasswordStep() string {
	title := titleStyle.Render("WiFi Password")

	networkName := m.selectedNetwork
	if networkName == "" {
		networkName = "Selected Network"
	}
	subtitle := subtitleStyle.Render(fmt.Sprintf("Enter password for %s", networkName))

	// Show password as dots
	display := strings.Repeat("•", len(m.networkPassword))
	input := inputStyle.Width(40).Render(display)

	help := helpStyle.Render("Enter: Continue • Esc: Back • Ctrl+C: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		input,
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

func (m setupModel) renderFinalizingStep() string {
	title := titleStyle.Render("Finalizing Setup")
	subtitle := progressStyle.Render("Configuring your Dogebox...")

	steps := []string{
		"Setting device name",
		"Configuring keyboard layout",
		"Setting timezone",
		"Preparing storage device",
		"Setting up binary caches",
		"Creating user account",
		"Configuring network",
		"Finalizing system bootstrap",
	}

	// Spinner frames
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	currentFrame := int(time.Now().UnixMilli()/100) % len(spinnerFrames)

	var display []string
	for i, step := range steps {
		var prefix string
		if i < len(m.setupStepsComplete) && m.setupStepsComplete[i] {
			// Step is complete - show tick
			prefix = successStyle.Render("✓")
		} else if i < len(m.setupStepsComplete) && i == m.getActiveStep() {
			// Currently processing - show spinner
			prefix = progressStyle.Render(spinnerFrames[currentFrame])
		} else {
			// Not yet started - show bullet
			prefix = normalStyle.Render("•")
		}

		stepText := normalStyle.Render(step)
		display = append(display, fmt.Sprintf("  %s %s", prefix, stepText))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		strings.Join(display, "\n"),
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}

// getActiveStep returns the index of the currently active step
func (m setupModel) getActiveStep() int {
	for i, complete := range m.setupStepsComplete {
		if !complete {
			return i
		}
	}
	return len(m.setupStepsComplete) - 1
}

func (m setupModel) renderCompleteStep() string {
	title := successStyle.Render("✓ Setup Complete!")
	subtitle := subtitleStyle.Render("Your Dogebox has been configured successfully")

	var networkDisplay string
	if m.networkType == "ethernet" {
		networkDisplay = fmt.Sprintf("Ethernet (%s)", m.networkInterface)
	} else if m.networkType == "wifi" {
		networkDisplay = fmt.Sprintf("WiFi: %s", m.selectedNetwork)
	} else {
		networkDisplay = "None (Manual configuration)"
	}

	summary := normalStyle.Render(fmt.Sprintf(
		"Device Name: %s\n"+
			"Keyboard Layout: %s\n"+
			"Timezone: %s\n"+
			"Storage Device: %s\n"+
			"System Binary Cache: %s\n"+
			"Pups Binary Cache: %s\n"+
			"Network: %s",
		m.deviceName,
		m.keyboardLayout,
		m.timezone,
		m.storageDevice,
		map[bool]string{true: "Enabled", false: "Disabled"}[m.binaryCacheOS],
		map[bool]string{true: "Enabled", false: "Disabled"}[m.binaryCachePups],
		networkDisplay,
	))

	help := helpStyle.Render("Enter: Exit • Q: Quit")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		summary,
		"",
		successStyle.Render("You can now use 'dbx dev' to start dev on your Dogebox!"),
		"",
		help,
	)

	return " " + strings.ReplaceAll(content, "\n", "\n ")
}
