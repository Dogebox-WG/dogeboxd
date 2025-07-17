package dbxdev

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dogeorg/dogeboxd/pkg/version"
)

// Style definitions
var (
	headerStyle      = lipgloss.NewStyle().Bold(true)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	borderStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	statusBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("240")).Padding(0, 1)
	pupBoxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Margin(0, 0, 1, 0)
	nameStyle        = lipgloss.NewStyle().Bold(true)
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// ASCII banner
var asciiBanner = `+===================================================+
|                                                   |
|      ____   ___   ____ _____ ____   _____  __     |
|     |  _ \ / _ \ / ___| ____| __ ) / _ \ \/ /     |
|     | | | | | | | |  _|  _| |  _ \| | | |  /      |
|     | |_| | |_| | |_| | |___| |_) | |_| /  \      |
|     |____/ \___/ \____|_____|____/ \___/_/\_\     |
|               Development Console                 |
|                                                   |
+===================================================+`

const leftIndent = " "

// buildBannerWithVersion merges the banner & version info.
func buildBannerWithVersion() (string, int) {
	asciiLines := strings.Split(asciiBanner, "\n")

	info := version.GetDBXRelease()
	verLines := []string{
		fmt.Sprintf("Release: %s", info.Release),
		fmt.Sprintf("Commit: %s", info.Git.Commit),
	}
	if info.Git.Dirty {
		verLines = append(verLines, "Dirty build: true")
	}

	keys := make([]string, 0, len(info.Packages))
	for k := range info.Packages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := info.Packages[k]
		verLines = append(verLines, fmt.Sprintf("%s: %s (%s)", k, p.Rev, p.Hash))
	}

	// bottom align version info with banner
	if len(verLines) < len(asciiLines) {
		pad := len(asciiLines) - len(verLines)
		verLines = append(make([]string, pad), verLines...)
	}

	maxLines := len(asciiLines)
	if len(verLines) > maxLines {
		maxLines = len(verLines)
		for len(asciiLines) < maxLines {
			asciiLines = append(asciiLines, "")
		}
	}
	for len(verLines) < maxLines {
		verLines = append(verLines, "")
	}

	// width of banner for alignment
	maxW := 0
	for _, l := range asciiLines {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}

	combined := make([]string, maxLines)
	for i := 0; i < maxLines; i++ {
		asciiPart := fmt.Sprintf("%-*s", maxW, asciiLines[i])
		combined[i] = asciiPart + "  " + verLines[i]
	}

	return strings.Join(combined, "\n"), maxLines
}

// indentLines prefixes each line with leftIndent.
func indentLines(s string) string {
	return leftIndent + strings.ReplaceAll(s, "\n", "\n"+leftIndent)
}

// View renders the entire UI.
func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	switch m.view {
	case viewConnectionError:
		return m.renderConnectionErrorView()
	case viewPupDetail:
		return m.renderPupDetailView()
	case viewCreatePup:
		return m.renderCreatePupView()
	case viewLogs:
		return m.renderLogsView()
	case viewRebuild:
		return m.renderRebuildView()
	case viewTemplateSelect:
		return m.renderTemplateSelectView()
	case viewNameInput:
		return m.renderNameInputView()
	case viewPasswordInput:
		return m.renderPasswordInputView()
	case viewTaskProgress:
		return m.renderTaskProgressView()
	case viewSourceList:
		return m.renderSourceListView()
	case viewSourceCreate:
		return m.renderSourceCreateView()
	case viewSourceDetail:
		return m.renderSourceDetailView()
	default:
		return m.renderLandingView()
	}
}

// renderLandingView composes the main landing page.
func (m model) renderLandingView() string {
	headerLine := headerStyle.Render("Available Actions:")
	actions := []string{"c: create pup", "s: search pups", "r: rebuild system", "u: sources"}
	actionsLine := strings.Join(actions, "\n")
	if m.searching {
		actionsLine += "\nSearch: " + m.searchQuery
	}

	body := m.renderPups()

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	helpText := "q: quit   c: create   s: search   r: rebuild   u: sources   ↑/↓: select   enter: details"
	if m.searching {
		helpText = "esc: cancel   type to search"
	}
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  " + helpText)

	banner, bannerLines := buildBannerWithVersion()

	headLines := bannerLines + 2 + 1 + len(actions) + 2
	if m.searching {
		headLines++
	}
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := headLines + bodyLines + 1

	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(headerLine) + "\n" + indentLines(actionsLine) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderPupDetailView composes the detail screen for a selected pup.
func (m model) renderPupDetailView() string {
	// Build body with pup details and actions list
	detailText := m.renderDetail()

	// Build dynamic actions
	actions := []string{"View Logs"}
	if m.detail.Enabled {
		actions = append(actions, "Disable pup")
	} else {
		actions = append(actions, "Enable pup")
	}

	// Render actions with selection markers
	actLines := make([]string, len(actions))
	for i, a := range actions {
		mark := "[ ]"
		if i == m.selDetail {
			mark = "[x]"
		}
		actLines[i] = fmt.Sprintf("%s %s", mark, a)
	}
	actionsBlock := strings.Join(actLines, "\n")

	body := detailText + "\n\n" + actionsBlock

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  esc: back   q: quit")

	banner, bannerLines := buildBannerWithVersion()

	headLines := bannerLines + 2 // banner + gap before body
	headLines += 0               // no header/actions here
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := headLines + bodyLines + 1

	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderCreatePupView composes the create pup placeholder screen.
func (m model) renderCreatePupView() string {
	body := "Create Pup (coming soon...)"

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  esc: back   q: quit")

	banner, bannerLines := buildBannerWithVersion()

	headLines := bannerLines + 2
	bodyLines := 1
	totalLines := headLines + bodyLines + 1

	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderLogsView shows full-screen log output (read-only)
func (m model) renderLogsView() string {
	banner, bannerLines := buildBannerWithVersion()

	labelLine := fmt.Sprintf("Logs for pup %s (%s)", m.detail.Name, m.detail.ID)

	// space for logs box (leave 1 gap before help bar)
	availableLines := m.height - bannerLines - 3 /*gaps*/ - 1 /*label*/ - 1 /*help*/
	if availableLines < 1 {
		availableLines = 1
	}

	// interior lines (box adds 2)
	interiorLines := availableLines - 2
	if interiorLines < 0 {
		interiorLines = 0
	}

	logs := m.logs
	if len(logs) > interiorLines {
		logs = logs[len(logs)-interiorLines:]
	}
	bodyContent := strings.Join(logs, "") // logs include \n

	logsBox := borderStyle.Width(m.width - 4).Render(bodyContent)

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  esc: back   q: quit")

	// recompute padding
	bodyLines := strings.Count(logsBox, "\n") + 1
	total := bannerLines + 3 + 1 + bodyLines + 1 // banner + gap + label + box + help
	padding := ""
	if total < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-total)
	}

	return indentLines(banner) + "\n\n" + indentLines(labelLine) + "\n" + indentLines(logsBox) + padding + "\n" + indentLines(help)
}

// renderRebuildView shows the system rebuild output.
func (m model) renderRebuildView() string {
	banner, bannerLines := buildBannerWithVersion()

	labelLine := "System rebuild in progress..."
	if m.rebuildComplete {
		labelLine = "System rebuild complete - press ESC to return"
	}

	// space for logs box (leave 1 gap before help bar)
	availableLines := m.height - bannerLines - 3 /*gaps*/ - 1 /*label*/ - 1 /*help*/
	if availableLines < 1 {
		availableLines = 1
	}

	// interior lines (box adds 2)
	interiorLines := availableLines - 2
	if interiorLines < 1 {
		interiorLines = 1
	}

	logs := m.logs
	if len(logs) > interiorLines {
		logs = logs[len(logs)-interiorLines:]
	}
	bodyContent := strings.Join(logs, "") // logs include \n

	logsBox := borderStyle.Width(m.width - 4).Render(bodyContent)

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  esc: back   q: quit")

	// recompute padding
	bodyLines := strings.Count(logsBox, "\n") + 1
	total := bannerLines + 3 + 1 + bodyLines + 1 // banner + gap + label + box + help
	padding := ""
	if total < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-total)
	}

	return indentLines(banner) + "\n\n" + indentLines(labelLine) + "\n" + indentLines(logsBox) + padding + "\n" + indentLines(help)
}

// renderPups creates the list view for pups.
func (m model) renderPups() string {
	filtered := []pupInfo{}
	for _, p := range m.pups {
		if m.searchQuery == "" || strings.Contains(strings.ToLower(p.Name), strings.ToLower(m.searchQuery)) {
			filtered = append(filtered, p)
		}
	}

	if len(filtered) == 0 {
		if m.searchQuery != "" {
			return "No pups match your search query"
		}
		return "No pups installed"
	}

	cardWidth := m.width - 5 // account for indent + borders
	boxes := make([]string, 0, len(filtered))

	if m.selected >= len(filtered) {
		m.selected = 0
	}

	for idx, p := range filtered {
		enabled := "disabled"
		if p.Enabled {
			enabled = "enabled"
		}

		// derive status considering Enabled flag
		stateTxt := strings.ToLower(p.State)
		if stateTxt == "ready" && !p.Enabled {
			stateTxt = "stopped"
		}

		// status colour
		statusCol := lipgloss.Color("1")
		switch stateTxt {
		case "running", "ready":
			statusCol = lipgloss.Color("10")
		case "starting", "installing":
			statusCol = lipgloss.Color("11")
		case "stopped", "uninstalled":
			statusCol = lipgloss.Color("8")
		}
		statusStyled := lipgloss.NewStyle().Foreground(statusCol).Render(strings.ToUpper(stateTxt))

		left := nameStyle.Render(p.Name)
		right := statusStyled
		checkbox := "[ ]"
		if idx == m.selected {
			checkbox = "[x]"
		}

		spaceAvail := cardWidth - 2 - lipgloss.Width(left) - lipgloss.Width(right) - 1 - lipgloss.Width(checkbox)
		if spaceAvail < 1 {
			spaceAvail = 1
		}
		header := left + strings.Repeat(" ", spaceAvail) + right + " " + checkbox

		var devLabel string
		if p.DevEnabled {
			devLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("DEV MODE ENABLED")
		} else if p.DevAvailable {
			devLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("DEV MODE AVAILABLE")
		}

		detailLeft := enabled
		if p.Error != "" {
			detailLeft += " | " + p.Error
		}
		gap2 := cardWidth - 2 - lipgloss.Width(detailLeft) - lipgloss.Width(devLabel)
		if gap2 < 1 {
			gap2 = 1
		}
		detailLine := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(detailLeft+strings.Repeat(" ", gap2)) + devLabel

		boxContent := header + "\n" + detailLine
		boxes = append(boxes, pupBoxStyle.Width(cardWidth).Render(boxContent))
	}

	return strings.Join(boxes, "\n")
}

func (m model) renderDetail() string {
	p := m.detail
	lines := []string{
		fmt.Sprintf("Name: %s", p.Name),
		fmt.Sprintf("State: %s", p.State),
		fmt.Sprintf("Enabled: %v", p.Enabled),
	}
	if p.DevEnabled {
		lines = append(lines, "Development Mode: ENABLED")
	} else if p.DevAvailable {
		lines = append(lines, "Development Mode: AVAILABLE")
	}
	if p.Error != "" {
		lines = append(lines, "Error: "+p.Error)
	}
	return strings.Join(lines, "\n")
}

// renderTemplateSelectView shows the list of available pup templates
func (m model) renderTemplateSelectView() string {
	banner, bannerLines := buildBannerWithVersion()

	var body string
	if m.templates == nil {
		body = "Loading templates..."
	} else if len(m.templates) == 0 {
		body = "No templates found."
	} else {
		title := headerStyle.Render("Select a Pup Template:")

		// Create template list
		var items []string
		for i, tpl := range m.templates {
			prefix := "  "
			if i == m.selectedTpl {
				prefix = "> "
			}

			line := prefix + tpl.Name
			if i == m.selectedTpl {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(line)
			}
			items = append(items, line)
		}

		list := strings.Join(items, "\n")
		body = title + "\n\n" + list
	}

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  ↑/↓: select   enter: confirm   esc: cancel")

	// Calculate padding
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + 2 + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderNameInputView shows the name input screen
func (m model) renderNameInputView() string {
	banner, bannerLines := buildBannerWithVersion()

	selectedTemplate := ""
	if m.selectedTpl < len(m.templates) {
		selectedTemplate = m.templates[m.selectedTpl].Name
	}

	title := headerStyle.Render(fmt.Sprintf("Creating pup from template: %s", selectedTemplate))

	// Determine target directory
	var devDir string
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		devDir = filepath.Join(dataDir, "dev")
	} else {
		homeDir, _ := os.UserHomeDir()
		devDir = filepath.Join(homeDir, "dev")
	}

	prompt := "Enter pup name: " + m.pupName
	if m.cloning {
		prompt = "Cloning template... Please wait."
	}

	location := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(fmt.Sprintf("Location: %s/<name>", devDir))

	var errLine string
	if m.nameInputErr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+m.nameInputErr)
	}

	body := title + "\n\n" + location + "\n\n" + prompt + errLine

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	helpText := "type name   enter: create   esc: cancel"
	if m.cloning {
		helpText = "cloning..."
	}
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  " + helpText)

	// Calculate padding
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + 2 + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderPasswordInputView shows the password input screen
func (m model) renderPasswordInputView() string {
	banner, bannerLines := buildBannerWithVersion()

	var title, subtitle string
	if m.templates == nil && m.pupName == "" {
		// Initial authentication for create pup flow
		title = headerStyle.Render("Create New Pup")
		subtitle = "Enter your Dogebox password to continue"
	} else {
		// This is the fallback case (shouldn't happen in new flow)
		title = headerStyle.Render("Authentication Required")
		subtitle = "Enter your Dogebox password to install the pup"
	}

	// Show asterisks for password
	var maskedPassword string
	for range m.password {
		maskedPassword += "*"
	}

	prompt := "Password: " + maskedPassword
	if m.authenticating {
		prompt = "Authenticating... Please wait."
	}

	var errLine string
	if m.passwordErr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+m.passwordErr)
	}

	body := title + "\n\n" + subtitle + "\n\n" + prompt + errLine

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	helpText := "type password   enter: authenticate   esc: cancel"
	if m.authenticating {
		helpText = "authenticating..."
	}
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  " + helpText)

	// Calculate padding
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + 2 + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderTaskProgressView shows the task progress screen
func (m model) renderTaskProgressView() string {
	banner, bannerLines := buildBannerWithVersion()

	title := headerStyle.Render("Creating Pup: " + m.pupName)

	// Spinner animation frames
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerFrame := spinnerFrames[int(time.Now().UnixMilli()/100)%len(spinnerFrames)]

	var tasks []string
	for _, task := range m.tasks {
		var icon string
		var color lipgloss.Color

		switch task.Status {
		case taskPending:
			icon = "[ ]"
			color = "7" // default
		case taskRunning:
			icon = "[" + spinnerFrame + "]"
			color = "11" // yellow
		case taskSuccess:
			icon = "[✓]"
			color = "10" // green
		case taskFailed:
			icon = "[✗]"
			color = "9" // red
		}

		line := lipgloss.NewStyle().Foreground(color).Render(icon) + " " + task.Name
		tasks = append(tasks, line)

		// Add error message if task failed
		if task.Status == taskFailed && task.Error != "" {
			errLine := "    " + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+task.Error)
			tasks = append(tasks, errLine)
		}
	}

	taskSection := title + "\n\n" + strings.Join(tasks, "\n")

	// Calculate heights for split view
	totalHeight := m.height - bannerLines - 3 // -3 for help bar and margins
	taskHeight := len(tasks) + 3              // +3 for title and spacing

	// Split remaining space between tasks and logs
	logsHeight := totalHeight - taskHeight
	if logsHeight < 5 {
		logsHeight = 5
	}
	if logsHeight > totalHeight/2 {
		logsHeight = totalHeight / 2
	}

	// Create logs section
	logsTitle := "Installation Logs:"
	var logsContent []string

	// Show last N lines that fit in the log area
	logLines := logsHeight - 2 // -2 for title and separator
	if logLines > 0 && len(m.taskLogs) > 0 {
		start := 0
		if len(m.taskLogs) > logLines {
			start = len(m.taskLogs) - logLines
		}
		for i := start; i < len(m.taskLogs); i++ {
			logsContent = append(logsContent, m.taskLogs[i])
		}
	}

	logSection := logsTitle + "\n" + strings.Repeat("─", m.width-2) + "\n" + strings.Join(logsContent, "\n")

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	helpText := "please wait..."
	if m.allTasksDone {
		helpText = "esc: back to main"
	}
	help := statusBarStyle.Width(m.width - 1).Render(metrics + "  |  " + helpText)

	// Calculate padding between sections
	usedLines := bannerLines + 2 + len(tasks) + 3 + 2 + len(logsContent) + 3 + 1
	padding := ""
	if usedLines < m.height {
		paddingLines := m.height - usedLines
		padding = strings.Repeat("\n"+leftIndent, paddingLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(taskSection) + "\n\n" + indentLines(logSection) + padding + "\n" + indentLines(help)
}

// renderConnectionErrorView shows the connection error screen
func (m model) renderConnectionErrorView() string {
	banner, bannerLines := buildBannerWithVersion()

	title := headerStyle.Render("Connection Error")

	socketPath := m.socketPath
	if socketPath == "" {
		socketPath = "<unknown>"
	}

	errorMsg := fmt.Sprintf("Cannot connect to Dogeboxd unix socket at %s", socketPath)

	var details string
	if m.connectionErr != "" {
		details = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Error: "+m.connectionErr)
	}

	body := title + "\n\n" + errorMsg + details

	help := statusBarStyle.Width(m.width - 1).Render("r: retry   q: quit   ctrl+c: quit")

	// Calculate padding
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + 2 + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return indentLines(banner) + "\n\n" + indentLines(body) + padding + "\n" + indentLines(help)
}

// renderSourceListView renders the source list screen
func (m model) renderSourceListView() string {
	banner, bannerLines := buildBannerWithVersion()
	title := headerStyle.Render("Source List")

	// Add Available Actions section
	actionsHeader := headerStyle.Render("Available Actions:")
	actions := []string{"c: create source", "enter: view details", "esc: back to main"}
	actionsLine := strings.Join(actions, "\n")

	// Build source list
	var content strings.Builder
	if len(m.sources) == 0 {
		content.WriteString(leftIndent + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(" No sources configured") + "\n")
	} else {
		for i, source := range m.sources {
			cursor := "  "
			var style lipgloss.Style
			if i == m.selectedSource {
				cursor = "→ "
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
			} else {
				style = lipgloss.NewStyle()
			}

			line := fmt.Sprintf("%s%s %s", leftIndent, cursor, source.Name)
			if source.Type != "" {
				line += fmt.Sprintf(" (%s)", source.Type)
			}
			content.WriteString(style.Render(line) + "\n")

			// Show description and location for selected source
			if i == m.selectedSource && source.Description != "" {
				content.WriteString(leftIndent + "    " + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(source.Description) + "\n")
			}
			if i == m.selectedSource {
				content.WriteString(leftIndent + "    " + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(source.Location) + "\n")
			}
		}
	}

	help := statusBarStyle.Width(m.width - 1).Render("c: create   enter: details   esc: back   q: quit")

	// Calculate padding
	body := leftIndent + title + "\n\n" + leftIndent + actionsHeader + "\n" + indentLines(actionsLine) + "\n\n" + content.String()
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return leftIndent + banner + "\n\n" + body + padding + help
}

// renderSourceCreateView renders the source creation screen
func (m model) renderSourceCreateView() string {
	banner, bannerLines := buildBannerWithVersion()
	title := headerStyle.Render("Create New Source")

	var content string
	var helpText string

	if m.creatingSource {
		content = leftIndent + "Creating source... Please wait."
		helpText = "creating..."
	} else {
		content = leftIndent + "Enter source URL:\n\n"
		content += leftIndent + "> " + m.sourceInput
		helpText = "enter: create   esc: cancel"
	}

	help := statusBarStyle.Width(m.width - 1).Render(helpText)

	// Calculate padding
	body := leftIndent + title + "\n\n" + content
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return leftIndent + banner + "\n\n" + body + padding + help
}

// renderSourceDetailView renders the source detail screen
func (m model) renderSourceDetailView() string {
	banner, bannerLines := buildBannerWithVersion()

	if m.selectedSource >= len(m.sources) {
		return leftIndent + banner + "\n\n" + leftIndent + "Invalid source"
	}

	source := m.sources[m.selectedSource]
	title := headerStyle.Render("Source Details")

	var content strings.Builder

	if m.deletingSource {
		content.WriteString(leftIndent + "Deleting source... Please wait.\n")
	} else {
		content.WriteString(leftIndent + "Name: " + source.Name + "\n")
		if source.Description != "" {
			content.WriteString(leftIndent + "Description: " + source.Description + "\n")
		}
		content.WriteString(leftIndent + "Type: " + source.Type + "\n")
		content.WriteString(leftIndent + "Location: " + source.Location + "\n")
		content.WriteString(leftIndent + "ID: " + dimStyle.Render(source.ID) + "\n")
	}

	helpText := "d: delete   esc: back   q: quit"
	if m.deletingSource {
		helpText = "deleting..."
	}
	help := statusBarStyle.Width(m.width - 1).Render(helpText)

	// Calculate padding
	body := leftIndent + title + "\n\n" + content.String()
	bodyLines := strings.Count(body, "\n") + 1
	totalLines := bannerLines + bodyLines + 1
	padding := ""
	if totalLines < m.height {
		padding = strings.Repeat("\n"+leftIndent, m.height-totalLines)
	}

	return leftIndent + banner + "\n\n" + body + padding + help
}
