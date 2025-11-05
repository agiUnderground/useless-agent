package token

import (
	"fmt"
	"log"
	"sync"
	"time"

	"useless-agent/internal/websocket"
)

type TokenTracker struct {
	totalTokens int64
	mu          sync.RWMutex
	startTime   time.Time
}

var (
	tracker *TokenTracker
	once    sync.Once
)

func GetTracker() *TokenTracker {
	once.Do(func() {
		tracker = &TokenTracker{
			startTime: time.Now(),
		}
	})
	return tracker
}

func AddTokens(count int) {
	t := GetTracker()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalTokens += int64(count)
}

func AddTokensAndSendUpdate(count int) {
	AddTokens(count)
	SendTokenUpdate()
}

func GetTotalTokens() int64 {
	t := GetTracker()
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.totalTokens
}

func Reset() {
	t := GetTracker()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalTokens = 0
	t.startTime = time.Now()
}

func GetTokensPerSecond() float64 {
	t := GetTracker()
	t.mu.RLock()
	defer t.mu.RUnlock()

	elapsed := time.Since(t.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(t.totalTokens) / elapsed
}

func SendTokenUpdate() {
	total := GetTotalTokens()
	websocket.SendTokenUpdate(int(total))
}

func LogTokenUsage(message string) {
	t := GetTracker()
	t.mu.RLock()
	defer t.mu.RUnlock()

	log.Printf("[TOKENS] %s - Total: %d, Rate: %.2f tokens/sec",
		message, t.totalTokens, GetTokensPerSecond())
}

func GetStats() TokenStats {
	t := GetTracker()
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TokenStats{
		TotalTokens:    t.totalTokens,
		StartTime:      t.startTime,
		ElapsedSeconds: time.Since(t.startTime).Seconds(),
		TokensPerSec:   GetTokensPerSecond(),
	}
}

type TokenStats struct {
	TotalTokens    int64     `json:"totalTokens"`
	StartTime      time.Time `json:"startTime"`
	ElapsedSeconds float64   `json:"elapsedSeconds"`
	TokensPerSec   float64   `json:"tokensPerSec"`
}

func (ts TokenStats) String() string {
	return "Token Stats:\n" +
		fmt.Sprintf("  Total Tokens: %d\n", ts.TotalTokens) +
		fmt.Sprintf("  Start Time: %s\n", ts.StartTime.Format(time.RFC3339)) +
		fmt.Sprintf("  Elapsed: %.2f seconds\n", ts.ElapsedSeconds) +
		fmt.Sprintf("  Rate: %.2f tokens/sec\n", ts.TokensPerSec)
}
