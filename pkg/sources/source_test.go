package source

import (
	"fmt"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
)

type retryPendingStubSource struct {
	config          dogeboxd.ManifestSourceConfiguration
	validatedConfig dogeboxd.ManifestSourceConfiguration
	validateErr     error
}

func (s retryPendingStubSource) ValidateFromLocation(location string) (dogeboxd.ManifestSourceConfiguration, error) {
	if location != s.config.Location {
		return dogeboxd.ManifestSourceConfiguration{}, fmt.Errorf("unexpected location %q", location)
	}
	if s.validateErr != nil {
		return dogeboxd.ManifestSourceConfiguration{}, s.validateErr
	}
	return s.validatedConfig, nil
}

func (s retryPendingStubSource) Config() dogeboxd.ManifestSourceConfiguration {
	return s.config
}

func (s retryPendingStubSource) List(bool) (dogeboxd.ManifestSourceList, error) {
	return dogeboxd.ManifestSourceList{Config: s.config}, nil
}

func (s retryPendingStubSource) Download(string, map[string]string) error {
	return nil
}

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

func TestRetryPendingSourcesPromotesValidatedConfiguration(t *testing.T) {
	store, err := dogeboxd.NewStoreManager(":memory:")
	if err != nil {
		t.Fatalf("NewStoreManager returned error: %v", err)
	}
	defer store.CloseDB()

	stateManager := system.NewStateManager(store)
	manager := NewSourceManager(dogeboxd.ServerConfig{}, stateManager, nil).(*sourceManager)

	location := "https://github.com/Dogebox-WG/pups.git"
	pendingConfig := dogeboxd.ManifestSourceConfiguration{
		ID:       pendingSourceID(location),
		Name:     location,
		Location: location,
		Type:     "git",
	}
	manager.sources = []dogeboxd.ManifestSource{
		retryPendingStubSource{
			config: pendingConfig,
			validatedConfig: dogeboxd.ManifestSourceConfiguration{
				ID:          "dogebox-official",
				Name:        "Dogebox Official",
				Description: "Official Dogebox pup source",
				Location:    location,
				Type:        "git",
			},
		},
	}

	retriedCount, err := manager.RetryPendingSources()
	if err != nil {
		t.Fatalf("RetryPendingSources returned error: %v", err)
	}
	if retriedCount != 1 {
		t.Fatalf("RetryPendingSources count = %d, want 1", retriedCount)
	}

	configs := manager.GetAllSourceConfigurations()
	if len(configs) != 1 {
		t.Fatalf("expected 1 source config, got %d", len(configs))
	}

	config := configs[0]
	if config.ID != "dogebox-official" {
		t.Fatalf("expected source ID %q, got %q", "dogebox-official", config.ID)
	}
	if config.Name != "Dogebox Official" {
		t.Fatalf("expected source name %q, got %q", "Dogebox Official", config.Name)
	}
	if config.Description != "Official Dogebox pup source" {
		t.Fatalf("expected source description %q, got %q", "Official Dogebox pup source", config.Description)
	}
	if isPendingSourceID(config.ID) {
		t.Fatalf("expected promoted source ID not to be pending, got %q", config.ID)
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
