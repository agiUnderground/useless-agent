package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"useless-agent/internal/storage"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust as needed for CORS
	},
	HandshakeTimeout: 10 * time.Second,
}

// WebSocketConnection wraps a websocket connection with its own mutex
type WebSocketConnection struct {
	conn   *websocket.Conn
	mutex  sync.Mutex
	closed bool
}

var websocketConnections []*WebSocketConnection
var wsmutex sync.Mutex

// Helper function to remove a failed connection
func removeConnection(wsConn *WebSocketConnection) {
	wsmutex.Lock()
	defer wsmutex.Unlock()
	for i, c := range websocketConnections {
		if c == wsConn {
			websocketConnections = append(websocketConnections[:i], websocketConnections[i+1:]...)
			wsConn.mutex.Lock()
			if !wsConn.closed {
				wsConn.conn.Close()
				wsConn.closed = true
			}
			wsConn.mutex.Unlock()
			break
		}
	}
}

// Helper function to safely write to a WebSocket connection
func safeWrite(wsConn *WebSocketConnection, messageType int, data []byte) error {
	wsConn.mutex.Lock()
	defer wsConn.mutex.Unlock()

	if wsConn.closed {
		return fmt.Errorf("connection is closed")
	}

	// Set write deadline to prevent blocking
	wsConn.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	err := wsConn.conn.WriteMessage(messageType, data)
	if err != nil {
		// Mark as closed and remove from connections
		wsConn.closed = true
		go removeConnection(wsConn)
	}
	return err
}

// WSHandler handles WebSocket connections
func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	wsConn := &WebSocketConnection{
		conn:   conn,
		mutex:  sync.Mutex{},
		closed: false,
	}

	wsmutex.Lock()
	websocketConnections = append(websocketConnections, wsConn)
	wsmutex.Unlock()

	// Start/rotate persistent session log using client IP
	clientIP := r.RemoteAddr
	if host, _, splitErr := net.SplitHostPort(r.RemoteAddr); splitErr == nil {
		clientIP = host
	}
	storage.StartSessionLog(clientIP, fmt.Sprintf("ws-%d", time.Now().Unix()))

	defer func() {
		wsConn.mutex.Lock()
		if !wsConn.closed {
			wsConn.conn.Close()
			wsConn.closed = true
		}
		wsConn.mutex.Unlock()
		removeConnection(wsConn)
	}()

	send := func(message string) {
		err := safeWrite(wsConn, websocket.TextMessage, []byte(message))
		if err != nil {
			return
		}
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				currentTime := time.Now()
				send(currentTime.Format("2006-01-02 15:04:05"))
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}
	}
}

// broadcastJSON marshals payload and writes it synchronously (in strict FIFO order)
// to all active WebSocket connections. Each connection has a 2s write deadline.
func broadcastJSON(label string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling %s: %v", label, err)
		return
	}

	wsmutex.Lock()
	connections := make([]*WebSocketConnection, len(websocketConnections))
	copy(connections, websocketConnections)
	wsmutex.Unlock()

	for _, wsConn := range connections {
		if err := safeWrite(wsConn, websocket.TextMessage, data); err != nil {
			log.Printf("Error sending %s: %v", label, err)
		}
	}
}

// SendTaskUpdate sends a task update to all WebSocket clients
func SendTaskUpdate(taskID, status, message, model, thinkingLevel string) {
	update := map[string]interface{}{
		"type":    "taskUpdate",
		"taskId":  taskID,
		"status":  status,
		"message": message,
	}
	if model != "" {
		update["model"] = model
	}
	if thinkingLevel != "" {
		update["thinkingLevel"] = thinkingLevel
	}
	broadcastJSON("task update", update)
}

// SendTokenUpdate sends a token update to all WebSocket clients
func SendTokenUpdate(total int) {
	broadcastJSON("token update", map[string]interface{}{
		"type":  "tokenUpdate",
		"total": total,
	})
}

// BroadcastMessage broadcasts a message to all WebSocket clients
func BroadcastMessage(messageType string, data interface{}) {
	broadcastJSON("broadcast message", map[string]interface{}{
		"type": messageType,
		"data": data,
	})
}

// SendLogMessage sends log data to all WebSocket clients
func SendLogMessage(logData string) {
	broadcastJSON("log message", map[string]interface{}{
		"type": "log",
		"data": logData,
	})
}

// SendExecutionEngineUpdate sends execution engine state updates to all WebSocket clients
func SendExecutionEngineUpdate(updateType string, data interface{}) {
	broadcastJSON("execution engine update", map[string]interface{}{
		"type":       "executionEngineUpdate",
		"updateType": updateType,
		"data":       data,
	})
}

// SendSubtaskUpdate sends subtask updates to all WebSocket clients
func SendSubtaskUpdate(taskID string, subtaskID int, description string, isActive bool, actions []interface{}) {
	broadcastJSON("subtask update", map[string]interface{}{
		"type":      "subtaskUpdate",
		"taskId":    taskID,
		"subtaskId": subtaskID,
		"subtask": map[string]interface{}{
			"id":          subtaskID,
			"description": description,
			"isActive":    isActive,
			"actions":     actions,
		},
	})
}

// SendActionUpdate sends action updates to all WebSocket clients
func SendActionUpdate(taskID string, subtaskID int, actionIndex int, action interface{}) {
	broadcastJSON("action update", map[string]interface{}{
		"type":        "actionUpdate",
		"taskId":      taskID,
		"subtaskId":   subtaskID,
		"actionIndex": actionIndex,
		"action":      action,
	})
}
