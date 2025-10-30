package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"useless-agent/internal/config"
	httpHandlers "useless-agent/internal/http"
	"useless-agent/internal/llm"
	"useless-agent/internal/mouse"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/websocket"
)

func main() {
	flag.Parse()

	if err := screenshot.SuppressXGBLogs(); err != nil {
		log.Fatalf("Failed to suppress xgb logs: %v", err)
	}

	// Initialize LLM system
	if err := llm.InitializeLLM(); err != nil {
		log.Fatalf("Failed to initialize LLM: %v", err)
	}

	// Set up stdout interception for log streaming
	if err := setupLogStreaming(); err != nil {
		log.Printf("Warning: Failed to setup log streaming: %v", err)
	}

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", websocket.WSHandler)
	mux.HandleFunc("/screenshot", httpHandlers.ScreenshotHandler)
	mux.HandleFunc("/mouse-input", mouse.MouseInputHandler)
	mux.HandleFunc("/mouse-click", mouse.MouseClickHandler)
	mux.HandleFunc("/llm-input", httpHandlers.LLMInputHandler)
	mux.HandleFunc("/video2", httpHandlers.Video2Handler)
	mux.HandleFunc("/task-cancel", httpHandlers.TaskCancelHandler)
	mux.HandleFunc("/user-assist", httpHandlers.UserAssistHandler)
	mux.HandleFunc("/execution-state", httpHandlers.ExecutionStateHandler)
	mux.HandleFunc("/ping", httpHandlers.PingHandler)

	bindAddr := net.JoinHostPort(*config.BindIP, strconv.Itoa(*config.BindPORT))
	log.Println("Server running on http://" + bindAddr)

	if err := http.ListenAndServe(bindAddr, httpHandlers.CORSMiddleware(mux)); err != nil {
		log.Fatal("Server error:", err)
	}
}

// setupLogStreaming creates a custom logger that duplicates output to both stdout and WebSocket clients
func setupLogStreaming() error {
	// Create a custom writer that sends to both stdout and WebSocket
	logWriter := &LogWriter{
		original: os.Stdout,
	}

	// Replace the standard logger with our custom one
	log.SetOutput(logWriter)
	log.SetFlags(log.LstdFlags)

	return nil
}

// LogWriter is a custom writer that duplicates writes to both original stdout and WebSocket clients
type LogWriter struct {
	original *os.File
	mu       sync.Mutex
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Write to original stdout
	n, err = lw.original.Write(p)
	if err != nil {
		return n, err
	}

	// Filter out empty or problematic log lines before sending to WebSocket clients
	logData := string(p)

	// Skip empty lines
	if len(strings.TrimSpace(logData)) == 0 {
		return n, nil
	}

	// Skip lines that are just separators (like "====" or "ASK ===")
	trimmed := strings.TrimSpace(logData)
	if strings.HasPrefix(trimmed, "===") && len(trimmed) <= 10 {
		return n, nil
	}

	// Skip lines containing "ASK ===" pattern
	if strings.Contains(logData, "ASK ===") {
		return n, nil
	}

	// Send to WebSocket clients (non-blocking)
	go func() {
		websocket.SendLogMessage(logData)
	}()

	return n, nil
}
