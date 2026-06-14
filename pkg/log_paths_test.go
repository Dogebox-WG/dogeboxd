package dogeboxd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLogTailer struct {
	lastChannelPath   string
	lastChannelOffset int64
	lastTailPath      string
	lastTailLimit     int
}

func (t *stubLogTailer) GetChannel(path string) (context.CancelFunc, chan string, error) {
	t.lastChannelPath = path
	return func() {}, make(chan string), nil
}

func (t *stubLogTailer) GetChannelFromOffset(path string, offset int64) (context.CancelFunc, chan string, error) {
	t.lastChannelPath = path
	t.lastChannelOffset = offset
	return func() {}, make(chan string), nil
}

func (t *stubLogTailer) GetTail(path string, limit int) ([]string, int64, error) {
	t.lastTailPath = path
	t.lastTailLimit = limit
	return []string{filepath.Base(path)}, 42, nil
}

type stubJournalReader struct {
	lastChannelService string
	lastChannelCursor  string
	lastTailService    string
	lastTailLimit      int
}

func (t *stubJournalReader) GetJournalChannel(service string) (context.CancelFunc, chan string, error) {
	t.lastChannelService = service
	return func() {}, make(chan string), nil
}

func (t *stubJournalReader) GetJournalChannelFromCursor(service string, cursor string) (context.CancelFunc, chan string, error) {
	t.lastChannelService = service
	t.lastChannelCursor = cursor
	return func() {}, make(chan string), nil
}

func (t *stubJournalReader) GetJournalTail(service string, limit int) ([]string, *string, error) {
	t.lastTailService = service
	t.lastTailLimit = limit
	resumeToken := "journal-cursor"
	return []string{service}, &resumeToken, nil
}

func TestDogeboxdGetJobLogTailUsesJobPath(t *testing.T) {
	config := &ServerConfig{ContainerLogDir: t.TempDir()}

	jm, err := setupTestJobManager()
	require.NoError(t, err)

	logtailer := &stubLogTailer{}
	dbx := Dogeboxd{
		JobManager: jm,
		config:     config,
		logtailer:  logtailer,
	}

	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	lines, resumeToken, err := dbx.GetJobLogTail(job.ID, 10)
	require.NoError(t, err)
	require.NotNil(t, resumeToken)
	assert.Equal(t, []string{filepath.Base(config.JobLogPath(job.ID))}, lines)
	assert.Equal(t, config.JobLogPath(job.ID), logtailer.lastTailPath)
	assert.Equal(t, 10, logtailer.lastTailLimit)
}

func TestDogeboxdGetJobLogChannelUsesJobPath(t *testing.T) {
	config := &ServerConfig{ContainerLogDir: t.TempDir()}

	jm, err := setupTestJobManager()
	require.NoError(t, err)

	logtailer := &stubLogTailer{}
	dbx := Dogeboxd{
		JobManager: jm,
		config:     config,
		logtailer:  logtailer,
	}

	job := createTestJob("InstallPup")
	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	t.Run("without resume token", func(t *testing.T) {
		cancel, channel, err := dbx.GetJobLogChannel(job.ID, nil)
		require.NoError(t, err)
		require.NotNil(t, cancel)
		require.NotNil(t, channel)
		assert.Equal(t, config.JobLogPath(job.ID), logtailer.lastChannelPath)
		assert.Equal(t, int64(0), logtailer.lastChannelOffset)
	})

	t.Run("with resume token", func(t *testing.T) {
		resumeToken := "123"
		cancel, channel, err := dbx.GetJobLogChannel(job.ID, &resumeToken)
		require.NoError(t, err)
		require.NotNil(t, cancel)
		require.NotNil(t, channel)
		assert.Equal(t, config.JobLogPath(job.ID), logtailer.lastChannelPath)
		assert.Equal(t, int64(123), logtailer.lastChannelOffset)
	})
}

func TestDogeboxdGetLogChannelUsesJournalSource(t *testing.T) {
	journalReader := &stubJournalReader{}
	logtailer := &stubLogTailer{}
	dbx := Dogeboxd{
		JournalReader: journalReader,
		logtailer:     logtailer,
		config:        &ServerConfig{ContainerLogDir: t.TempDir()},
	}

	t.Run("without resume token", func(t *testing.T) {
		cancel, channel, err := dbx.GetLogChannel("dbx", nil)
		require.NoError(t, err)
		require.NotNil(t, cancel)
		require.NotNil(t, channel)
		assert.Equal(t, "dogeboxd.service", journalReader.lastChannelService)
		assert.Equal(t, "", logtailer.lastChannelPath)
	})

	t.Run("with resume token", func(t *testing.T) {
		resumeToken := "cursor-123"
		cancel, channel, err := dbx.GetLogChannel("dkm", &resumeToken)
		require.NoError(t, err)
		require.NotNil(t, cancel)
		require.NotNil(t, channel)
		assert.Equal(t, "dkm.service", journalReader.lastChannelService)
		assert.Equal(t, "cursor-123", journalReader.lastChannelCursor)
		assert.Equal(t, "", logtailer.lastChannelPath)
	})
}

func TestDogeboxdGetLogTailUsesJournalSource(t *testing.T) {
	journalReader := &stubJournalReader{}
	logtailer := &stubLogTailer{}
	dbx := Dogeboxd{
		JournalReader: journalReader,
		logtailer:     logtailer,
		config:        &ServerConfig{ContainerLogDir: t.TempDir()},
	}

	lines, resumeToken, err := dbx.GetLogTail("dbx", 25)
	require.NoError(t, err)
	require.NotNil(t, resumeToken)
	assert.Equal(t, []string{"dogeboxd.service"}, lines)
	assert.Equal(t, "dogeboxd.service", journalReader.lastTailService)
	assert.Equal(t, 25, journalReader.lastTailLimit)
	assert.Equal(t, "", logtailer.lastTailPath)
}

func TestServerConfigLogFileNames(t *testing.T) {
	config := ServerConfig{ContainerLogDir: "/tmp/logs"}

	assert.Equal(t, "pup-demo", config.PupLogFileName("demo"))
	assert.Equal(t, filepath.Join("/tmp/logs", "pup-demo"), config.PupLogPath("demo"))
	assert.Equal(t, "job-demo", config.JobLogFileName("demo"))
	assert.Equal(t, filepath.Join("/tmp/logs", "job-demo"), config.JobLogPath("demo"))
}
