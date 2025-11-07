package mouse

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-vgo/robotgo"
)

var mouseMutex sync.Mutex

// Coordinate represents mouse coordinates
type Coordinate struct {
	X int `json:"X"`
	Y int `json:"y"`
}

// GetCursorPosition gets the current cursor position
func GetCursorPosition() (int, int) {
	x, y := robotgo.Location()
	log.Printf("getCursorPosition, current mouse position [%d,%d]", x, y)
	return x, y
}

// GetCursorPositionJSON gets the current cursor position as JSON
func GetCursorPositionJSON() (string, error) {
	x, y := robotgo.Location()
	log.Printf("getCursorPositionJSON, current mouse position [%d,%d]", x, y)

	var cursorCoord Coordinate
	cursorCoord.X = x
	cursorCoord.Y = y
	jsonData, err := json.Marshal(cursorCoord)
	if err != nil {
		log.Println("failed to json.Marshal cursor position.")
		return "", nil
	}
	log.Printf("getCursorPositionJSON, current mouse position json string: %s", string(jsonData))

	return string(jsonData), nil
}

// MouseInputHandler handles mouse input HTTP requests
func MouseInputHandler(w http.ResponseWriter, r *http.Request) {
	var x int
	var y int

	x, _ = strconv.Atoi(r.URL.Query().Get("x"))
	y, _ = strconv.Atoi(r.URL.Query().Get("y"))

	mouseMutex.Lock()
	func() {
		defer mouseMutex.Unlock()
		robotgo.MoveSmoothRelative(x, y)
	}()

	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result": "ok",
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// MouseClickHandler handles mouse click HTTP requests
func MouseClickHandler(w http.ResponseWriter, r *http.Request) {
	robotgo.Click()

	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result": "ok",
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}
