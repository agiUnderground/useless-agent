package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdimage "image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"useless-agent/internal/config"
	"useless-agent/internal/image"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/storage"
	"useless-agent/internal/task"
	"useless-agent/internal/websocket"
	"useless-agent/pkg/x11"
)

// ScreenshotHandler handles screenshot requests
func ScreenshotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	img, err := screenshot.CaptureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Encode the image as PNG
	pngBytes, err := screenshot.EncodeToPNG(img)
	if err != nil {
		http.Error(w, "Failed to encode image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the PNG image to the response
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)
}

// LLMInputHandler handles LLM input requests
func LLMInputHandler(w http.ResponseWriter, r *http.Request) {
	type PostMessage struct {
		Text          string `json:"text"`
		SessionID     string `json:"sessionId,omitempty"`
		Model         string `json:"model,omitempty"`
		Provider      string `json:"provider,omitempty"`
		ThinkingLevel string `json:"thinkingLevel,omitempty"`
	}
	type ConfirmationMessage struct {
		ReceivedText string `json:"Received llm input text,omitempty"`
	}

	var receivedMessage PostMessage
	var acknowledgment ConfirmationMessage

	err := json.NewDecoder(r.Body).Decode(&receivedMessage)
	if err != nil {
		fmt.Println(err)
	}

	log.Println("Received message:", receivedMessage.Text)
	log.Println("Received sessionID:", receivedMessage.SessionID)
	log.Println("Received model:", receivedMessage.Model, "provider:", receivedMessage.Provider)

	// Resolve provider and model
	providerName := receivedMessage.Provider
	modelName := receivedMessage.Model
	if providerName == "" && modelName != "" {
		if resolved, ok := config.ResolveProviderForModel(modelName); ok {
			providerName = resolved
		}
	}
	if providerName == "" && modelName == "" {
		providerName, modelName = config.GetDefaultProviderAndModel()
	} else if providerName == "" {
		providerName, _ = config.GetDefaultProviderAndModel()
	}

	// Verify credentials exist for the chosen provider
	if pcfg, ok := config.GetProviderConfig(providerName); !ok || pcfg.APIKey == "" {
		http.Error(w, fmt.Sprintf("No API credentials configured for provider %q (model %q)", providerName, modelName), http.StatusBadRequest)
		return
	}

	acknowledgment.ReceivedText = receivedMessage.Text

	// Send acknowledgment via WebSocket
	websocket.BroadcastMessage("llmInputReceived", acknowledgment)

	// Use provided session ID or fall back to "default" if not provided
	sessionID := receivedMessage.SessionID
	if sessionID == "" {
		sessionID = "default"
		log.Println("No sessionID provided, using default")
	}

	// Ensure persistent session log is active for this session
	clientIP := r.RemoteAddr
	if host, _, splitErr := net.SplitHostPort(r.RemoteAddr); splitErr == nil {
		clientIP = host
	}
	storage.StartSessionLog(clientIP, sessionID)

	// Resolve thinking level (default to lowest supported level for thinking models)
	thinkingLevel := receivedMessage.ThinkingLevel
	if levels := config.ThinkingLevelsForModel(modelName); len(levels) > 0 {
		if thinkingLevel == "" {
			thinkingLevel = levels[0]
		}
	} else {
		thinkingLevel = ""
	}

	// Create a new task for this request (starts in "in-the-queue" and emits initial update)
	newTask := task.CreateTask(receivedMessage.Text, providerName, modelName, thinkingLevel, sessionID)

	log.Printf("Created task %s with message: %s (provider: %s, model: %s)", newTask.ID, receivedMessage.Text, providerName, modelName)

	// Enqueue the task (transitions to "in-progress" immediately if no task is running)
	task.EnqueueTask(newTask)

	// Return immediate JSON response
	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result":        "Task queued successfully",
		"taskId":        newTask.ID,
		"sessionId":     sessionID,
		"model":         newTask.Model,
		"provider":      newTask.Provider,
		"thinkingLevel": newTask.ThinkingLevel,
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// ModelsHandler returns all available models and whether their providers have credentials configured.
func ModelsHandler(w http.ResponseWriter, r *http.Request) {
	defaultProvider, defaultModel := config.GetDefaultProviderAndModel()
	resp := map[string]interface{}{
		"defaultProvider": defaultProvider,
		"defaultModel":    defaultModel,
		"models":          config.GetAvailableModels(),
	}
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// ArtifactsHandler lists or uploads persistent task artifacts.
func ArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		taskID := r.URL.Query().Get("taskId")
		artifacts, err := storage.ListArtifacts(taskID)
		if err != nil {
			http.Error(w, "Failed to list artifacts: "+err.Error(), http.StatusInternalServerError)
			return
		}
		jsonBytes, err := json.Marshal(map[string]interface{}{
			"artifactsDir": config.GetArtifactsDir(),
			"artifacts":    artifacts,
		})
		if err != nil {
			http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonBytes)

	case http.MethodPost:
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, "Expected multipart form upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing 'file' field: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		taskID := r.FormValue("taskId")
		info, err := storage.SaveArtifact(taskID, header.Filename, file)
		if err != nil {
			http.Error(w, "Failed to save artifact: "+err.Error(), http.StatusInternalServerError)
			return
		}
		jsonBytes, _ := json.Marshal(info)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(jsonBytes)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ArtifactDownloadHandler serves a saved artifact file by relative path under /artifacts/.
func ArtifactDownloadHandler(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	if relPath == "" {
		ArtifactsHandler(w, r)
		return
	}
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		http.Error(w, "Invalid artifact path", http.StatusBadRequest)
		return
	}
	fullPath := filepath.Join(config.GetArtifactsDir(), cleanRel)
	http.ServeFile(w, r, fullPath)
}

// TaskCancelHandler handles task cancellation requests
func TaskCancelHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskId")
	if taskID == "" {
		http.Error(w, "taskId parameter is required", http.StatusBadRequest)
		return
	}

	success := task.CancelTask(taskID)

	// Send WebSocket status update for canceled tasks
	if success {
		modelName := ""
		thinkingLevel := ""
		if t, ok := task.GetTask(taskID); ok {
			modelName = t.Model
			thinkingLevel = t.ThinkingLevel
		}
		websocket.SendTaskUpdate(taskID, "canceled", "Task canceled by user", modelName, thinkingLevel)
	}

	var response []map[string]interface{}
	if success {
		response = append(response, map[string]interface{}{
			"result": "Task canceled successfully",
			"taskId": taskID,
		})
	} else {
		response = append(response, map[string]interface{}{
			"result": "Task not found or already completed/canceled",
			"taskId": taskID,
		})
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// UserAssistHandler handles user-assist message requests
func UserAssistHandler(w http.ResponseWriter, r *http.Request) {
	type UserAssistRequest struct {
		TaskID  string `json:"taskId"`
		Message string `json:"message"`
	}

	type UserAssistResponse struct {
		Result   string `json:"result"`
		TaskID   string `json:"taskId"`
		Accepted bool   `json:"accepted"`
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

	// Add user-assist message to the task
	accepted := task.AddUserAssistMessage(request.TaskID, request.Message)

	response := UserAssistResponse{
		Result:   "User-assist message processed",
		TaskID:   request.TaskID,
		Accepted: accepted,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// PingHandler handles ping requests
func PingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// CORSMiddleware adds CORS headers to responses
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight (OPTIONS) requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Pass the request to the next handler
		next.ServeHTTP(w, r)
	})
}

// Video2Handler handles video2 requests (bounding box detection)
func Video2Handler(w http.ResponseWriter, r *http.Request) {
	// Capture and prepare initial image
	img, err := screenshot.CaptureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot with BB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to grayscale and RGBA
	grayImg := image.ConvertToGrayscale(img)
	img = grayImg
	rgbaImg := stdimage.NewRGBA(img.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), img, stdimage.Point{0, 0}, draw.Src)

	// Find dominant colors
	dominantColors := image.FindDominantColors(rgbaImg)
	dominantColors = dominantColors[:20]

	// Create drawing image
	drawImg := stdimage.NewRGBA(rgbaImg.Bounds())
	draw.Draw(drawImg, drawImg.Bounds(), rgbaImg, stdimage.Point{0, 0}, draw.Src)

	// Reset bounding box array
	var bbArray []image.BoundingBox
	bbCounter := 1

	// Process each dominant color
	for _, colorElem := range dominantColors {
		// Create masks and find components
		mask := image.CreateMask(rgbaImg, colorElem, 0)
		components := image.FindConnectedComponents(mask)

		mask2 := image.CreateMask(rgbaImg, colorElem, 80) // 90 is really good for small text.
		components2 := image.FindConnectedComponents(mask2)
		newComponents := append(components, components2...)
		components = newComponents

		// Process each component
		for _, component := range components {
			percentage := image.CalculatePercentage(component, mask)
			if percentage >= 80 {
				x1, y1, x2, y2 := image.FindBoundingBox(component)
				absy := abs(y1, y2)
				absx := abs(x1, x2)

				if absy < 10 || absx < 10 {
					continue
				}

				// Draw initial bounding box
				borderColor := color.RGBA{R: 255, G: 0, B: 132, A: 255}
				image.DrawBoundingBox(drawImg, x1, y1, x2, y2, borderColor)

				// Process larger components
				if absy >= 25 && absx >= 25 {
					bbArray = append(bbArray, image.BoundingBox{
						ID: bbCounter,
						X:  x1,
						Y:  y1,
						X2: x2,
						Y2: y2,
					})

					// Draw ID box and number
					borderColor = color.RGBA{R: 12, G: 236, B: 28, A: 255}
					image.DrawBoundingBox(drawImg, x1+1, y1+1, x1+15, y1+15, borderColor)
					_, err = DrawText(drawImg, x1+2, y1+2, []string{strconv.Itoa(bbCounter)})
					if err != nil {
						fmt.Println(err)
					}
					bbCounter++
				}
			}
		}
	}

	// Write output image
	buf := new(bytes.Buffer)
	png.Encode(buf, drawImg)
	w.Header().Set("Content-Type", "image/png")
	w.Write(buf.Bytes())
}

// abs returns the absolute difference between two integers
func abs(x, y int) int {
	if x < y {
		return y - x
	}
	return x - y
}

// DrawText draws text on an image (placeholder)
func DrawText(img interface{}, offsetX, offsetY int, text []string) (interface{}, error) {
	// This is a placeholder implementation
	// The actual implementation would be moved from main.go
	log.Printf("Drawing text at (%d, %d): %v", offsetX, offsetY, text)
	return img, nil
}

// GetX11WindowsData gets X11 windows data
func GetX11WindowsData() (string, error) {
	log.Printf("=== GETTING X11 WINDOWS DATA ===")

	// Get display from config - always pass the display value, even if it's default ":0"
	display := *config.Display
	log.Printf("Using display from config: %s", display)

	x11WindowsJSON, err := x11.GetX11WindowsWithDisplay(display)
	if err != nil {
		log.Printf("Failed to get X11 windows data: %v", err)
		return "[]", err
	}

	// Log the retrieved data for debugging
	log.Printf("X11 windows data retrieved successfully:")
	log.Printf("Raw JSON data: %s", x11WindowsJSON)

	// Parse and log structured data for better readability
	var x11WindowsData x11.X11WindowInfo
	if parseErr := json.Unmarshal([]byte(x11WindowsJSON), &x11WindowsData); parseErr == nil {
		log.Printf("Parsed X11 windows data - Total windows: %d", len(x11WindowsData.Windows))
		for i, window := range x11WindowsData.Windows {
			log.Printf("Window %d: ID=%d, Title='%s', Class='%s', Position=(%d,%d), Size=%dx%d, Visible=%t",
				i+1, window.ID, window.Title, window.Class, window.Position.X, window.Position.Y,
				window.Size.Width, window.Size.Height, window.Visible)
		}
	} else {
		log.Printf("Failed to parse X11 windows JSON for logging: %v", parseErr)
	}

	log.Printf("=== X11 WINDOWS DATA END ===")
	return x11WindowsJSON, nil
}

// ExecutionStateHandler handles requests for current execution engine state
func ExecutionStateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get current execution state
	state := task.GetExecutionState()

	jsonBytes, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}
