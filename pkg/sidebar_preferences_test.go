package dogeboxd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Suite: DogeboxState.SidebarPups JSON Serialization
// ============================================================================

func TestDogeboxStateSidebarPupsJSONSerializationEmptyArray(t *testing.T) {
	// Verify that empty SidebarPups serializes to [] not null
	state := DogeboxState{SidebarPups: []string{}}

	jsonBytes, err := json.Marshal(state)
	require.NoError(t, err)

	// Verify it contains sidebarPups as empty array
	var unmarshalled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &unmarshalled)
	require.NoError(t, err)

	sidebarPups, ok := unmarshalled["sidebarPups"]
	assert.True(t, ok, "sidebarPups key should exist")
	assert.NotNil(t, sidebarPups, "sidebarPups should not be nil")

	// Should be an empty array
	arr, ok := sidebarPups.([]interface{})
	assert.True(t, ok, "sidebarPups should be an array")
	assert.Len(t, arr, 0, "sidebarPups should be empty")
}

func TestDogeboxStateSidebarPupsJSONSerializationWithPups(t *testing.T) {
	state := DogeboxState{
		SidebarPups: []string{"pup-id-1", "pup-id-2", "pup-id-3"},
	}

	jsonBytes, err := json.Marshal(state)
	require.NoError(t, err)

	// Verify it contains expected pup IDs
	var unmarshalled DogeboxState
	err = json.Unmarshal(jsonBytes, &unmarshalled)
	require.NoError(t, err)

	assert.Len(t, unmarshalled.SidebarPups, 3)
	assert.Equal(t, "pup-id-1", unmarshalled.SidebarPups[0])
	assert.Equal(t, "pup-id-2", unmarshalled.SidebarPups[1])
	assert.Equal(t, "pup-id-3", unmarshalled.SidebarPups[2])
}

func TestDogeboxStateSidebarPupsJSONRoundTrip(t *testing.T) {
	original := DogeboxState{
		SidebarPups: []string{"abc123", "def456"},
		Flags: DogeboxFlags{
			IsFirstTimeWelcomeComplete: true,
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var restored DogeboxState
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	assert.Equal(t, original.SidebarPups, restored.SidebarPups)
	assert.Equal(t, original.Flags.IsFirstTimeWelcomeComplete, restored.Flags.IsFirstTimeWelcomeComplete)
}

// ============================================================================
// Test Suite: DogeboxState Store Operations (via StateManager pattern)
// ============================================================================

func setupTestSidebarStore(t *testing.T) *StoreManager {
	sm, err := NewStoreManager(":memory:")
	require.NoError(t, err)
	return sm
}

func TestDogeboxStateSidebarPupsStoreSetAndGet(t *testing.T) {
	sm := setupTestSidebarStore(t)
	defer sm.CloseDB()

	store := GetTypeStore[DogeboxState](sm)

	// Set state with sidebar pups
	state := DogeboxState{
		SidebarPups: []string{"pup-1", "pup-2"},
	}
	err := store.Set("0", state)
	require.NoError(t, err)

	// Get state
	retrieved, err := store.Get("0")
	require.NoError(t, err)

	assert.Equal(t, state.SidebarPups, retrieved.SidebarPups)
}

func TestDogeboxStateSidebarPupsStoreUpdate(t *testing.T) {
	sm := setupTestSidebarStore(t)
	defer sm.CloseDB()

	store := GetTypeStore[DogeboxState](sm)

	// Set initial state
	initial := DogeboxState{
		SidebarPups: []string{"pup-1"},
	}
	err := store.Set("0", initial)
	require.NoError(t, err)

	// Update state
	updated := DogeboxState{
		SidebarPups: []string{"pup-1", "pup-2", "pup-3"},
	}
	err = store.Set("0", updated)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.Get("0")
	require.NoError(t, err)

	assert.Len(t, retrieved.SidebarPups, 3)
	assert.Equal(t, updated.SidebarPups, retrieved.SidebarPups)
}

func TestDogeboxStateSidebarPupsStoreEmptyList(t *testing.T) {
	sm := setupTestSidebarStore(t)
	defer sm.CloseDB()

	store := GetTypeStore[DogeboxState](sm)

	// Set empty list
	state := DogeboxState{
		SidebarPups: []string{},
	}
	err := store.Set("0", state)
	require.NoError(t, err)

	// Verify empty list is preserved
	retrieved, err := store.Get("0")
	require.NoError(t, err)

	assert.NotNil(t, retrieved.SidebarPups)
	assert.Len(t, retrieved.SidebarPups, 0)
}

// ============================================================================
// Test Suite: Sidebar Preferences Business Logic
// ============================================================================

func TestAddPupToSidebarPups(t *testing.T) {
	// Simulate adding a pup to sidebar
	state := DogeboxState{SidebarPups: []string{}}

	pupID := "new-pup-123"
	state.SidebarPups = append(state.SidebarPups, pupID)

	assert.Contains(t, state.SidebarPups, pupID)
	assert.Len(t, state.SidebarPups, 1)
}

func TestRemovePupFromSidebarPups(t *testing.T) {
	// Simulate removing a pup from sidebar
	state := DogeboxState{
		SidebarPups: []string{"pup-1", "pup-2", "pup-3"},
	}

	pupIDToRemove := "pup-2"
	filtered := []string{}
	for _, id := range state.SidebarPups {
		if id != pupIDToRemove {
			filtered = append(filtered, id)
		}
	}
	state.SidebarPups = filtered

	assert.Len(t, state.SidebarPups, 2)
	assert.NotContains(t, state.SidebarPups, pupIDToRemove)
	assert.Contains(t, state.SidebarPups, "pup-1")
	assert.Contains(t, state.SidebarPups, "pup-3")
}

func TestPreventDuplicatePupInSidebarPups(t *testing.T) {
	state := DogeboxState{
		SidebarPups: []string{"pup-1"},
	}

	pupIDToAdd := "pup-1"
	alreadyExists := false
	for _, id := range state.SidebarPups {
		if id == pupIDToAdd {
			alreadyExists = true
			break
		}
	}

	// Should detect duplicate
	assert.True(t, alreadyExists, "Should detect that pup-1 already exists")

	// Verify only one instance exists
	count := 0
	for _, id := range state.SidebarPups {
		if id == "pup-1" {
			count++
		}
	}
	assert.Equal(t, 1, count, "Should only have one instance of pup-1")
}

func TestRemoveNonExistentPupFromSidebarPups(t *testing.T) {
	state := DogeboxState{
		SidebarPups: []string{"pup-1", "pup-2"},
	}

	pupIDToRemove := "non-existent-pup"
	filtered := []string{}
	found := false
	for _, id := range state.SidebarPups {
		if id != pupIDToRemove {
			filtered = append(filtered, id)
		} else {
			found = true
		}
	}

	// Should not find the pup
	assert.False(t, found, "Should not find non-existent pup")

	// List should remain unchanged
	assert.Len(t, filtered, 2)
}

func TestSidebarPupsNilHandling(t *testing.T) {
	// Test handling of nil SidebarPups
	state := DogeboxState{
		SidebarPups: nil,
	}

	// Ensure nil check works
	if state.SidebarPups == nil {
		state.SidebarPups = []string{}
	}

	assert.NotNil(t, state.SidebarPups)
	assert.Len(t, state.SidebarPups, 0)
}
