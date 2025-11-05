package server

import (
	"log"
	"net/http"
	"strconv"

	"useless-agent/internal/config"
	"useless-agent/internal/websocket"
)

type Server struct {
	cfg   *config.Config
	wsHub *websocket.Hub
}

func New(cfg *config.Config, wsHub *websocket.Hub) *Server {
	return &Server{
		cfg:   cfg,
		wsHub: wsHub,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", s.wsHub.HandleWebSocket)
	mux.HandleFunc("/screenshot", ScreenshotHandler)
	mux.HandleFunc("/video2", Video2Handler)
	mux.HandleFunc("/llm-input", LLMInputHandler)
	mux.HandleFunc("/task-cancel", TaskCancelHandler)
	mux.HandleFunc("/user-assist", UserAssistHandler)
	mux.HandleFunc("/execution-state", ExecutionStateHandler)
	mux.HandleFunc("/ping", PingHandler)

	bindAddr := ":" + strconv.Itoa(*s.cfg.BindPORT)
	log.Printf("Server starting on %s", bindAddr)

	go s.wsHub.Run()

	return http.ListenAndServe(bindAddr, CORSMiddleware(mux))
}
