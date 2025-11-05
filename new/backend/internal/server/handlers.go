package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdimage "image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"strconv"

	imagepkg "useless-agent/internal/image"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/task"
	"useless-agent/internal/websocket"
)

type ScreenshotRequest struct {
	Format  string `json:"format"`
	Quality int    `json:"quality"`
}

type LLMInputRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"sessionId,omitempty"`
}

type TaskCancelRequest struct {
	TaskID string `json:"taskId"`
}

type UserAssistRequest struct {
	TaskID  string `json:"taskId"`
	Message string `json:"message"`
}

type Response struct {
	Result    string      `json:"result"`
	TaskID    string      `json:"taskId,omitempty"`
	SessionID string      `json:"sessionId,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func ScreenshotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	img, err := screenshot.CaptureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pngBytes, err := screenshot.EncodeToPNG(img)
	if err != nil {
		http.Error(w, "Failed to encode image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)
}

func Video2Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	img, err := screenshot.CaptureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot with BB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	grayImg := screenshot.ConvertToGrayscale(img)
	img = grayImg
	rgbaImg := stdimage.NewRGBA(img.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), img, stdimage.Point{0, 0}, draw.Src)

	dominantColors := imagepkg.FindDominantColors(rgbaImg)
	dominantColors = dominantColors[:20]

	drawImg := stdimage.NewRGBA(rgbaImg.Bounds())
	draw.Draw(drawImg, drawImg.Bounds(), rgbaImg, stdimage.Point{0, 0}, draw.Src)

	var bbArray []imagepkg.BoundingBox
	bbCounter := 1

	for _, colorElem := range dominantColors {
		mask := imagepkg.CreateMask(rgbaImg, colorElem.Color, 0)
		components := imagepkg.FindConnectedComponents(mask)

		mask2 := imagepkg.CreateMask(rgbaImg, colorElem.Color, 80)
		components2 := imagepkg.FindConnectedComponents(mask2)
		newComponents := append(components, components2...)
		components = newComponents

		for _, component := range components {
			percentage := imagepkg.CalculatePercentage(component, mask)
			if percentage >= 80 {
				x1, y1, x2, y2 := imagepkg.FindBoundingBox(component)
				absy := abs(y1, y2)
				absx := abs(x1, x2)

				if absy < 10 || absx < 10 {
					continue
				}

				borderColor := color.RGBA{R: 255, G: 0, B: 132, A: 255}
				imagepkg.DrawBoundingBox(drawImg, x1, y1, x2, y2, borderColor)

				if absy >= 25 && absx >= 25 {
					bbArray = append(bbArray, imagepkg.BoundingBox{
						ID: bbCounter,
						X:  x1,
						Y:  y1,
						X2: x2,
						Y2: y2,
					})

					borderColor = color.RGBA{R: 12, G: 236, B: 28, A: 255}
					imagepkg.DrawBoundingBox(drawImg, x1+1, y1+1, x1+15, y1+15, borderColor)
					_, err = drawText(drawImg, x1+2, y1+2, []string{strconv.Itoa(bbCounter)})
					if err != nil {
						fmt.Println(err)
					}
					bbCounter++
				}
			}
		}
	}

	buf := new(bytes.Buffer)
	png.Encode(buf, drawImg)
	w.Header().Set("Content-Type", "image/png")
	w.Write(buf.Bytes())
}

func LLMInputHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var request LLMInputRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		log.Printf("Error decoding LLM input request: %v", err)
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	log.Printf("Received message: %s", request.Text)
	log.Printf("Received sessionID: %s", request.SessionID)

	websocket.BroadcastMessage("llmInputReceived", map[string]interface{}{
		"receivedText": request.Text,
	})

	sessionID := request.SessionID
	if sessionID == "" {
		sessionID = "default"
		log.Println("No sessionID provided, using default")
	}

	newTask := task.CreateTask(request.Text)
	newTask.Status = "in-the-queue"

	log.Printf("Created task %s with message: %s", newTask.ID, request.Text)

	websocket.SendTaskUpdate(newTask.ID, newTask.Status, newTask.Message)
	task.EnqueueTask(newTask)

	response := Response{
		Result:    "Task queued successfully",
		TaskID:    newTask.ID,
		SessionID: sessionID,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func TaskCancelHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	taskID := r.URL.Query().Get("taskId")
	if taskID == "" {
		http.Error(w, "taskId parameter is required", http.StatusBadRequest)
		return
	}

	success := task.CancelTask(taskID)

	if success {
		websocket.SendTaskUpdate(taskID, "canceled", "Task canceled by user")
	}

	var response Response
	if success {
		response = Response{
			Result: "Task canceled successfully",
			TaskID: taskID,
		}
	} else {
		response = Response{
			Result: "Task not found or already completed/canceled",
			TaskID: taskID,
		}
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func UserAssistHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var request UserAssistRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		log.Printf("Error decoding user-assist request: %v", err)
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	if request.TaskID == "" || request.Message == "" {
		http.Error(w, "taskId and message are required", http.StatusBadRequest)
		return
	}

	log.Printf("Received user-assist message for task %s: %s", request.TaskID, request.Message)

	accepted := task.AddUserAssistMessage(request.TaskID, request.Message)

	response := Response{
		Result: "User-assist message processed",
		Data: map[string]interface{}{
			"taskId":   request.TaskID,
			"accepted": accepted,
		},
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func ExecutionStateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	state := task.GetExecutionState()

	jsonBytes, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func PingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func abs(x, y int) int {
	if x < y {
		return y - x
	}
	return x - y
}

func drawText(img interface{}, offsetX, offsetY int, text []string) (interface{}, error) {
	log.Printf("Drawing text at (%d, %d): %v", offsetX, offsetY, text)
	return img, nil
}
