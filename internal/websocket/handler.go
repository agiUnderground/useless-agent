package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

// SendTaskUpdate sends a task update to all WebSocket clients
func SendTaskUpdate(taskID, status, message string) {
	wsmutex.Lock()
	defer wsmutex.Unlock()

	update := map[string]interface{}{
		"type":    "taskUpdate",
		"taskId":  taskID,
		"status":  status,
		"message": message,
	}

	updateJSON, err := json.Marshal(update)
	if err != nil {
		log.Println("Error marshaling task update:", err)
		return
	}

	// Use a goroutine to send updates without blocking
	go func() {
		// Create a copy of connections to avoid holding mutex for too long
		wsmutex.Lock()
		connections := make([]*WebSocketConnection, len(websocketConnections))
		copy(connections, websocketConnections)
		wsmutex.Unlock()

		for _, wsConn := range connections {
			err := safeWrite(wsConn, websocket.TextMessage, updateJSON)
			if err != nil {
				log.Println("Error sending task update:", err)
			}
		}
	}()
}

// SendTokenUpdate sends a token update to all WebSocket clients
func SendTokenUpdate(total int) {
	wsmutex.Lock()
	defer wsmutex.Unlock()

	update := map[string]interface{}{
		"type":  "tokenUpdate",
		"total": total,
	}
	updateJSON, err := json.Marshal(update)
	if err != nil {
		log.Println("Error marshaling token update:", err)
		return
	}

	// Use a goroutine to send updates without blocking
	go func() {
		// Create a copy of connections to avoid holding mutex for too long
		wsmutex.Lock()
		connections := make([]*WebSocketConnection, len(websocketConnections))
		copy(connections, websocketConnections)
		wsmutex.Unlock()

		for _, wsConn := range connections {
			err := safeWrite(wsConn, websocket.TextMessage, updateJSON)
			if err != nil {
				log.Println("Error sending token update:", err)
			}
		}
	}()
}

// BroadcastMessage broadcasts a message to all WebSocket clients
func BroadcastMessage(messageType string, data interface{}) {
	wsmutex.Lock()
	defer wsmutex.Unlock()

	update := map[string]interface{}{
		"type": messageType,
		"data": data,
	}
	updateJSON, err := json.Marshal(update)
	if err != nil {
		log.Println("Error marshaling broadcast message:", err)
		return
	}

	// Use a goroutine to send updates without blocking
	go func() {
		// Create a copy of connections to avoid holding mutex for too long
		wsmutex.Lock()
		connections := make([]*WebSocketConnection, len(websocketConnections))
		copy(connections, websocketConnections)
		wsmutex.Unlock()

		for _, wsConn := range connections {
			err := safeWrite(wsConn, websocket.TextMessage, updateJSON)
			if err != nil {
				log.Println("Error sending broadcast message:", err)
			}
		}
	}()
}

// SendLogMessage sends log data to all WebSocket clients
func SendLogMessage(logData string) {
	wsmutex.Lock()
	defer wsmutex.Unlock()

	update := map[string]interface{}{
		"type": "log",
		"data": logData,
	}

	updateJSON, err := json.Marshal(update)
	if err != nil {
		log.Println("Error marshaling log message:", err)
		return
	}

	// Use a goroutine to send updates without blocking
	go func() {
		// Create a copy of connections to avoid holding mutex for too long
		wsmutex.Lock()
		connections := make([]*WebSocketConnection, len(websocketConnections))
		copy(connections, websocketConnections)
		wsmutex.Unlock()

		for _, wsConn := range connections {
			err := safeWrite(wsConn, websocket.TextMessage, updateJSON)
			if err != nil {
				log.Println("Error sending log message:", err)
			}
		}
	}()
}
