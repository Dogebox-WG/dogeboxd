package dbxdev

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/net/websocket"
)

// connectWebSocketCmd establishes websocket connection to dogeboxd
func connectWebSocketCmd(token string) tea.Cmd {
	return func() tea.Msg {
		// Create websocket config
		wsURL := fmt.Sprintf("ws://dogeboxd/ws/state/?token=%s", token)
		origin := "http://dogeboxd"
		config, err := websocket.NewConfig(wsURL, origin)
		if err != nil {
			return wsConnectedMsg{connected: false, err: err}
		}

		// Connect to unix socket
		conn, err := getSocketConn()
		if err != nil {
			return wsConnectedMsg{connected: false, err: err}
		}

		// Create websocket client
		ws, err := websocket.NewClient(config, conn)
		if err != nil {
			conn.Close()
			return wsConnectedMsg{connected: false, err: err}
		}

		// Start reading messages
		go readWebSocketMessages(ws)

		return wsConnectedMsg{connected: true, err: nil}
	}
}

var wsConn *websocket.Conn

func readWebSocketMessages(conn *websocket.Conn) {
	wsConn = conn
	defer conn.Close()

	for {
		var msg wsMessage
		err := websocket.JSON.Receive(conn, &msg)
		if err != nil {
			log.Printf("websocket read error: %v", err)
			if program != nil {
				program.Send(wsConnectedMsg{connected: false, err: err})
			}
			return
		}

		// Handle different message types
		switch msg.Type {
		case "progress":
			// Handle installation progress messages (ActionProgress)
			if progressData, ok := msg.Update.(map[string]interface{}); ok {
				if progressMsg, ok := progressData["msg"].(string); ok && progressMsg != "" {
					step := ""
					if s, ok := progressData["step"].(string); ok {
						step = s
					}
					logMsg := progressMsg
					if step != "" {
						logMsg = fmt.Sprintf("[%s] %s", step, progressMsg)
					}
					if program != nil {
						program.Send(wsLogMsg{message: logMsg})
					}
				}
			}
		case "action":
			// Check if this is an action completion message
			if msg.ID != "" && msg.Error == "" {
				// This might be a successful action completion
				if pupData, ok := msg.Update.(map[string]interface{}); ok {
					// Check if this is a pup state (has an 'id' field)
					if pupID, ok := pupData["id"].(string); ok {
						// Extract pup name
						pupName := ""
						if manifest, ok := pupData["manifest"].(map[string]interface{}); ok {
							if meta, ok := manifest["meta"].(map[string]interface{}); ok {
								pupName, _ = meta["name"].(string)
							}
						}

						if program != nil {
							program.Send(actionCompleteMsg{
								jobID:   msg.ID,
								pupID:   pupID,
								pupName: pupName,
								success: true,
								error:   "",
							})
						}
					}
				}
			} else if msg.ID != "" && msg.Error != "" {
				// Action failed
				if program != nil {
					program.Send(actionCompleteMsg{
						jobID:   msg.ID,
						success: false,
						error:   msg.Error,
					})
				}
			}

			// Also check if this is a log message with progress info
			if progressData, ok := msg.Update.(map[string]interface{}); ok {
				if progressMsg, ok := progressData["msg"].(string); ok {
					if program != nil {
						program.Send(wsLogMsg{message: progressMsg})
					}
				}
			}
		case "pup":
			// Handle pup state updates
			if pupData, ok := msg.Update.(map[string]interface{}); ok {
				pupID, _ := pupData["id"].(string)
				installation, _ := pupData["installation"].(string)

				// Extract pup name from manifest.meta.name
				pupName := ""
				if manifest, ok := pupData["manifest"].(map[string]interface{}); ok {
					if meta, ok := manifest["meta"].(map[string]interface{}); ok {
						pupName, _ = meta["name"].(string)
					}
				}

				if program != nil && (pupID != "" || pupName != "") && installation != "" {
					program.Send(pupStateMsg{
						pupID:   pupID,
						pupName: pupName,
						state:   installation,
					})
				}
			}
		case "recovery":
			// Handle recovery messages (system logs)
			if logMsg, ok := msg.Update.(string); ok {
				if program != nil {
					program.Send(wsLogMsg{message: logMsg})
				}
			}
		}
	}
}

// closeWebSocket closes the websocket connection if open
func closeWebSocket() {
	if wsConn != nil {
		wsConn.Close()
		wsConn = nil
	}
}
