package dbxdev

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
}

// Init satisfies tea.Model and starts the periodic ticker & initial fetch.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }),
		fetchPupsCmd(),
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

// Update handles all incoming BubbleTea messages and updates state.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.searching {
				m.searchQuery += "q"
				break
			}
			return m, tea.Quit
		case "r", "R":
			if m.view == viewLanding && !m.searching {
				m.logs = nil
				m.view = viewRebuild
				m.logs = []string{}
				m.rebuildComplete = false
				return m, startRebuildCmd()
			}
			if m.searching {
				m.searchQuery += "r"
				break
			}
			m.refreshMetrics()
			return m, fetchPupsCmd()
		case "s":
			if m.view != viewLanding {
				break
			}
			if !m.searching {
				m.searching = true
				m.searchQuery = ""
				break
			}
			if m.searching {
				m.searchQuery += "s"
			}
		case "up":
			if m.view == viewLanding && !m.searching && len(m.pups) > 0 {
				m.selected = (m.selected - 1 + len(m.pups)) % len(m.pups)
			} else if m.view == viewPupDetail {
				m.selDetail = (m.selDetail - 1 + detailActionsCount) % detailActionsCount
			}
		case "down":
			if m.view == viewLanding && !m.searching && len(m.pups) > 0 {
				m.selected = (m.selected + 1) % len(m.pups)
			} else if m.view == viewPupDetail {
				m.selDetail = (m.selDetail + 1) % detailActionsCount
			}
		case "k":
			if m.searching {
				m.searchQuery += "k"
			} else if m.view == viewLanding && len(m.pups) > 0 {
				m.selected = (m.selected - 1 + len(m.pups)) % len(m.pups)
			}
		case "j":
			if m.searching {
				m.searchQuery += "j"
			} else if m.view == viewLanding && len(m.pups) > 0 {
				m.selected = (m.selected + 1) % len(m.pups)
			}
		case "enter":
			if m.view == viewLanding && len(m.pups) > 0 {
				m.view = viewPupDetail
				m.detail = m.pups[m.selected]
				m.selDetail = 0
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
			}
		case "c":
			if m.searching || m.view != viewLanding {
				break
			}
			m.view = viewCreatePup
		case "esc":
			if m.view == viewLogs || m.view == viewRebuild {
				stopTail()
				if m.view == viewLogs {
					m.view = viewPupDetail
				} else {
					m.view = viewLanding
				}
			} else if m.view == viewPupDetail || m.view == viewCreatePup {
				m.view = viewLanding
			} else if m.searching {
				m.searching = false
				m.searchQuery = ""
			}
		default:
			if m.searching {
				switch msg.Type {
				case tea.KeyBackspace, tea.KeyDelete:
					if len(m.searchQuery) > 0 {
						m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					}
				case tea.KeyRunes:
					m.searchQuery += msg.String()
				}
			}
		}

	case tickMsg:
		// Refresh system metrics
		if cpus, _ := cpu.PercentWithContext(context.Background(), 0, false); len(cpus) > 0 {
			m.cpuPercent = cpus[0]
		}
		if v, _ := mem.VirtualMemory(); v != nil {
			m.memUsed = v.Used / (1024 * 1024)
			m.memTotal = v.Total / (1024 * 1024)
		}
		// Schedule next tick
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })

	case pupsMsg:
		if msg.err == nil {
			m.pups = msg.list
		}
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
