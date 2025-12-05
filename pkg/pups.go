package dogeboxd

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Pup states
const (
	STATE_INSTALLING   string = "installing"
	STATE_UPGRADING    string = "upgrading"
	STATE_READY        string = "ready"
	STATE_UNREADY      string = "unready"
	STATE_UNINSTALLING string = "uninstalling"
	STATE_UNINSTALLED  string = "uninstalled"
	STATE_PURGING      string = "purging"
	STATE_BROKEN       string = "broken"
	STATE_STOPPED      string = "stopped"
	STATE_STARTING     string = "starting"
	STATE_RUNNING      string = "running"
	STATE_STOPPING     string = "stopping"
)

// Pup broken reasons
const (
	BROKEN_REASON_STATE_UPDATE_FAILED          string = "state_update_failed"
	BROKEN_REASON_DOWNLOAD_FAILED              string = "download_failed"
	BROKEN_REASON_NIX_FILE_MISSING             string = "nix_file_missing"
	BROKEN_REASON_NIX_HASH_MISMATCH            string = "nix_hash_mismatch"
	BROKEN_REASON_STORAGE_CREATION_FAILED      string = "storage_creation_failed"
	BROKEN_REASON_DELEGATE_KEY_CREATION_FAILED string = "delegate_key_creation_failed"
	BROKEN_REASON_DELEGATE_KEY_WRITE_FAILED    string = "delegate_key_write_failed"
	BROKEN_REASON_ENABLE_FAILED                string = "enable_failed"
	BROKEN_REASON_NIX_APPLY_FAILED             string = "nix_apply_failed"
)

const (
	PUP_CHANGED_INSTALLATION int = iota
	PUP_ADOPTED                  = iota
	PUP_PURGED                   = iota
)

// PupManager Errors
var (
	ErrPupNotFound      = errors.New("pup not found")
	ErrPupAlreadyExists = errors.New("pup already exists")
)

/* Pup state vs pup stats
 * ┌─────────────────────────────┬───────────────────────────────┐
 * │PupState.Installation        │ PupStats.Status               │
 * ├─────────────────────────────┼───────────────────────────────┤
 * │                             │                               │
 * │installing                   │    stopped                    │
 * │ready                       ─┼─>  starting                   │
 * │unready                      │    running                    │
 * │uninstalling                 │    stopping                   │
 * │uninstalled                  │                               │
 * │broken                       │                               │
 * └─────────────────────────────┴───────────────────────────────┘
 *
 * Valid actions: install, stop, start, restart, uninstall
 */

// PupState is persisted to disk
type PupState struct {
	ID           string                      `json:"id"`
	LogoBase64   string                      `json:"logoBase64"`
	Source       ManifestSourceConfiguration `json:"source"`
	Manifest     PupManifest                 `json:"manifest"`
	Config       map[string]string           `json:"config"`
	ConfigSaved  bool                        `json:"configSaved"`  // Has config been saved at least once?
	Providers    map[string]string           `json:"providers"`    // providers of interface dependencies
	Hooks        []PupHook                   `json:"hooks"`        // webhooks
	Installation string                      `json:"installation"` // see table above and constants
	BrokenReason string                      `json:"brokenReason"` // reason for being in a broken state
	Enabled      bool                        `json:"enabled"`      // Is this pup supposed to be running?
	NeedsConf    bool                        `json:"needsConf"`    // Has all required config been provided?
	NeedsDeps    bool                        `json:"needsDeps"`    // Have all dependencies been met?
	IP           string                      `json:"ip"`           // Internal IP for this pup
	Version      string                      `json:"version"`
	WebUIs       []PupWebUI                  `json:"webUIs"`

	IsDevModeEnabled bool     `json:"isDevModeEnabled"`
	DevModeServices  []string `json:"devModeServices"`
}

// Represents a Web UI exposed port from the manifest
type PupWebUI struct {
	Name     string `json:"name"`
	Internal int    `json:"-"`
	Port     int    `json:"port"`
}

type PupHook struct {
	Port int    `json:"port"`
	Path string `json:"path"`
	ID   string `json:"id"`
}

type PupMetrics[T any] struct {
	Name   string     `json:"name"`
	Label  string     `json:"label"`
	Type   string     `json:"type"`
	Values *Buffer[T] `json:"values"`
}

// PupStats is not persisted to disk, and holds the running
// stats for the pup process, ie: disk, CPU, etc.
type PupStats struct {
	ID            string            `json:"id"`
	Status        string            `json:"status"`
	SystemMetrics []PupMetrics[any] `json:"systemMetrics"`
	Metrics       []PupMetrics[any] `json:"metrics"`
	Issues        PupIssues         `json:"issues"`
}

type PupLogos struct {
	MainLogoBase64 string `json:"mainLogoBase64"`
}

type PupAsset struct {
	Logos PupLogos `json:"logos"`
}

type PupIssues struct {
	DepsNotRunning   []string `json:"depsNotRunning"`
	HealthWarnings   []string `json:"healthWarnings"`
	UpgradeAvaialble bool     `json:"upgradeAvailable"`
}

type PupDependencyReport struct {
	Interface             string                        `json:"interface"`
	Version               string                        `json:"version"`
	Optional              bool                          `json:"optional"`
	CurrentProvider       string                        `json:"currentProvider"`
	InstalledProviders    []string                      `json:"installedProviders"`
	InstallableProviders  []PupManifestDependencySource `json:"InstallableProviders"`
	DefaultSourceProvider PupManifestDependencySource   `json:"DefaultProvider"`
}

type PupHealthStateReport struct {
	Issues    PupIssues
	NeedsConf bool
	NeedsDeps bool
}

// Represents a change to pup state
type Pupdate struct {
	ID    string
	Event int // see consts above ^
	State PupState
}

type Buffer[T any] struct {
	Values []T
	Tail   int
}

func NewBuffer[T any](size int) *Buffer[T] {
	return &Buffer[T]{
		Values: make([]T, size),
		Tail:   0,
	}
}

func (b *Buffer[T]) Add(value T) {
	b.Values[b.Tail] = value
	b.Tail = (b.Tail + 1) % len(b.Values)
}

func (b *Buffer[T]) GetValues() []T {
	firstN := make([]T, len(b.Values))
	if b.Tail > 0 {
		copy(firstN, b.Values[b.Tail:])
		copy(firstN[len(b.Values)-b.Tail:], b.Values[:b.Tail])
	} else {
		copy(firstN, b.Values)
	}
	return firstN
}

func (b *Buffer[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.GetValues())
}

type AdoptPupOptions struct {
	/// Install pup with development features enabled
	DevMode bool
}

/* The PupManager is responsible for all aspects of the pup lifecycle
 * see pkg/pup/manager.go
 */
type PupManager interface {
	// Run starts the PupManager as a service.
	Run(started, stopped chan bool, stop chan context.Context) error

	// GetUpdateChannel returns a channel for receiving pup updates.
	GetUpdateChannel() chan Pupdate

	// GetStatsChannel returns a channel for receiving pup stats.
	GetStatsChannel() chan []PupStats

	// GetStateMap returns a map of all pup states.
	GetStateMap() map[string]PupState

	// GetStatsMap returns a map of all pup stats.
	GetStatsMap() map[string]PupStats

	// GetAssetsMap returns a map of pup assets like logos.
	GetAssetsMap() map[string]PupAsset

	// AdoptPup adds a new pup from a manifest. It returns the PupID and an error if any.
	AdoptPup(m PupManifest, source ManifestSource, options AdoptPupOptions) (string, error)

	// UpdatePup updates the state of a pup with provided update functions.
	UpdatePup(id string, updates ...func(*PupState, *[]Pupdate)) (PupState, error)

	// PurgePup removes a pup and its state from the manager.
	PurgePup(pupId string) error

	// GetPup retrieves the state and stats for a specific pup by ID.
	GetPup(id string) (PupState, PupStats, error)

	// FindPupByIP retrieves a pup by its assigned IP address.
	FindPupByIP(ip string) (PupState, PupStats, error)

	// GetAllFromSource retrieves all pups from a specific source.
	GetAllFromSource(source ManifestSourceConfiguration) []*PupState

	// GetPupFromSource retrieves a specific pup by name from a source.
	GetPupFromSource(name string, source ManifestSourceConfiguration) *PupState

	// GetMetrics retrieves the metrics for a specific pup.
	GetMetrics(pupId string) map[string]interface{}

	// UpdateMetrics updates the metrics for a pup based on provided data.
	UpdateMetrics(u UpdateMetrics)

	// CanPupStart checks if a pup can start based on its current state and dependencies.
	CanPupStart(pupId string) (bool, error)

	// GetPupHealthState returns the health state report for a pup.
	GetPupHealthState(pup *PupState) PupHealthStateReport

	// CalculateDeps calculates the dependencies for a pup.
	CalculateDeps(pupID string) ([]PupDependencyReport, error)

	// SetSourceManager sets the SourceManager for the PupManager.
	SetSourceManager(sourceManager SourceManager)

	// FastPollPup initiates a rapid polling of a specific pup for debugging or immediate updates.
	FastPollPup(pupId string)

	GetPupSpecificEnvironmentVariablesForContainer(pupID string) map[string]string
}

func SetPupInstallation(state string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Installation = state
		*pu = append(*pu, Pupdate{
			ID:    p.ID,
			Event: PUP_CHANGED_INSTALLATION,
			State: *p,
		})
	}
}

func SetPupBrokenReason(reason string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.BrokenReason = reason
	}
}

func SetPupConfig(newFields map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Config == nil {
			p.Config = map[string]string{}
		}

		fieldIndex := ManifestConfigFieldIndex(p.Manifest.Config)

		for k, v := range newFields {
			if _, ok := fieldIndex[k]; !ok {
				continue
			}
			p.Config[k] = v
		}

		// Mark config as saved (satisfies showOnInstall requirement)
		p.ConfigSaved = true

		p.NeedsConf = ManifestConfigNeedsValues(p.Manifest.Config, p.Config)
	}
}

func SetPupProviders(newProviders map[string]string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Providers == nil {
			p.Providers = make(map[string]string)
		}

		for k, v := range newProviders {
			p.Providers[k] = v
		}
	}
}

func SetPupVersion(version string) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Version = version
	}
}

func SetPupManifest(manifest PupManifest) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Manifest = manifest
		// Recalculate if config needs values based on new manifest
		p.NeedsConf = ManifestConfigNeedsValues(p.Manifest.Config, p.Config)
	}
}

func PupEnabled(b bool) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		p.Enabled = b
	}
}

func SetPupHooks(newHooks []PupHook) func(*PupState, *[]Pupdate) {
	return func(p *PupState, pu *[]Pupdate) {
		if p.Hooks == nil {
			p.Hooks = []PupHook{}
		}

		for _, hook := range newHooks {
			id, err := newID(16)
			if err != nil {
				fmt.Println("couldn't generate random ID for hook")
				continue
			}
			hook.ID = id
			p.Hooks = append(p.Hooks, hook)
		}
	}
}

// Generate a somewhat random ID string
func newID(l int) (string, error) {
	var ID string
	b := make([]byte, l)
	_, err := rand.Read(b)
	if err != nil {
		return ID, err
	}
	return fmt.Sprintf("%x", b), nil
}

// ===========================================
// =============== Pup Updates ===============
// ===========================================

/* Pup update types
 */
// PupUpdateInfo tracks available updates for a pup
type PupUpdateInfo struct {
	PupID             string       `json:"pupId"`
	CurrentVersion    string       `json:"currentVersion"`
	LatestVersion     string       `json:"latestVersion"`
	AvailableVersions []PupVersion `json:"availableVersions"`
	UpdateAvailable   bool         `json:"updateAvailable"`
	LastChecked       time.Time    `json:"lastChecked"`
}

// PupVersion represents a version available for update
type PupVersion struct {
	Version          string                `json:"version"`
	ReleaseNotes     string                `json:"releaseNotes"` // From GitHub release body
	ReleaseDate      time.Time             `json:"releaseDate"`
	ReleaseURL       string                `json:"releaseUrl"` // GitHub release URL
	BreakingChanges  []string              `json:"breakingChanges"`
	InterfaceChanges []PupInterfaceVersion `json:"interfaceChanges"`
}

// PupInterfaceVersion tracks changes to provided interfaces
type PupInterfaceVersion struct {
	InterfaceName string   `json:"interfaceName"`
	OldVersion    string   `json:"oldVersion"`
	NewVersion    string   `json:"newVersion"`
	ChangeType    string   `json:"changeType"`   // "major", "minor", "patch"
	AffectedPups  []string `json:"affectedPups"` // PupIDs that depend on this interface
}

// SkippedPupUpdate tracks skipped updates (persisted on backend)
type SkippedPupUpdate struct {
	PupID               string    `json:"pupId"`
	SkippedAtVersion    string    `json:"skippedAtVersion"`
	LatestVersionAtSkip string    `json:"latestVersionAtSkip"`
	SkippedAt           time.Time `json:"skippedAt"`
}

// PupUpdatePreviousVersion tracks update history for rollback
type PupUpdatePreviousVersion struct {
	PupID           string              `json:"pupId"`
	PreviousVersion *PupVersionSnapshot `json:"previousVersion"` // Only keep last version
}

// PupVersionSnapshot stores data needed for rollback
// Note: User data in storage directory is NOT snapshotted - only state/config
type PupVersionSnapshot struct {
	Version        string            `json:"version"`
	Manifest       PupManifest       `json:"manifest"`
	Config         map[string]string `json:"config"`
	Providers      map[string]string `json:"providers"`
	Enabled        bool              `json:"enabled"`
	SnapshotDate   time.Time         `json:"snapshotDate"`
	SourceID       string            `json:"sourceId"`
	SourceLocation string            `json:"sourceLocation"` // For re-downloading
}

// GitHubRelease represents a release from GitHub API
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
}

/* Pup update actions
 */
type CheckPupUpdates struct {
	PupID string // Empty string = check all pups
}

// PupUpdatesCheckedEvent is emitted when a pup update check completes
type PupUpdatesCheckedEvent struct {
	PupsChecked      int  `json:"pupsChecked"`
	UpdatesAvailable int  `json:"updatesAvailable"`
	IsPeriodicCheck  bool `json:"isPeriodicCheck"`
}

/* The PupUpdateChecker is used to check for pup updates
 */
type PupUpdateChecker interface {
	// CheckForUpdates checks for updates for a specific pup
	CheckForUpdates(pupID string) (PupUpdateInfo, error)

	// CheckAllPupUpdates checks for updates for all installed pups
	CheckAllPupUpdates() map[string]PupUpdateInfo

	// GetCachedUpdateInfo retrieves update information from the cache
	GetCachedUpdateInfo(pupID string) (PupUpdateInfo, bool)

	// GetAllCachedUpdates retrieves all update information from the cache
	GetAllCachedUpdates() map[string]PupUpdateInfo

	// ClearCacheEntry removes a specific pup from the update cache
	ClearCacheEntry(pupID string)

	// StartPeriodicCheck starts a background goroutine that checks for updates periodically
	StartPeriodicCheck(stop chan bool)

	// GetEventChannel returns the channel for update check completion events
	GetEventChannel() <-chan PupUpdatesCheckedEvent

	// DetectInterfaceChanges compares interfaces between two manifests and returns changes
	DetectInterfaceChanges(oldManifest, newManifest PupManifest) []PupInterfaceVersion
}

/* The SkippedUpdatesManager manages user preferences for skipped pup updates
 */
type SkippedUpdatesManager interface {
	// SkipUpdate marks updates as skipped for a specific pup
	SkipUpdate(pupID, currentVersion, latestVersion string) error

	// ClearSkipped removes the skip status for a specific pup
	ClearSkipped(pupID string) error

	// IsSkipped checks if updates are currently skipped for a pup
	IsSkipped(pupID, latestVersion string) bool

	// GetSkipInfo retrieves skip info for a specific pup
	GetSkipInfo(pupID string) (SkippedPupUpdate, bool)

	// GetAllSkipped returns all skipped updates
	GetAllSkipped() map[string]SkippedPupUpdate
}
