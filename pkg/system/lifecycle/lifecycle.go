package lifecycle

import (
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func NewLifecycleManager(config dogeboxd.ServerConfig) dogeboxd.LifecycleManager {
	// TODO: Do some discovery
	return LifecycleManagerLinux{config: config}
}
