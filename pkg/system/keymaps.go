package system

import (
	_ "embed"
	"encoding/json"

	"github.com/dogeorg/dogeboxd/pkg/system/nix"
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
