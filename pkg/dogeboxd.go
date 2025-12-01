/*
Dogebox internal architecture:

 Actions are instructions from the user to do something, and come externally
 via the REST API or Websocket etc.  These are submitted to Dogeboxd.AddAction
 and become Jobs in the job queue, returning a Job ID

 Jobs are either processed directly, or if related to the system in some way,
 handed to the SystemUpdater.

 Completed Jobs are submitted to the Changes channel for reporting back to
 the user, along with their Job ID.

                                       ┌──────────────┐
                                       │  Dogeboxd{}  │
                                       │              │
                                       │  ┌────────►  │
                                       │  │Dogebox │  │
 REST API  ─────┐                      │  │Run Loop│  │
                │                      │  ◄──────┬─┘  │
                │                      │     ▲   │    │
                │                      │     │   ▼    │
                │              ======= │  ┌──┴─────►  │ =======   Job ID
                │ Actions      Jobs    │  │ System │  │ Changes
 WebSocket ─────┼───────────►  Channel │  │ Updater│  │ Channel ───► WebSocket
                │ Job ID       ======= │  ◄────────┘  │ =======
                │ ◄────                │              │
                │                      │   ▲      │   │
                │                      │   │      │   │
                │                      └───┼──────┼───┘
 System         │                          │      │
 Events   ──────┘                          │      ▼
                                           Nix CLI
                                           SystemD

*/

package dogeboxd

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

type syncQueue struct {
	jobQueue      []Job
	jobQLock      sync.Mutex
	jobInProgress sync.Mutex
	jobTimer      time.Time
}

type Dogeboxd struct {
	Pups           PupManager
	SystemUpdater  SystemUpdater
	SystemMonitor  SystemMonitor
	JournalReader  JournalReader
	NetworkManager NetworkManager
	sm             StateManager
	sources        SourceManager
	nix            NixManager
	logtailer      LogTailer
	queue          *syncQueue
	jobs           chan Job
	Changes        chan Change
	JobManager     *JobManager
	config         *ServerConfig
}

func NewDogeboxd(
	stateManager StateManager,
	pups PupManager,
	updater SystemUpdater,
	monitor SystemMonitor,
	journal JournalReader,
	networkManager NetworkManager,
	sourceManager SourceManager,
	nixManager NixManager,
	logtailer LogTailer,
	jobManager *JobManager,
	config *ServerConfig,
) Dogeboxd {
	q := syncQueue{
		jobQueue:      []Job{},
		jobQLock:      sync.Mutex{},
		jobInProgress: sync.Mutex{},
	}
	s := Dogeboxd{
		Pups:           pups,
		SystemUpdater:  updater,
		SystemMonitor:  monitor,
		JournalReader:  journal,
		NetworkManager: networkManager,
		sm:             stateManager,
		sources:        sourceManager,
		nix:            nixManager,
		logtailer:      logtailer,
		queue:          &q,
		jobs:           make(chan Job, 256),
		Changes:        make(chan Change, 256),
		JobManager:     jobManager,
		config:         config,
	}

	return s
	// TODO start monitoring all installed services
	// SUB TO PUP MANAGER monitor.GetMonChannel() <- []string{"dbus.service"}
}

// SetJobManager sets the JobManager reference after Dogeboxd is created
func (t *Dogeboxd) SetJobManager(jm *JobManager) {
	t.JobManager = jm
}

// Main Dogeboxd goroutine, handles routing messages in
// and out of the system via job and change channels,
// handles messages from subsystems ie: SystemUpdater,
// SystemMonitor etc.
func (t Dogeboxd) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
		mainloop:
			for {
			dance:
				select {

				// Handle shutdown
				case <-stop:
					break mainloop

				// Hand incoming jobs to the Job Dispatcher
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					j.Start = time.Now() // start the job timer

					// Create job record for tracking (skip routine operations like metrics)
					if t.JobManager != nil && t.shouldTrackJob(j) {
						record, err := t.JobManager.CreateJobRecord(j)
						if err == nil {
							t.sendChange(Change{ID: "internal", Type: "job:created", Update: record})
						}
					}

					t.jobDispatcher(j)

				// Handle pupdates from PupManager
				case p, ok := <-t.Pups.GetUpdateChannel():
					if !ok {
						break dance
					}
					t.sendChange(Change{"internal", "", "pup", p.State})

				// Handle stats from PupManager
				case stats, ok := <-t.Pups.GetStatsChannel():
					if !ok {
						break dance
					}
					t.sendChange(Change{"internal", "", "stats", stats})

				// Handle completed jobs from SystemUpdater
				case j, ok := <-t.SystemUpdater.GetUpdateChannel():
					if !ok {
						break dance
					}
					// job is finished, unlock the queue for the next job
					t.queue.jobInProgress.Unlock()
					j.Logger.Step("queue").Progress(100).Log(fmt.Sprintf("finished in %.2fs, queued %.2fs", time.Since(t.queue.jobTimer).Seconds(), time.Since(j.Start).Seconds()))

					// if this job was successful, AND it was a
					// job that results in the stop/start of a pup,
					// tell the PupManager to poll for state changes
					switch j.A.(type) {
					case InstallPup:
						t.Pups.FastPollPup(j.State.ID)
					case EnablePup:
						t.Pups.FastPollPup(j.State.ID)
					case DisablePup:
						t.Pups.FastPollPup(j.State.ID)
					case UpdatePupProviders:
						t.Pups.FastPollPup(j.State.ID)
					case UninstallPup:
						t.Pups.FastPollPup(j.State.ID)
					case PurgePup:
						t.Pups.FastPollPup(j.State.ID)
					}

					// TODO: explain why we I this
					if j.Err == "" && j.State != nil {
						state, _, err := t.Pups.GetPup(j.State.ID)
						if err == nil {
							j.Success = state
						}
					}

					// Update job record as completed/failed
					if t.JobManager != nil {
						err := t.JobManager.CompleteJob(j.ID, j.Err)
						if err == nil {
							jobRecord, getErr := t.JobManager.GetJob(j.ID)
							if getErr == nil {
								t.sendChange(Change{ID: "internal", Type: "job_completed", Update: jobRecord})
							}
						}
					}

					t.sendFinishedJob("action", j)

				case <-time.After(time.Millisecond * 100): // Periodic check
					t.pumpQueue()
				}
			}
		}()
		// flag to Conductor we are running
		started <- true
		// Wait on a stop signal
		<-stop
		// do shutdown things and flag we are stopped
		stopped <- true
	}()
	return nil
}

// pumpQueue runs every 100ms and attempts to push another job to the SystemUpdater
// which has been queued with enqueue. Only one job can be running at a time.
// jobInProgress is unlocked int he main loop in Run when a job is finished.
func (t *Dogeboxd) pumpQueue() {
	if t.queue.jobInProgress.TryLock() {
		t.queue.jobQLock.Lock()
		if len(t.queue.jobQueue) > 0 {

			job := t.queue.jobQueue[0]
			t.queue.jobQueue = t.queue.jobQueue[1:]
			t.queue.jobQLock.Unlock()

			job.Logger.Step("queue").Log(fmt.Sprintf("Queued, position %d\n", len(t.queue.jobQueue)))
			t.SystemUpdater.AddJob(job)
			t.queue.jobTimer = time.Now()
		} else {
			t.queue.jobQLock.Unlock()
			t.queue.jobInProgress.Unlock()
		}
	}
}

// Add the new job to the queue
func (t *Dogeboxd) enqueue(j Job) {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()
	t.queue.jobQueue = append(t.queue.jobQueue, j)
}

// Add an Action to the Action queue, returns a unique ID
// which can be used to match the outcome in the Event queue
func (t Dogeboxd) AddAction(a Action) string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println(">> AddAction: Entropic Failure, add more Overminds.")
	}
	id := fmt.Sprintf("%x", b)
	j := Job{A: a, ID: id}
	j.Logger = NewActionLogger(j, "", t)
	t.jobs <- j
	return id
}

/* jobDispatcher handles any incomming Jobs
 * based on their Action type, some to internal
 * helpers and others sent to the system updater
 * for handling.
 */
func (t Dogeboxd) jobDispatcher(j Job) {
	switch a := j.A.(type) {

	// System actions
	case InstallPup:
		t.createPupFromManifest(j, a.PupName, a.PupVersion, a.SourceId, a.Options)
	case InstallPups:
		for i, pup := range a {
			pupJobID := fmt.Sprintf("%s-%d", j.ID, i+1)

			// Create a separate job for each pup in the batch
			pupJob := Job{
				ID:      pupJobID,
				A:       pup,
				Err:     j.Err,
				Success: j.Success,
				Start:   j.Start,
				Logger:  NewActionLogger(Job{ID: pupJobID}, "", t),
				State:   j.State,
			}
			t.createPupFromManifest(pupJob, pup.PupName, pup.PupVersion, pup.SourceId, pup.Options)
		}
	case UninstallPup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case PurgePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case EnablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case DisablePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)

	// Dogebox actions
	case UpdatePupConfig:
		t.updatePupConfig(j, a)

	case UpdatePupProviders:
		t.updatePupProviders(j, a)

	case UpdatePupHooks:
		t.updatePupHooks(j, a)

	case ImportBlockchainData:
		t.enqueue(j)

	// Host Actions
	case UpdatePendingSystemNetwork:
		t.enqueue(j)

	case EnableSSH:
		t.enqueue(j)

	case DisableSSH:
		t.enqueue(j)

	case AddSSHKey:
		t.enqueue(j)

	case RemoveSSHKey:
		t.enqueue(j)

	case AddBinaryCache:
		t.enqueue(j)

	case RemoveBinaryCache:
		t.enqueue(j)

	case SystemUpdate:
		t.enqueue(j)

	// Pup router actions
	case UpdateMetrics:
		t.Pups.UpdateMetrics(a)

	default:
		fmt.Printf("Unknown action type: %v\n", a)
	}
}

/* This is where we create a 'PupState' from a ManifestID
* and set it to be installed by the SystemUpdater. After
* this point the Pup has entered a managed state and will
* only be installable again after this one has been purged.
*
* Future: support multiple pup instances per manifest
 */
func (t *Dogeboxd) createPupFromManifest(j Job, pupName, pupVersion, sourceId string, pupOptions AdoptPupOptions) {
	// Fetch the correct manifest from the source manager
	manifest, source, err := t.sources.GetSourceManifest(sourceId, pupName, pupVersion)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup, no manifest: %s", err)
		t.sendFinishedJob("action", j)
		return
	}

	// create a new pup for the manifest
	pupID, err := t.Pups.AdoptPup(manifest, source, pupOptions)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup: %s", err)
		t.sendFinishedJob("action", j)
		return
	}

	// send the job off to the SystemUpdater to install
	t.sendSystemJobWithPupDetails(j, pupID)
}

// Handle batch installation of multiple pups
func (t *Dogeboxd) installPups(j Job, pups InstallPups) {
	for _, pup := range pups {
		t.createPupFromManifest(j, pup.PupName, pup.PupVersion, pup.SourceId, pup.Options)
	}
}

// Handle an UpdatePupConfig action
func (t *Dogeboxd) updatePupConfig(j Job, u UpdatePupConfig) {
	log := j.Logger.Step("config")

	// Get state before update to check if we need to auto-enable
	oldState, _, _ := t.Pups.GetPup(u.PupID)
	wasNeedingConfig := oldState.NeedsConf

	newState, err := t.Pups.UpdatePup(u.PupID, SetPupConfig(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("couldn't update config for %s: %v", u.PupID, err)
		t.sendFinishedJob("action", j)
		return
	}

	// Write config to secure storage (inside pup container, not exposed on host)
	if err := WritePupConfigToStorage(t.config.DataDir, u.PupID, newState.Config, log); err != nil {
		j.Err = fmt.Sprintf("failed to write config to storage: %v", err)
		t.sendFinishedJob("action", j)
		return
	}

	// Check if config requirements are now satisfied
	healthReport := t.Pups.GetPupHealthState(&newState)
	configNowSatisfied := wasNeedingConfig && !healthReport.NeedsConf && !healthReport.NeedsDeps

	// If config is now satisfied and pup isn't enabled, enable it
	if configNowSatisfied && !newState.Enabled {
		log.Logf("Config requirements satisfied, enabling pup")
		newState, err = t.Pups.UpdatePup(u.PupID, PupEnabled(true))
		if err != nil {
			j.Err = fmt.Sprintf("failed to enable pup after config: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
	}

	// Rebuild nix configuration and restart the pup
	dbxState := t.sm.Get().Dogebox
	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		j.Err = fmt.Sprintf("failed to apply configuration: %v", err)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success = newState
	t.sendFinishedJob("action", j)
}

// Handle an UpdatePupProviders action
func (t *Dogeboxd) updatePupProviders(j Job, u UpdatePupProviders) {
	log := j.Logger.Step("update providers")
	_, err := t.Pups.UpdatePup(u.PupID, SetPupProviders(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	pupState, _, err := t.Pups.GetPup(u.PupID)
	j.Success = pupState
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	canPupStart, err := t.Pups.CanPupStart(u.PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	// If the pup may now start, update all of our nix files and rebuild.
	if canPupStart {
		dbxState := t.sm.Get().Dogebox

		nixPatch := t.nix.NewPatch(log)
		t.nix.UpdateSystemContainerConfiguration(nixPatch)
		t.nix.WritePupFile(nixPatch, pupState, dbxState)

		if err := nixPatch.Apply(); err != nil {
			j.Err = fmt.Sprintf("Failed to apply nix patch: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
	}

	t.sendFinishedJob("action", j)
}

// Handle an UpdatePupHooks action
func (t *Dogeboxd) updatePupHooks(j Job, u UpdatePupHooks) {
	_, err := t.Pups.UpdatePup(u.PupID, SetPupHooks(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success, _, err = t.Pups.GetPup(u.PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}
	t.sendFinishedJob("action", j)
}

// send changes without blocking if the channel is full
func (t Dogeboxd) sendChange(c Change) {
	timer := time.After(200 * time.Millisecond)
	select {
	case t.Changes <- c:
	case <-timer:
		fmt.Println("Can't sent change, no receiver", c)
	}
}

// helper to report a completed job back to the client
func (t Dogeboxd) sendFinishedJob(changeType string, j Job) {
	if j.Err != "" {
		j.Logger.Step("queue").Err(j.Err)
	}

	// Update job record as completed/failed for immediate jobs (those that don't go through SystemUpdater)
	// This ensures jobs like UpdatePupProviders get properly marked as completed
	// Only call CompleteJob if the job is still active (not already completed by SystemUpdater path)
	if t.JobManager != nil && t.shouldTrackJob(j) && t.JobManager.IsJobActive(j.ID) {
		err := t.JobManager.CompleteJob(j.ID, j.Err)
		if err == nil {
			jobRecord, getErr := t.JobManager.GetJob(j.ID)
			if getErr == nil {
				t.sendChange(Change{ID: "internal", Type: "job:completed", Update: jobRecord})
			}
		}
	}

	t.sendChange(Change{ID: j.ID, Error: j.Err, Type: changeType, Update: j.Success})
}

// shouldTrackJob determines if a job should create a visible job record
// Excludes routine background operations that users don't need to see
func (t Dogeboxd) shouldTrackJob(j Job) bool {
	switch j.A.(type) {
	case UpdateMetrics:
		return false // Metrics updates happen every 10s, don't track
	case UpdatePupConfig:
		return false // Config updates are instantaneous, don't need tracking
	case UpdatePupHooks:
		return false // Hook updates are instantaneous
	default:
		return true // Track everything else
	}
}

// updates the client on the progress of any inflight actions
func (t Dogeboxd) sendProgress(p ActionProgress) {
	// Update job record with progress
	if t.JobManager != nil {
		err := t.JobManager.UpdateJobProgress(p)
		if err == nil {
			jobRecord, getErr := t.JobManager.GetJob(p.ActionID)
			if getErr == nil {
				t.sendChange(Change{ID: "internal", Type: "job:updated", Update: jobRecord})
			}
		}
	}

	t.sendChange(Change{ID: p.ActionID, Type: "progress", Update: p})
}

// helper to attach PupState to a job and send it to the SystemUpdater
func (t Dogeboxd) sendSystemJobWithPupDetails(j Job, PupID string) {
	p, _, err := t.Pups.GetPup(PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	j.State = &p
	j.Logger.PupID = PupID

	// Send job to the system updater for handling
	t.enqueue(j)
}

// WritePupConfigToStorage writes the pup's user configuration to a secure file
// in the pup's storage directory. This file is loaded by systemd via EnvironmentFile
// directive, keeping sensitive config values (like passwords) out of the nix files.
func WritePupConfigToStorage(dataDir string, pupID string, config map[string]string, log SubLogger) error {
	// Convert config map to JSON
	configJSON, err := configToJSON(config)
	if err != nil {
		if log != nil {
			log.Errf("Failed to serialize config to JSON: %v", err)
		}
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "write-config",
		"--data-dir", dataDir,
		"--pupId", pupID,
		"--config", configJSON,
	)

	if log != nil {
		log.Logf("Writing pup config to storage")
		log.LogCmd(cmd)
	}

	if err := cmd.Run(); err != nil {
		if log != nil {
			log.Errf("Failed to write pup config: %v", err)
		}
		return fmt.Errorf("failed to write pup config: %w", err)
	}

	return nil
}

// configToJSON converts a config map to JSON string for passing to _dbxroot
func configToJSON(config map[string]string) (string, error) {
	if config == nil {
		return "{}", nil
	}

	// Use encoding/json to properly escape values
	bytes, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

var allowedJournalServices = map[string]string{
	"dbx": "dogeboxd.service",
	"dkm": "dkm.service",
}

func (t Dogeboxd) GetLogChannel(PupID string) (context.CancelFunc, chan string, error) {
	// We read dogeboxd and dkm from the host systemd journal,
	// and read everything else (pups) from the container logs we export.
	service, ok := allowedJournalServices[PupID]
	if ok {
		return t.JournalReader.GetJournalChannel(service)
	}

	// Check that we've actually got a valid pup id.
	_, _, err := t.Pups.GetPup(PupID)
	if err != nil {
		return nil, nil, err
	}

	return t.logtailer.GetChannel(PupID)
}

// GetJobLogChannel returns a log channel for a specific job
// Streams logs from the job's ActionLogger in real-time (same system as pup logs)
func (t Dogeboxd) GetJobLogChannel(JobID string) (context.CancelFunc, chan string, error) {
	// Verify job exists
	_, err := t.JobManager.GetJob(JobID)
	if err != nil {
		return nil, nil, fmt.Errorf("job not found: %s", JobID)
	}

	// Get log channel from the action logger for this job
	return t.logtailer.GetChannel(JobID)
}
