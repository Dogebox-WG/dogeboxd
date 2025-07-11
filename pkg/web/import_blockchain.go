package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var (
	importInProgress bool
	importMutex      sync.Mutex
)

func (t api) importBlockchainData(w http.ResponseWriter, r *http.Request) {
	// Prevent duplicate imports
	importMutex.Lock()
	if importInProgress {
		importMutex.Unlock()
		sendErrorResponse(w, http.StatusConflict, "Blockchain import already in progress")
		return
	}
	importInProgress = true
	importMutex.Unlock()

	// Reset the flag when the function returns
	defer func() {
		importMutex.Lock()
		importInProgress = false
		importMutex.Unlock()
	}()

	// Generate a random ID for this import action
	idBytes := make([]byte, 8)
	_, err := rand.Read(idBytes)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to generate action ID")
		return
	}
	actionID := "import-blockchain-" + hex.EncodeToString(idBytes)

	// Add the blockchain data import action
	t.dbx.AddAction(dogeboxd.ImportBlockchainData{})

	sendResponse(w, map[string]any{
		"success": true,
		"id":      actionID,
		"message": "Import blockchain action initiated",
	})
}
