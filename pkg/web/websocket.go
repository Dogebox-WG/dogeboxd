package web

import (
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"golang.org/x/net/websocket"
)

// Represents a websocket connection from a client
type WSCONN struct {
	WS   *websocket.Conn
	Stop chan bool
}

func (t *WSCONN) IsClosed() bool {
	return t.Stop == nil
}

func (t *WSCONN) Close() {
	if t.Stop != nil {
		close(t.Stop)
		t.Stop = nil
	}
}

// Handle incomming websocket connections for general updates
func (t api) getUpdateSocket(w http.ResponseWriter, r *http.Request) {
	initialPayload := func() any {
		return dogeboxd.Change{ID: "internal", Error: "", Type: "bootstrap", Update: t.getRawBS()}
	}
	t.ws.GetWSHandler(initialPayload).ServeHTTP(w, r)
}

// Handle incoming websocket connections for pup log output
func (t api) getPupLogSocket(w http.ResponseWriter, r *http.Request) {
	PupID := r.PathValue("PupID")
	wh, err := GetLogHandler(PupID, t.dbx)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error establishing pup log channel")
		return
	}
	wh.ServeHTTP(w, r)
}

// Handle incoming websocket connections for job log output
func (t api) getJobLogSocket(w http.ResponseWriter, r *http.Request) {
	JobID := r.PathValue("JobID")
	wh, err := GetJobLogHandler(JobID, t.dbx)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error establishing job log channel: "+err.Error())
		return
	}
	wh.ServeHTTP(w, r)
}

// Handle incoming websocket connections for activity updates
func (t api) getActivitiesSocket(w http.ResponseWriter, r *http.Request) {
	wh := t.GetActivitiesHandler()
	wh.ServeHTTP(w, r)
}
