package token

import (
	"encoding/json"
	"log"
	"sync"

	"useless-agent/internal/websocket"
)

// Token tracking globals
var (
	totalTokensUsed int
	tokenMutex      sync.Mutex
)

// AddTokensAndSendUpdate adds tokens to the total and sends update to all websocket clients
func AddTokensAndSendUpdate(tokens int) {
	tokenMutex.Lock()
	totalTokensUsed += tokens
	currentTotal := totalTokensUsed
	tokenMutex.Unlock()

	// Send update to all websocket clients (non-blocking)
	go websocket.SendTokenUpdate(currentTotal)
	log.Printf("Token usage updated: %d (added: %d)", currentTotal, tokens)
}

// ResetTokenCounter resets the token counter and sends update
func ResetTokenCounter() {
	tokenMutex.Lock()
	totalTokensUsed = 0
	tokenMutex.Unlock()

	// Send reset update to all websocket clients (non-blocking)
	go websocket.SendTokenUpdate(0)
	log.Printf("Token counter reset")
}

// GetTotalTokens returns the current total tokens used
func GetTotalTokens() int {
	tokenMutex.Lock()
	defer tokenMutex.Unlock()
	return totalTokensUsed
}

// CreateTokenUpdateJSON creates a JSON token update message
func CreateTokenUpdateJSON(total int) ([]byte, error) {
	update := map[string]interface{}{
		"type":  "tokenUpdate",
		"total": total,
	}
	return json.Marshal(update)
}
