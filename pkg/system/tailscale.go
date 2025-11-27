package system

import (
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t SystemUpdater) tailscaleUpdate(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)

	// Update firewall rules to allow Tailscale
	t.nix.UpdateFirewallRules(patch, dbxState)

	// Update Tailscale service configuration
	t.nix.UpdateTailscale(patch, dbxState.Tailscale, dbxState.Hostname)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to update Tailscale configuration: %v", err)
		return err
	}

	return nil
}

func (t SystemUpdater) EnableTailscale(l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox
	state.Tailscale.Enabled = true

	if err := t.sm.SetDogebox(state); err != nil {
		return err
	}

	return t.tailscaleUpdate(state, l)
}

func (t SystemUpdater) DisableTailscale(l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox
	state.Tailscale.Enabled = false

	if err := t.sm.SetDogebox(state); err != nil {
		return err
	}

	return t.tailscaleUpdate(state, l)
}

func (t SystemUpdater) SetTailscaleConfig(config dogeboxd.SetTailscaleConfig, l dogeboxd.SubLogger) error {
	state := t.sm.Get().Dogebox

	// Update config values
	state.Tailscale.AuthKey = config.AuthKey
	state.Tailscale.Hostname = config.Hostname
	state.Tailscale.AdvertiseRoutes = config.AdvertiseRoutes
	state.Tailscale.Tags = config.Tags
	if config.ListenPort > 0 {
		state.Tailscale.ListenPort = config.ListenPort
	}

	if err := t.sm.SetDogebox(state); err != nil {
		return err
	}

	// Only apply if Tailscale is enabled
	if state.Tailscale.Enabled {
		return t.tailscaleUpdate(state, l)
	}

	return nil
}

func (t SystemUpdater) GetTailscaleState() dogeboxd.DogeboxStateTailscaleConfig {
	return t.sm.Get().Dogebox.Tailscale
}
