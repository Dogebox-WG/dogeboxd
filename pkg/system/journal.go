package system

import (
	"context"
	"fmt"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/coreos/go-systemd/sdjournal"
)

func NewJournalReader(config dogeboxd.ServerConfig) JournalReader {
	return JournalReader{
		config: config,
	}
}

type JournalReader struct {
	config dogeboxd.ServerConfig
}

func (t JournalReader) GetJournalChannel(service string) (context.CancelFunc, chan string, error) {
	return t.getJournalChannel(service, nil)
}

func (t JournalReader) GetJournalChannelFromCursor(service string, cursor string) (context.CancelFunc, chan string, error) {
	return t.getJournalChannel(service, &cursor)
}

func (t JournalReader) getJournalChannel(service string, cursor *string) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan string, 10)

	go func() {
		j, err := sdjournal.NewJournal()
		if err != nil {
			fmt.Println(err)
			close(out)
			return
		}
		defer j.Close()

		// Add a match for the specific service
		err = j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service))
		if err != nil {
			fmt.Println(err)
			close(out)
			return
		}

		if cursor != nil {
			err = j.SeekCursor(*cursor)
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}

			// Advance once so subsequent reads only emit entries after the cursor.
			_, err = j.Next()
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}
		} else {
			// Seek to the end of the journal
			err = j.SeekTail()
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			default:
				i, err := j.Next()
				if err != nil {
					fmt.Println("!!", err)
					continue
				}

				if i == 0 {
					time.Sleep(time.Second)
					continue
				}

				entry, err := j.GetEntry()
				if err != nil {
					continue
				}

				out <- entry.Fields["MESSAGE"]
			}
		}
	}()
	return cancel, out, nil
}

func (t JournalReader) GetJournalTail(service string, limit int) ([]string, *string, error) {
	if limit <= 0 {
		return nil, nil, fmt.Errorf("Log tail limit must be greater than zero")
	}

	j, err := sdjournal.NewJournal()
	if err != nil {
		return nil, nil, err
	}
	defer j.Close()

	err = j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service))
	if err != nil {
		return nil, nil, err
	}

	err = j.SeekTail()
	if err != nil {
		return nil, nil, err
	}

	_, err = j.PreviousSkip(uint64(limit))
	if err != nil {
		return nil, nil, err
	}

	lines := []string{}
	var lastCursor *string
	for {
		n, err := j.Next()
		if err != nil {
			return nil, nil, err
		}
		if n == 0 {
			break
		}

		entry, err := j.GetEntry()
		if err != nil {
			return nil, nil, err
		}

		cursor, err := j.GetCursor()
		if err != nil {
			return nil, nil, err
		}

		lastCursor = &cursor
		lines = append(lines, entry.Fields["MESSAGE"])
	}

	return lines, lastCursor, nil
}
