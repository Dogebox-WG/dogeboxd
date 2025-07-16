package dbxdev

import (
	"fmt"
	"sort"
	"strings"

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
	case viewPupDetail:
		return m.renderPupDetailView()
	case viewCreatePup:
		return m.renderCreatePupView()
	case viewLogs:
		return m.renderLogsView()
	case viewRebuild:
		return m.renderRebuildView()
	default:
		return m.renderLandingView()
	}
}

// renderLandingView composes the main landing page.
func (m model) renderLandingView() string {
	headerLine := headerStyle.Render("Available Actions:")
	actions := []string{"c: create pup", "s: search pups", "r: rebuild system"}
	actionsLine := strings.Join(actions, "\n")
	if m.searching {
		actionsLine += "\nSearch: " + m.searchQuery
	}

	body := m.renderPups()

	metrics := fmt.Sprintf("CPU %.0f%%  Mem %d/%dMB", m.cpuPercent, m.memUsed, m.memTotal)
	helpText := "q: quit   c: create pup   s: search   r: rebuild   ↑/↓: select   enter: details"
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
