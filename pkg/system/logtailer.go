package system

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewLogTailer(config dogeboxd.ServerConfig) LogTailer {
	return LogTailer{
		config: config,
	}
}

type LogTailer struct {
	config dogeboxd.ServerConfig
}

func (t LogTailer) GetChan(pupId string) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan string, 10)

	go func() {
		logFile := filepath.Join(t.config.ContainerLogDir, "pup-"+pupId)

		// Wait for the file to be created (up to 30 seconds)
		var file *os.File
		var err error
		for i := 0; i < 300; i++ { // 300 * 100ms = 30 seconds
			file, err = os.Open(logFile)
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if err != nil {
			// File never appeared, close the channel
			log.Printf("Log file never appeared: %s", logFile)
			close(out)
			return
		}
		defer file.Close()

		log.Printf("Opened log file: %s", file.Name())

		// First, send existing content
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			close(out)
			return
		}

		// Read and send all existing content
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				close(out)
				return
			case out <- scanner.Text():
			}
		}

		// Now seek to the end for live streaming
		_, err = file.Seek(0, io.SeekEnd)
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
