package dogeboxd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These test the job retry path, which stores serialized
// ActionPayload on JobRecord and later reconstructs the original Action.

func TestSerializeDeserializeInstallPup(t *testing.T) {
	payload, err := SerializeAction(InstallPup{
		PupName:      "core",
		PupVersion:   "1.2.3",
		SourceId:     "source-1",
		SessionToken: "session-token",
	})
	require.NoError(t, err)

	action, err := DeserializeAction(payload)
	require.NoError(t, err)

	installAction, ok := action.(InstallPup)
	require.True(t, ok)
	assert.Equal(t, "core", installAction.PupName)
	assert.Equal(t, "1.2.3", installAction.PupVersion)
	assert.Equal(t, "source-1", installAction.SourceId)
	assert.Equal(t, "session-token", installAction.SessionToken)
}

func TestSerializeDeserializeInstallPups(t *testing.T) {
	payload, err := SerializeAction(InstallPups{
		{PupName: "core", PupVersion: "1.2.3"},
		{PupName: "wallet", PupVersion: "4.5.6"},
	})
	require.NoError(t, err)

	action, err := DeserializeAction(payload)
	require.NoError(t, err)

	installActions, ok := action.(InstallPups)
	require.True(t, ok)
	require.Len(t, installActions, 2)
	assert.Equal(t, "core", installActions[0].PupName)
	assert.Equal(t, "wallet", installActions[1].PupName)
}
