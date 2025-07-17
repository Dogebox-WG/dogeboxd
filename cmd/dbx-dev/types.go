package dbxdev

import "time"

// tickMsg is emitted every second to refresh metrics.
type tickMsg time.Time

// pupInfo describes a single pup entry in the list.
type pupInfo struct {
	ID           string
	Name         string
	State        string
	Enabled      bool
	Error        string
	DevEnabled   bool
	DevAvailable bool
}

// pupsMsg is returned by fetchPupsCmd.
type pupsMsg struct {
	list []pupInfo
	err  error
}

// logLineMsg carries a single log line.
type logLineMsg string

// viewState represents the current screen being displayed.
type viewState int

const (
	viewLanding viewState = iota
	viewPupDetail
	viewCreatePup
	viewLogs
	viewRebuild
)

// rebuildFinishedMsg signals when rebuild completes
type rebuildFinishedMsg struct{}

const detailActionsCount = 2 // currently View Logs and Enable/Disable
