package websocket

import (
	"encoding/json"
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
}

var websocketConnections []*websocket.Conn
var wsmutex sync.Mutex

// WSHandler handles WebSocket connections
func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	websocketConnections = append(websocketConnections, conn)
	defer conn.Close()

	send := func(message string) {
		err := conn.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			// log.Println("Write error:", err)
			conn.Close()
			wsmutex.Lock()
			func() {
				defer wsmutex.Unlock()
				for i, c := range websocketConnections {
					if c == conn {
						websocketConnections = append(websocketConnections[:i], websocketConnections[i+1:]...)
						break
					}
				}
			}()
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

	for _, conn := range websocketConnections {
		err = conn.WriteMessage(websocket.TextMessage, updateJSON)
		if err != nil {
			log.Println("Error sending task update:", err)
		}
	}
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

	for _, conn := range websocketConnections {
		err = conn.WriteMessage(websocket.TextMessage, updateJSON)
		if err != nil {
			log.Println("Error sending token update:", err)
		}
	}
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

	for _, conn := range websocketConnections {
		err = conn.WriteMessage(websocket.TextMessage, updateJSON)
		if err != nil {
			log.Println("Error sending broadcast message:", err)
		}
	}
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

	for _, conn := range websocketConnections {
		err = conn.WriteMessage(websocket.TextMessage, updateJSON)
		if err != nil {
			log.Println("Error sending log message:", err)
		}
	}
}
