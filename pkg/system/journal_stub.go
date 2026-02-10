//go:build !linux || !cgo
// +build !linux !cgo

package system

import (
	"context"
	"errors"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
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
	return func() {}, nil, errors.New("systemd journal reader unavailable on this platform")
}
