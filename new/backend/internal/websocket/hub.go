package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var defaultHub *Hub

type Hub struct {
	connections map[*Connection]bool
	register    chan *Connection
	unregister  chan *Connection
	broadcast   chan []byte
	mutex       sync.RWMutex
}

type Connection struct {
	conn   *websocket.Conn
	send   chan []byte
	mutex  sync.Mutex
	closed bool
}

func NewHub() *Hub {
	h := &Hub{
		connections: make(map[*Connection]bool),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		broadcast:   make(chan []byte),
	}
	defaultHub = h
	return h
}

func (h *Hub) Run() {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		HandshakeTimeout: 10 * time.Second,
	}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		connection := &Connection{
			conn:   conn,
			send:   make(chan []byte, 256),
			mutex:  sync.Mutex{},
			closed: false,
		}

		h.register <- connection

		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					currentTime := time.Now()
					timeMsg := currentTime.Format("2006-01-02 15:04:05")
					h.broadcast <- []byte(timeMsg)
				case message, ok := <-connection.send:
					if !ok {
						return
					}
					connection.mutex.Lock()
					err := connection.conn.WriteMessage(websocket.TextMessage, message)
					connection.mutex.Unlock()
					if err != nil {
						return
					}
				}
			}
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				h.unregister <- connection
				break
			}
		}
	})

	for {
		select {
		case conn := <-h.register:
			h.mutex.Lock()
			h.connections[conn] = true
			h.mutex.Unlock()

		case conn := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.connections[conn]; ok {
				delete(h.connections, conn)
				conn.mutex.Lock()
				conn.closed = true
				conn.conn.Close()
				conn.mutex.Unlock()
			}
			h.mutex.Unlock()

		case message := <-h.broadcast:
			h.mutex.RLock()
			for conn := range h.connections {
				select {
				case conn.send <- message:
				default:
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func SendLogMessage(logData string) {
	if defaultHub == nil {
		return
	}

	message := map[string]interface{}{
		"type": "log",
		"data": logData,
	}

	data, _ := json.Marshal(message)

	defaultHub.broadcast <- data
}

func SendTaskUpdate(taskID, status, message string) {
	if defaultHub == nil {
		return
	}

	update := map[string]interface{}{
		"type":    "taskUpdate",
		"taskId":  taskID,
		"status":  status,
		"message": message,
	}

	data, _ := json.Marshal(update)

	defaultHub.broadcast <- data
}

func SendTokenUpdate(total int) {
	if defaultHub == nil {
		return
	}

	update := map[string]interface{}{
		"type":  "tokenUpdate",
		"total": total,
	}

	data, _ := json.Marshal(update)

	defaultHub.broadcast <- data
}

func BroadcastMessage(messageType string, data interface{}) {
	if defaultHub == nil {
		return
	}

	message := map[string]interface{}{
		"type": messageType,
		"data": data,
	}

	jsonData, _ := json.Marshal(message)

	defaultHub.broadcast <- jsonData
}

func BroadcastExecutionEngineUpdate(updateType string, data interface{}) {
	if defaultHub == nil {
		return
	}

	update := map[string]interface{}{
		"type":       "executionEngineUpdate",
		"updateType": updateType,
		"data":       data,
	}

	jsonData, _ := json.Marshal(update)
	defaultHub.broadcast <- jsonData
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		HandshakeTimeout: 10 * time.Second,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	connection := &Connection{
		conn:   conn,
		send:   make(chan []byte, 256),
		mutex:  sync.Mutex{},
		closed: false,
	}

	h.register <- connection

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				currentTime := time.Now()
				timeMsg := currentTime.Format("2006-01-02 15:04:05")
				h.broadcast <- []byte(timeMsg)
			case message, ok := <-connection.send:
				if !ok {
					return
				}
				connection.mutex.Lock()
				err := connection.conn.WriteMessage(websocket.TextMessage, message)
				connection.mutex.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			h.unregister <- connection
			break
		}
	}
}

func SendExecutionEngineUpdate(updateType string, data interface{}) {
	if defaultHub == nil {
		return
	}

	update := map[string]interface{}{
		"type":       "executionEngineUpdate",
		"updateType": updateType,
		"data":       data,
	}

	jsonData, _ := json.Marshal(update)

	defaultHub.broadcast <- jsonData
}
