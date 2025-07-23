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
	viewTemplateSelect
	viewNameInput
	viewPasswordInput
	viewConnectionError
	viewTaskProgress
	viewSourceList
	viewSourceCreate
	viewSourceDetail
	viewSetupRequired
)

// rebuildFinishedMsg signals when rebuild completes
type rebuildFinishedMsg struct{}

const detailActionsCount = 2 // currently View Logs and Enable/Disable

// templateInfo describes a pup template from the repository
type templateInfo struct {
	Name string
	Path string
}

// templatesMsg is returned by fetchTemplatesCmd
type templatesMsg struct {
	templates []templateInfo
	err       error
}

// cloneCompleteMsg signals when template cloning is done
type cloneCompleteMsg struct {
	err error
}

// authMsg is returned when authentication completes
type authMsg struct {
	token string
	err   error
}

// sourceAddedMsg is returned when a source is added
type sourceAddedMsg struct {
	sourceId string
	err      error
}

// pupInstalledMsg is returned when pup installation is triggered
type pupInstalledMsg struct {
	jobID string
	err   error
}

// taskStatus represents the state of a task
type taskStatus int

const (
	taskPending taskStatus = iota
	taskRunning
	taskSuccess
	taskFailed
)

// task represents a single task in the progress view
type task struct {
	Name   string
	Status taskStatus
	Error  string
}

// wsMessage represents a websocket message from dogeboxd
type wsMessage struct {
	ID     string      `json:"id"`
	Error  string      `json:"error"`
	Type   string      `json:"type"`
	Update interface{} `json:"update"`
}

// wsConnectedMsg signals websocket connection status
type wsConnectedMsg struct {
	connected bool
	err       error
}

// wsLogMsg contains a log message from websocket
type wsLogMsg struct {
	message string
}

// pupStateMsg contains pup state update
type pupStateMsg struct {
	pupID   string
	pupName string // manifest.meta.name from the pup
	state   string
}

// actionCompleteMsg is sent when an action completes
type actionCompleteMsg struct {
	jobID   string
	pupID   string
	pupName string
	success bool
	error   string
}

// bootstrapCheckMsg is returned when checking connection to dogeboxd
type bootstrapCheckMsg struct {
	socketPath            string
	err                   error
	configurationComplete bool
}

// sourceInfo holds information about a single source
type sourceInfo struct {
	ID          string
	Name        string
	Description string
	Location    string
	Type        string
}

// sourcesMsg is returned by fetchSourcesCmd
type sourcesMsg struct {
	sources []sourceInfo
	err     error
}

// sourceCreatedMsg is returned when a source is created
type sourceCreatedMsg struct {
	err error
}

// sourceDeletedMsg is returned when a source is deleted
type sourceDeletedMsg struct {
	err error
}

// templateCompleteMsg signals when template file replacement is done
type templateCompleteMsg struct {
	err error
}

// manifestUpdateMsg signals when manifest hash update is done
type manifestUpdateMsg struct {
	err error
}

// pupNameValidationMsg is returned when pup name validation completes
type pupNameValidationMsg struct {
	err error
}
