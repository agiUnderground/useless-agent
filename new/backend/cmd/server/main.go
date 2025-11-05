package main

import (
	"log"
	"os"
	"strings"
	"sync"

	"useless-agent/internal/config"
	"useless-agent/internal/llm"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/server"
	"useless-agent/internal/websocket"
)

func main() {
	if err := screenshot.SuppressXGBLogs(); err != nil {
		log.Fatalf("Failed to suppress xgb logs: %v", err)
	}

	cfg := config.New()

	if err := llm.Initialize(); err != nil {
		log.Fatalf("Failed to initialize LLM: %v", err)
	}

	wsHub := websocket.NewHub()

	srv := server.New(cfg, wsHub)

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type LogWriter struct {
	original *os.File
	mu       sync.Mutex
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	n, err = lw.original.Write(p)
	if err != nil {
		return n, err
	}

	logData := string(p)
	logData = strings.TrimSpace(logData)

	if logData == "" || strings.HasPrefix(logData, "===") && len(logData) <= 10 {
		return n, nil
	}

	if strings.Contains(logData, "ASK ===") {
		return n, nil
	}

	go websocket.SendLogMessage(logData)
	return n, nil
}

func setupLogStreaming() error {
	logWriter := &LogWriter{
		original: os.Stdout,
	}

	log.SetOutput(logWriter)
	log.SetFlags(log.LstdFlags)
	return nil
}
