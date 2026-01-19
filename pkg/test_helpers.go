package dogeboxd

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// UnknownAction is a test-only action type for testing unknown action handling
type UnknownAction struct {
	Type string
}

func (UnknownAction) ActionName() string { return "unknown" }

// setupTestJobManager creates a JobManager with a test database
func setupTestJobManager() (*JobManager, error) {
	// Create a temporary database file for testing
	dbPath := ":memory:"
	sm, err := NewStoreManager(dbPath)
	if err != nil {
		return nil, err
	}

	jm := NewJobManager(sm, nil)

	return jm, nil
}

// setupTestJobManagerWithDBX creates a JobManager with a minimal Dogeboxd instance
func setupTestJobManagerWithDBX() (*JobManager, *testDogeboxd, error) {
	// Create a temporary database file for testing
	dbPath := ":memory:"
	sm, err := NewStoreManager(dbPath)
	if err != nil {
		return nil, nil, err
	}

	// Create a minimal Dogeboxd instance for testing
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}

	jm := NewJobManager(sm, testDBX)
	jm.SetDogeboxd(testDBX)

	// Start a goroutine to collect changes
	changes := make([]Change, 0)
	go func() {
		for change := range testDBX.Changes {
			changes = append(changes, change)
		}
	}()

	return jm, &testDogeboxd{changes: changes, dbx: testDBX}, nil
}

// testDogeboxd is a test helper to track changes
type testDogeboxd struct {
	changes []Change
	dbx     *Dogeboxd
}

func (t *testDogeboxd) GetChanges() []Change {
	return t.changes
}

// createTestJob creates a test Job with the specified action type
func createTestJob(actionType string) Job {
	job := Job{
		ID:    fmt.Sprintf("test-job-%d", time.Now().UnixNano()),
		Start: time.Now(),
	}

	switch actionType {
	case "InstallPup":
		job.A = InstallPup{PupName: "test-app"}
	case "InstallPups":
		job.A = InstallPups{{PupName: "test-app"}}
	case "UninstallPup":
		job.A = UninstallPup{PupID: "test-pup-id"}
	case "PurgePup":
		job.A = PurgePup{PupID: "test-pup-id"}
	case "EnablePup":
		job.A = EnablePup{PupID: "test-pup-id"}
	case "DisablePup":
		job.A = DisablePup{PupID: "test-pup-id"}
	case "UpdatePupConfig":
		job.A = UpdatePupConfig{PupID: "test-pup-id"}
	case "UpdatePupProviders":
		job.A = UpdatePupProviders{PupID: "test-pup-id"}
	case "ImportBlockchainData":
		job.A = ImportBlockchainData{}
	case "UpdatePendingSystemNetwork":
		job.A = UpdatePendingSystemNetwork{}
	case "EnableSSH":
		job.A = EnableSSH{}
	case "DisableSSH":
		job.A = DisableSSH{}
	case "AddSSHKey":
		job.A = AddSSHKey{Key: "ssh-rsa test"}
	case "RemoveSSHKey":
		job.A = RemoveSSHKey{ID: "test-key-id"}
	case "AddBinaryCache":
		job.A = AddBinaryCache{Host: "cache.example.com", Key: "test-key"}
	case "RemoveBinaryCache":
		job.A = RemoveBinaryCache{ID: "test-cache-id"}
	case "UpdateMetrics":
		job.A = UpdateMetrics{}
	default:
		job.A = InstallPup{PupName: "test-app"}
	}

	return job
}

// createTestActionProgress creates a test ActionProgress
func createTestActionProgress(actionID string, progress int, step string, msg string) ActionProgress {
	return ActionProgress{
		ActionID:  actionID,
		PupID:     "test-pup-id",
		Progress:  progress,
		Step:      step,
		Msg:       msg,
		Error:     false,
		StepTaken: time.Second,
	}
}

// createTestActionProgressWithError creates a test ActionProgress with an error
func createTestActionProgressWithError(actionID string, step string, msg string) ActionProgress {
	return ActionProgress{
		ActionID:  actionID,
		PupID:     "test-pup-id",
		Progress:  0,
		Step:      step,
		Msg:       msg,
		Error:     true,
		StepTaken: time.Second,
	}
}

// cleanupTestDB closes the database connection
func cleanupTestDB(db *sql.DB) error {
	if db != nil {
		return db.Close()
	}
	return nil
}
