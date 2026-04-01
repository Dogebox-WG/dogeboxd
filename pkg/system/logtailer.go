package system

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

const maxInitialLines = 1000
const tailReadChunkSize int64 = 8192
const logFileOpenAttempts = 300
const logFileOpenRetryDelay = 100 * time.Millisecond

func NewLogTailer(config dogeboxd.ServerConfig) LogTailer {
	return LogTailer{
		config: config,
	}
}

type LogTailer struct {
	config dogeboxd.ServerConfig
}

func (t LogTailer) GetChannel(pupId string) (context.CancelFunc, chan string, error) {
	return t.GetChannelFromOffset(pupId, -1)
}

func (t LogTailer) GetChannelFromOffset(pupId string, startOffset int64) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan string, 10)

	go func() {
		logFile := filepath.Join(t.config.ContainerLogDir, "pup-"+pupId)

		// Wait for the file to be created (up to 30 seconds)
		file, err := waitForLogFile(logFile)
		if err != nil {
			// File never appeared, close the channel
			log.Printf("Log file never appeared: %s", logFile)
			close(out)
			return
		}
		defer file.Close()

		log.Printf("Opened log file: %s", file.Name())

		offset, err := resolveStartOffset(file, startOffset)
		if err != nil {
			close(out)
			return
		}

		_, err = file.Seek(offset, io.SeekStart)
		if err != nil {
			close(out)
			return
		}

		reader := bufio.NewReader(file)

		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					close(out)
					return
				}
				out <- line
			}
		}

	}()
	return cancel, out, nil
}

func (t LogTailer) GetTail(pupId string, limit int) ([]string, int64, error) {
	if limit <= 0 {
		return nil, 0, fmt.Errorf("Log tail limit must be greater than zero")
	}

	logFile := filepath.Join(t.config.ContainerLogDir, "pup-"+pupId)

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, 0, nil
		}
		return nil, 0, err
	}
	defer file.Close()

	lines, cursor, err := readLastLines(file, limit)
	if err != nil {
		return nil, 0, err
	}

	return lines, cursor, nil
}

func waitForLogFile(logFile string) (*os.File, error) {
	var file *os.File
	var err error
	for i := 0; i < logFileOpenAttempts; i++ {
		if i > 0 {
			time.Sleep(logFileOpenRetryDelay)
		}

		file, err = os.Open(logFile)
		if err == nil {
			return file, nil
		}
	}

	return nil, err
}

func resolveStartOffset(file *os.File, requestedOffset int64) (int64, error) {
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}

	size := stat.Size()
	if requestedOffset < 0 || requestedOffset > size {
		return size, nil
	}

	return requestedOffset, nil
}

func readLastLines(file *os.File, limit int) ([]string, int64, error) {
	if limit <= 0 {
		return nil, 0, fmt.Errorf("Log tail limit must be greater than zero")
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	endOffset := stat.Size()
	if endOffset == 0 {
		return []string{}, 0, nil
	}

	trailingByte := make([]byte, 1)
	_, err = file.ReadAt(trailingByte, endOffset-1)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}

	newlinesNeeded := limit
	if trailingByte[0] == '\n' {
		newlinesNeeded++
	}

	startOffset := int64(0)
	foundBoundary := false

	for currentOffset := endOffset; currentOffset > 0 && !foundBoundary; {
		chunkStart := currentOffset - tailReadChunkSize
		if chunkStart < 0 {
			chunkStart = 0
		}

		chunk := make([]byte, currentOffset-chunkStart)
		_, err = file.ReadAt(chunk, chunkStart)
		if err != nil && err != io.EOF {
			return nil, 0, err
		}

		for idx := len(chunk) - 1; idx >= 0; idx-- {
			if chunk[idx] != '\n' {
				continue
			}

			newlinesNeeded--
			if newlinesNeeded == 0 {
				startOffset = chunkStart + int64(idx) + 1
				foundBoundary = true
				break
			}
		}

		currentOffset = chunkStart
	}

	reader := io.NewSectionReader(file, startOffset, endOffset-startOffset)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, err
	}

	lines := splitLogLines(data)
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	return lines, endOffset, nil
}

func splitLogLines(data []byte) []string {
	if len(data) == 0 {
		return []string{}
	}

	parts := bytes.Split(data, []byte{'\n'})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}

	lines := make([]string, len(parts))
	for i, part := range parts {
		lines[i] = string(part)
	}

	return lines
}
