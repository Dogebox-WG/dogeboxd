package dbxdev

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// model holds all UI state for the dashboard.
type model struct {
	width, height int
	cpuPercent    float64
	memUsed       uint64
	memTotal      uint64
	pups          []pupInfo

	selected int

	view      viewState
	detail    pupInfo
	selDetail int // selected action index in detail view

	logs      []string
	logActive bool // generic tail active flag

	rebuildComplete bool // track if rebuild finished

	searching   bool
	searchQuery string

	// Create pup flow
	templates    []templateInfo
	selectedTpl  int // selected template index
	pupName      string
	nameInputErr string
	cloning      bool

	// Auth flow
	password       string
	passwordErr    string
	authToken      string
	authenticating bool

	// Connection state
	connectionErr string
	socketPath    string

	// Task progress
	tasks        []task
	allTasksDone bool
	taskLogs     []string
	wsConnected  bool
	targetPupID  string // Track pup by name initially, then by ID once available
	installJobID string // Track the job ID for the installation

	// Source management
	sources        []sourceInfo
	selectedSource int
	sourceInput    string
	creatingSource bool
	deletingSource bool

	// Store template name for use in templating
	selectedTemplateName string
}

// Init performs initial setup and returns a command to check dogeboxd connection
func (m model) Init() tea.Cmd {
	// Return a batch of commands to run on startup
	return tea.Batch(
		checkBootstrapCmd(m.socketPath),
		fetchPupsCmd(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

// refreshMetrics fetches CPU & memory metrics immediately.
func (m *model) refreshMetrics() {
	if cpus, _ := cpu.PercentWithContext(context.Background(), 0, false); len(cpus) > 0 {
		m.cpuPercent = cpus[0]
	}
	if v, _ := mem.VirtualMemory(); v != nil {
		m.memUsed = v.Used / (1024 * 1024)
		m.memTotal = v.Total / (1024 * 1024)
	}
}

// Update handles messages and updates the model
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// First, check if we're in any input mode
		isInputMode := m.searching ||
			(m.view == viewNameInput && !m.cloning) ||
			(m.view == viewPasswordInput && !m.authenticating) ||
			m.view == viewSourceCreate

		// Handle special keys that work in all modes
		switch msg.String() {
		case "ctrl+c":
			stopTail()
			return m, tea.Quit
		case "esc":
			if m.view == viewLogs || m.view == viewRebuild {
				stopTail()
				if m.view == viewLogs {
					m.view = viewPupDetail
				} else {
					m.view = viewLanding
				}
			} else if m.view == viewPupDetail || m.view == viewCreatePup || m.view == viewTemplateSelect || m.view == viewNameInput || m.view == viewPasswordInput {
				// Clear auth token if canceling create flow
				if m.view == viewTemplateSelect || m.view == viewNameInput || m.view == viewPasswordInput {
					m.authToken = ""
				}
				m.view = viewLanding
			} else if m.view == viewSourceList {
				m.view = viewLanding
			} else if m.view == viewSourceCreate && !m.creatingSource {
				m.view = viewSourceList
			} else if m.view == viewSourceDetail && !m.deletingSource {
				m.view = viewSourceList
			} else if m.view == viewTaskProgress && m.allTasksDone {
				// Only allow escape when all tasks are done
				m.view = viewLanding
				// Clear auth token after completing create flow
				m.authToken = ""
				// Close websocket if still open
				closeWebSocket()
				// Refresh pup list if tasks were successful
				allSuccess := true
				for _, t := range m.tasks {
					if t.Status != taskSuccess {
						allSuccess = false
						break
					}
				}
				if allSuccess {
					return m, fetchPupsCmd()
				}
			} else if m.searching {
				m.searching = false
				m.searchQuery = ""
			}
		}

		// If we have a connection error, don't process any other keys except quit
		if m.view == viewConnectionError {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "r":
				// Retry connection
				return m, tea.Batch(
					checkBootstrapCmd(m.socketPath),
					fetchPupsCmd(),
				)
			}
			return m, nil
		}

		// If we're in task progress view, only allow escape when done
		if m.view == viewTaskProgress && !m.allTasksDone {
			// Tasks still running, ignore all input
			return m, nil
		}

		// If we're in input mode, handle text input
		if isInputMode {
			switch msg.String() {
			case "enter":
				if m.searching {
					// Search mode stays active after enter
				} else if m.view == viewNameInput && m.pupName != "" {
					// Validate name with new stricter rules
					if len(m.pupName) < 3 {
						m.nameInputErr = "Name must be at least 3 characters"
					} else if len(m.pupName) > 30 {
						m.nameInputErr = "Name must be 30 characters or less"
					} else if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(m.pupName) {
						m.nameInputErr = "Name must contain only lowercase letters, numbers, underscores, and dashes (a-z, 0-9, _, -)"
					} else {
						// Run async validation for existing pup/directory check
						return m, validatePupNameCmd(m.pupName, m.pups)
					}
				} else if m.view == viewPasswordInput && m.password != "" && !m.authenticating {
					// Start authentication
					m.authenticating = true
					return m, authenticateCmd(m.password)
				} else if m.view == viewSourceCreate && m.sourceInput != "" && !m.creatingSource {
					// Create source with the URL
					m.creatingSource = true
					return m, createSourceCmd(m.sourceInput)
				}
			default:
				// Handle text input for each mode
				if m.searching {
					switch msg.Type {
					case tea.KeyBackspace, tea.KeyDelete:
						if len(m.searchQuery) > 0 {
							m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
						}
					case tea.KeyRunes:
						m.searchQuery += msg.String()
					}
				} else if m.view == viewNameInput && !m.cloning {
					switch msg.Type {
					case tea.KeyBackspace, tea.KeyDelete:
						if len(m.pupName) > 0 {
							m.pupName = m.pupName[:len(m.pupName)-1]
							m.nameInputErr = ""
						}
					case tea.KeyRunes:
						m.pupName += msg.String()
						m.nameInputErr = ""
					}
				} else if m.view == viewPasswordInput && !m.authenticating {
					switch msg.Type {
					case tea.KeyBackspace, tea.KeyDelete:
						if len(m.password) > 0 {
							m.password = m.password[:len(m.password)-1]
							m.passwordErr = ""
						}
					case tea.KeyRunes:
						m.password += msg.String()
						m.passwordErr = ""
					}
				} else if m.view == viewSourceCreate && !m.creatingSource {
					switch msg.Type {
					case tea.KeyBackspace, tea.KeyDelete:
						if len(m.sourceInput) > 0 {
							m.sourceInput = m.sourceInput[:len(m.sourceInput)-1]
						}
					case tea.KeyRunes:
						m.sourceInput += msg.String()
					}
				}
			}
			// Don't process action keys when in input mode
			return m, nil
		}

		// Now handle action keys (only when NOT in input mode)
		switch msg.String() {
		case "up", "k":
			if m.view == viewLanding && len(m.pups) > 0 {
				m.selected = (m.selected - 1 + len(m.pups)) % len(m.pups)
			} else if m.view == viewPupDetail {
				m.selDetail = (m.selDetail - 1 + detailActionsCount) % detailActionsCount
			} else if m.view == viewTemplateSelect && len(m.templates) > 0 {
				m.selectedTpl = (m.selectedTpl - 1 + len(m.templates)) % len(m.templates)
			} else if m.view == viewSourceList && len(m.sources) > 0 {
				m.selectedSource = (m.selectedSource - 1 + len(m.sources)) % len(m.sources)
			}
		case "down", "j":
			if m.view == viewLanding && len(m.pups) > 0 {
				m.selected = (m.selected + 1) % len(m.pups)
			} else if m.view == viewPupDetail {
				m.selDetail = (m.selDetail + 1) % detailActionsCount
			} else if m.view == viewTemplateSelect && len(m.templates) > 0 {
				m.selectedTpl = (m.selectedTpl + 1) % len(m.templates)
			} else if m.view == viewSourceList && len(m.sources) > 0 {
				m.selectedSource = (m.selectedSource + 1) % len(m.sources)
			}
		case "enter", "l":
			if m.view == viewLanding && len(m.pups) > 0 {
				m.view = viewPupDetail
				m.detail = m.pups[m.selected]
				m.selDetail = 0
			} else if m.view == viewSourceList && len(m.sources) > 0 {
				m.view = viewSourceDetail
			} else if m.view == viewPupDetail {
				switch m.selDetail {
				case 0:
					if !m.logActive {
						cmd := openLogFileCmd(m.detail.ID)
						m.view = viewLogs
						m.logs = nil
						return m, cmd
					}
				case 1:
					act := "disable"
					if !m.detail.Enabled {
						act = "enable"
					}
					return m, tea.Batch(pupActionCmd(m.detail.ID, act), fetchPupsCmd())
				}
			} else if m.view == viewTemplateSelect && len(m.templates) > 0 {
				// Move to name input
				m.view = viewNameInput
			}
		case "c":
			if m.view == viewLanding {
				// Reset create pup state
				m.templates = nil
				m.selectedTpl = 0
				m.pupName = ""
				m.nameInputErr = ""
				m.password = ""
				m.passwordErr = ""
				m.authToken = ""
				// Go to password input first
				m.view = viewPasswordInput
			} else if m.view == viewSourceList {
				// Switch to source creation mode
				m.view = viewSourceCreate
				m.sourceInput = ""
			}
		case "r":
			if m.view == viewLanding {
				m.view = viewRebuild
				m.rebuildComplete = false
				cmd := startRebuildCmd()
				return m, cmd
			}
		case "u":
			if m.view == viewLanding {
				// Go to source list view
				m.view = viewSourceList
				m.sources = nil
				m.selectedSource = 0
				return m, fetchSourcesCmd()
			}
		case "d":
			if m.view == viewLanding && len(m.pups) > 0 && m.pups[m.selected].DevAvailable {
				mode := "enable"
				if m.pups[m.selected].DevEnabled {
					mode = "disable"
				}
				return m, tea.Batch(pupActionCmd(m.pups[m.selected].ID, "dev-mode-"+mode), fetchPupsCmd())
			} else if m.view == viewSourceDetail && m.selectedSource < len(m.sources) && !m.deletingSource {
				// Delete the selected source
				source := m.sources[m.selectedSource]
				m.deletingSource = true
				return m, deleteSourceCmd(source.ID)
			}
		case "/":
			if m.view == viewLanding {
				m.searching = true
				m.searchQuery = ""
			}
		case "h", "left":
			if m.view == viewPupDetail {
				m.view = viewLanding
			} else if m.view == viewSourceDetail {
				m.view = viewSourceList
			}
		case "q":
			return m, tea.Quit
		case "R":
			m.refreshMetrics()
			return m, fetchPupsCmd()
		}

	case tickMsg:
		// Update metrics
		m.refreshMetrics()

		// Force update if we have running tasks to animate spinner
		hasRunningTasks := false
		if m.view == viewTaskProgress {
			for _, t := range m.tasks {
				if t.Status == taskRunning {
					hasRunningTasks = true
					break
				}
			}
		}

		// Continue ticking
		cmds := []tea.Cmd{
			tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
		}

		// Add a faster tick for spinner animation when tasks are running
		if hasRunningTasks {
			cmds = append(cmds, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return tea.WindowSizeMsg{Width: m.width, Height: m.height}
			}))
		}

		return m, tea.Batch(cmds...)

	case pupsMsg:
		if msg.err == nil {
			m.pups = msg.list
		}
		return m, nil
	case sourcesMsg:
		if msg.err == nil {
			m.sources = msg.sources
			if m.selectedSource >= len(m.sources) {
				m.selectedSource = 0
			}
		}
		return m, nil
	case sourceCreatedMsg:
		m.creatingSource = false
		if msg.err == nil {
			// Source created successfully, go back to source list
			m.view = viewSourceList
			m.sourceInput = ""
			return m, fetchSourcesCmd()
		}
		// TODO: handle error display
		return m, nil
	case sourceDeletedMsg:
		m.deletingSource = false
		if msg.err == nil {
			// Source deleted successfully, go back to source list
			m.view = viewSourceList
			return m, fetchSourcesCmd()
		}
		// TODO: handle error display
		return m, nil
	case logLineMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > 500 {
			m.logs = m.logs[len(m.logs)-500:]
		}
		return m, nil
	case rebuildFinishedMsg:
		// Don't automatically switch back - let user press ESC
		globalActive = false
		m.rebuildComplete = true
		return m, nil
	case templatesMsg:
		if msg.err != nil {
			// Handle error - go back to landing
			m.view = viewLanding
		} else {
			m.templates = msg.templates
		}
		return m, nil
	case cloneCompleteMsg:
		m.cloning = false
		if msg.err != nil {
			// Update task status
			if m.view == viewTaskProgress && len(m.tasks) > 0 {
				m.tasks[0].Status = taskFailed
				m.tasks[0].Error = msg.err.Error()
				m.allTasksDone = true
			} else {
				m.nameInputErr = msg.err.Error()
			}
		} else {
			// Success - update task and proceed to templating
			if m.view == viewTaskProgress && len(m.tasks) > 0 {
				m.tasks[0].Status = taskSuccess
				m.taskLogs = append(m.taskLogs, "Template cloned successfully")

				// Start templating task
				if len(m.tasks) > 1 {
					m.tasks[1].Status = taskRunning
					return m, templateFilesCmd(m.pupName, m.selectedTemplateName)
				}
			}
		}
	case authMsg:
		m.authenticating = false
		if msg.err != nil {
			m.passwordErr = msg.err.Error()
		} else {
			m.authToken = msg.token
			// If we're in the create pup flow (haven't selected a template yet)
			if m.templates == nil && m.pupName == "" {
				// Move to template selection
				m.view = viewTemplateSelect
				return m, fetchTemplatesCmd()
			}
		}
	case wsConnectedMsg:
		if msg.err != nil {
			m.taskLogs = append(m.taskLogs, fmt.Sprintf("Warning: WebSocket connection failed: %v", msg.err))
		} else {
			m.wsConnected = true
			m.taskLogs = append(m.taskLogs, "WebSocket connected")
		}

		// Continue with source addition regardless of websocket status
		if m.view == viewTaskProgress && len(m.tasks) > 3 {
			m.tasks[3].Status = taskRunning

			// Determine the dev directory for source location
			devDir, err := getDataDir()
			if err != nil {
				// Handle error - could return an error message or use fallback
				return m, nil
			}
			sourceLocation := filepath.Join(devDir, m.pupName)
			// Add the source
			return m, addSourceCmd(sourceLocation, m.authToken)
		}
	case wsLogMsg:
		// Add log message to our log buffer
		if m.view == viewTaskProgress {
			m.taskLogs = append(m.taskLogs, msg.message)
			// Keep only last 100 lines
			if len(m.taskLogs) > 100 {
				m.taskLogs = m.taskLogs[len(m.taskLogs)-100:]
			}
		}
	case actionCompleteMsg:
		// Check if this is our installation job completing
		if m.view == viewTaskProgress && msg.jobID == m.installJobID {
			if msg.success && msg.pupID != "" {
				// Update our target pup ID now that we have it
				m.targetPupID = msg.pupID
				m.taskLogs = append(m.taskLogs, fmt.Sprintf("Pup created with ID: %s", msg.pupID))
			} else if !msg.success {
				// Installation failed
				if len(m.tasks) > 4 && m.tasks[4].Status == taskRunning {
					m.tasks[4].Status = taskFailed
					m.tasks[4].Error = msg.error
					m.allTasksDone = true
					m.taskLogs = append(m.taskLogs, fmt.Sprintf("Installation failed: %s", msg.error))
					closeWebSocket()
				}
			}
		}
	case pupStateMsg:
		// Check if this is our pup and if it's ready
		// We can match by either ID or name
		if m.view == viewTaskProgress && (msg.pupID == m.targetPupID || msg.pupName == m.pupName) {
			if msg.state == "ready" {
				// Mark installation as complete
				if len(m.tasks) > 4 && m.tasks[4].Status == taskRunning {
					m.tasks[4].Status = taskSuccess
					m.allTasksDone = true
					m.taskLogs = append(m.taskLogs, "Pup installation complete!")
					closeWebSocket()
					// Refresh pup list in the background
					return m, fetchPupsCmd()
				}
			} else if msg.state == "broken" {
				// Installation failed
				if len(m.tasks) > 4 && m.tasks[4].Status == taskRunning {
					m.tasks[4].Status = taskFailed
					m.tasks[4].Error = "Installation failed - pup is in broken state"
					m.allTasksDone = true
					closeWebSocket()
				}
			}
		}
	case templateCompleteMsg:
		if m.view == viewTaskProgress && len(m.tasks) > 1 {
			if msg.err != nil {
				m.tasks[1].Status = taskFailed
				m.tasks[1].Error = msg.err.Error()
				m.allTasksDone = true
			} else {
				m.tasks[1].Status = taskSuccess
				m.taskLogs = append(m.taskLogs, "Files templated successfully")

				// Start manifest update task
				if len(m.tasks) > 2 {
					m.tasks[2].Status = taskRunning
					return m, updateManifestHashCmd(m.pupName)
				}
			}
		}

	case manifestUpdateMsg:
		if m.view == viewTaskProgress && len(m.tasks) > 2 {
			if msg.err != nil {
				m.tasks[2].Status = taskFailed
				m.tasks[2].Error = msg.err.Error()
				m.allTasksDone = true
			} else {
				m.tasks[2].Status = taskSuccess
				m.taskLogs = append(m.taskLogs, "Manifest updated successfully")

				// Connect websocket before starting next task
				if m.authToken != "" {
					return m, connectWebSocketCmd(m.authToken)
				}
			}
		}
	case pupNameValidationMsg:
		if msg.err != nil {
			m.nameInputErr = msg.err.Error()
		} else {
			// Validation passed, proceed with creation
			// Store the template name for later use
			m.selectedTemplateName = m.templates[m.selectedTpl].Name

			// Initialize tasks with the new ones
			m.tasks = []task{
				{Name: "Clone template", Status: taskPending},
				{Name: "Template files", Status: taskPending},
				{Name: "Update manifest", Status: taskPending},
				{Name: "Add Pup as source", Status: taskPending},
				{Name: "Install pup", Status: taskPending},
			}
			m.allTasksDone = false
			m.taskLogs = []string{}
			m.wsConnected = false
			// Move to task progress view
			m.view = viewTaskProgress
			// Start first task
			m.tasks[0].Status = taskRunning
			template := m.templates[m.selectedTpl]
			return m, cloneTemplateCmd(template, m.pupName)
		}
	case sourceAddedMsg:
		// Update task status
		if m.view == viewTaskProgress && len(m.tasks) > 3 {
			if msg.err != nil {
				m.tasks[3].Status = taskFailed
				m.tasks[3].Error = msg.err.Error()
				m.allTasksDone = true
				closeWebSocket()
			} else {
				m.tasks[3].Status = taskSuccess
				m.taskLogs = append(m.taskLogs, "Source added successfully")
				// Start next task
				if len(m.tasks) > 4 {
					m.tasks[4].Status = taskRunning
					// Initially track by name until we get the actual pup ID
					m.targetPupID = m.pupName
					// Trigger pup installation
					return m, installPupCmd(msg.sourceId, m.pupName, m.authToken)
				}
			}
		}
	case pupInstalledMsg:
		// This just means we triggered the installation
		if m.view == viewTaskProgress && len(m.tasks) > 4 {
			if msg.err != nil {
				m.tasks[4].Status = taskFailed
				m.tasks[4].Error = msg.err.Error()
				m.allTasksDone = true
				closeWebSocket()
			} else {
				// Installation started, wait for state change via websocket
				m.installJobID = msg.jobID
				m.taskLogs = append(m.taskLogs, "Installation started, monitoring progress...")
			}
		}
	case bootstrapCheckMsg:
		if msg.err != nil {
			m.connectionErr = msg.err.Error()
			m.socketPath = msg.socketPath
			m.view = viewConnectionError
		} else {
			// Connection successful, ensure we're not showing error view
			if m.view == viewConnectionError {
				m.view = viewLanding
			}
		}
	}
	return m, nil
}

// startTailLogCmd creates command to tail logs.
func startTailLogCmd(pupID string) tea.Cmd {
	return func() tea.Msg {
		containerDir := os.Getenv("DBX_CONTAINER_LOG_DIR")
		if containerDir == "" {
			containerDir = "/var/log/containers"
		}
		filePath := filepath.Join(containerDir, "pup-"+pupID)
		file, err := os.Open(filePath)
		if err != nil {
			return nil
		}
		// seek to end
		file.Seek(0, io.SeekEnd)
		r := bufio.NewReader(file)
		// simplified: no cancel yet

		// send cancel via special msg? We'll embed in model separately
		tea.Printf("Tailing log %s", filePath)

		return tea.Batch(func() tea.Msg {
			// deliver cancel to model
			return logLineMsg("__TAIL_START__")
		}, func() tea.Msg {
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(200 * time.Millisecond)
						continue
					}
					return nil
				}
				return logLineMsg(line)
			}
		})
	}
}

// openLogFileCmd opens the pup log file and prepares reader stored globally.
func openLogFileCmd(pupID string) tea.Cmd {
	return func() tea.Msg {
		containerDir := os.Getenv("DBX_CONTAINER_LOG_DIR")
		if containerDir == "" {
			containerDir = os.Getenv("HOME") + "/data/containerlogs"
		}
		path := filepath.Join(containerDir, "pup-"+pupID)
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		file.Seek(0, io.SeekEnd)
		globalFile = file
		globalReader = bufio.NewReader(file)
		globalActive = true
		return tailNextLineCmd() // read first line when available
	}
}

var (
	globalReader *bufio.Reader
	globalActive bool
)

func stopTail() {
	globalActive = false
	if globalFile != nil {
		globalFile.Close()
		globalFile = nil
	}
}

var globalFile *os.File

func tailNextLineCmd() tea.Cmd {
	return func() tea.Msg {
		if !globalActive || globalReader == nil {
			return nil
		}
		for {
			line, err := globalReader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(150 * time.Millisecond)
					continue
				}
				return nil
			}
			return logLineMsg(line)
		}
	}
}

func startRebuildCmd() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("sudo", "nixos-rebuild", "switch")
		pr, _ := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return logLineMsg("error: " + err.Error() + "\n")
		}

		go func() {
			if program == nil {
				return
			}
			s := bufio.NewScanner(pr)
			for s.Scan() {
				line := s.Text()
				program.Send(logLineMsg(line + "\n"))
			}
			if err := s.Err(); err != nil {
				program.Send(logLineMsg("stream error: " + err.Error() + "\n"))
			}
			cmd.Wait()
			program.Send(logLineMsg("\n\nRebuild finished.\n"))
			program.Send(rebuildFinishedMsg{})
		}()
		return nil
	}
}
