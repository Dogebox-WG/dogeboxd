package system

import (
	_ "embed"
	"encoding/json"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

type Timezone struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
	// File timezones.json contains manually generated timezone list.
	// TODO: Find a way to automatically generate this list
	//go:embed timezones.json
	tz_data        []byte
	tz_precompiled = func() (s []Timezone) {
		if err := json.Unmarshal(tz_data, &s); err != nil {
			panic(err)
		}
		return
	}()
)

func (t SystemUpdater) TimezoneUpdate(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)
	t.nix.UpdateFirewallRules(patch, dbxState)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(patch, values)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to commit system state: %v", err)
		return err
	}

	return nil
}

func GetTimezones() ([]Timezone, error) {
	return tz_precompiled, nil
}

func GetTimezone() (string, error) {
	timezoneString, err := nix.GetConfigValue("time.timeZone")
	if err != nil {
		return "", err
	}
	return timezoneString, nil
}
