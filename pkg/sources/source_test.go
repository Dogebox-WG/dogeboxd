package source

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
)

func TestAddSourcePendingPersistsConfiguration(t *testing.T) {
	store, err := dogeboxd.NewStoreManager(":memory:")
	if err != nil {
		t.Fatalf("NewStoreManager returned error: %v", err)
	}
	defer store.CloseDB()

	stateManager := system.NewStateManager(store)
	manager := NewSourceManager(dogeboxd.ServerConfig{}, stateManager, nil)

	location := "https://github.com/Dogebox-WG/pups.git"
	if err := manager.AddSourcePending(location); err != nil {
		t.Fatalf("AddSourcePending returned error: %v", err)
	}

	configs := manager.GetAllSourceConfigurations()
	if len(configs) != 1 {
		t.Fatalf("expected 1 source config, got %d", len(configs))
	}

	config := configs[0]
	if config.ID != pendingSourceID(location) {
		t.Fatalf("expected source ID %q, got %q", pendingSourceID(location), config.ID)
	}
	if config.Name != location {
		t.Fatalf("expected source name %q, got %q", location, config.Name)
	}
	if config.Location != location {
		t.Fatalf("expected source location %q, got %q", location, config.Location)
	}
	if config.Type != "git" {
		t.Fatalf("expected source type %q, got %q", "git", config.Type)
	}

	reloadedManager := NewSourceManager(dogeboxd.ServerConfig{}, stateManager, nil)
	reloadedConfigs := reloadedManager.GetAllSourceConfigurations()
	if len(reloadedConfigs) != 1 {
		t.Fatalf("expected 1 reloaded source config, got %d", len(reloadedConfigs))
	}
	if reloadedConfigs[0] != config {
		t.Fatalf("expected reloaded config %+v, got %+v", config, reloadedConfigs[0])
	}
}

func TestAddSourcePendingRejectsDuplicateLocation(t *testing.T) {
	store, err := dogeboxd.NewStoreManager(":memory:")
	if err != nil {
		t.Fatalf("NewStoreManager returned error: %v", err)
	}
	defer store.CloseDB()

	stateManager := system.NewStateManager(store)
	manager := NewSourceManager(dogeboxd.ServerConfig{}, stateManager, nil)

	location := "https://github.com/Dogebox-WG/pups.git"
	if err := manager.AddSourcePending(location); err != nil {
		t.Fatalf("first AddSourcePending returned error: %v", err)
	}

	if err := manager.AddSourcePending(location); err == nil {
		t.Fatal("expected duplicate AddSourcePending to fail, got nil")
	}
}
