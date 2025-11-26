package system

import (
	_ "embed"
	"encoding/json"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
	"github.com/dogeorg/dogeboxd/pkg/utils"
)

type Keymap struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
	// File keymaps.json contains manually generated keymaps list.
	// TODO: Find a way to automatically generate this list
	//go:embed keymaps.json
	data        []byte
	precompiled = func() (s []Keymap) {
		if err := json.Unmarshal(data, &s); err != nil {
			panic(err)
		}
		return
	}()
)

func (t SystemUpdater) KeymapUpdate(dbxState dogeboxd.DogeboxState, log dogeboxd.SubLogger) error {
	patch := t.nix.NewPatch(log)
	//t.nix.UpdateFirewallRules(patch, dbxState)

	values := utils.GetNixSystemTemplateValues(dbxState)
	t.nix.UpdateSystem(patch, values)

	if err := patch.Apply(); err != nil {
		log.Errf("Failed to commit system state: %v", err)
		return err
	}

	return nil
}

func GetKeymaps() ([]Keymap, error) {
	return precompiled, nil
}

func GetKeymap() (string, error) {
	keymapString, err := nix.GetConfigValue("console.keyMap")
	if err != nil {
		return "", err
	}
	return keymapString, nil
}
