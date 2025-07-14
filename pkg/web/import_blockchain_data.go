package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync/atomic"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var (
	importInProgress atomic.Bool
)

func (t api) importBlockchainData(w http.ResponseWriter, r *http.Request) {
	// Prevent duplicate imports using atomic compare-and-swap
	if !importInProgress.CompareAndSwap(false, true) {
		sendErrorResponse(w, http.StatusConflict, "Blockchain import already in progress")
		return
	}

	// Reset the flag when the function returns
	defer func() {
		importInProgress.Store(false)
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
