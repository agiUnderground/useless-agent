package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"strconv"

	"useless-agent/internal/config"
	httpHandlers "useless-agent/internal/http"
	"useless-agent/internal/mouse"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/websocket"
)

func main() {
	flag.Parse()

	if err := screenshot.SuppressXGBLogs(); err != nil {
		log.Fatalf("Failed to suppress xgb logs: %v", err)
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
	mux.HandleFunc("/ping", httpHandlers.PingHandler)

	bindAddr := net.JoinHostPort(*config.BindIP, strconv.Itoa(*config.BindPORT))
	log.Println("Server running on http://" + bindAddr)

	if err := http.ListenAndServe(bindAddr, httpHandlers.CORSMiddleware(mux)); err != nil {
		log.Fatal("Server error:", err)
	}
}
