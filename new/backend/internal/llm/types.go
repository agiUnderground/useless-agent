package llm

import (
	"context"
	"time"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// CreateClient creates a new LLM client with the given configuration
	CreateClient(config ProviderConfig) (Client, error)
}

// Client defines the interface for LLM clients
type Client interface {
	// Close closes the client and cleans up resources
	Close() error

	// CreateChatCompletion creates a non-streaming chat completion
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// CreateChatCompletionStream creates a streaming chat completion
	CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (Stream, error)

	// EstimateTokensFromMessages estimates the number of tokens in the messages
	EstimateTokensFromMessages(messages []Message) TokenEstimate
}

// Stream defines the interface for streaming chat completions
type Stream interface {
	// Recv receives the next response from the stream
	Recv() (*ChatCompletionStreamResponse, error)

	// Close closes the stream
	Close() error
}

// ProviderConfig holds the configuration for LLM providers
type ProviderConfig struct {
	APIKey     string
	BaseURL    string
	ModelID    string
	Timeout    time.Duration
	MaxRetries int
	MaxSize    int64
	Debug      bool
	HTTPClient interface{} // Allow provider-specific HTTP client configuration
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model           string    `json:"model"`
	Temperature     float64   `json:"temperature,omitempty"`
	PresencePenalty float64   `json:"presence_penalty,omitempty"`
	MaxTokens       int       `json:"max_tokens,omitempty"`
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream,omitempty"`
	JSONMode        bool      `json:"json_mode,omitempty"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ChatCompletionStreamResponse represents a streaming chat completion response
type ChatCompletionStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// TokenEstimate represents a token estimation result
type TokenEstimate struct {
	EstimatedTokens int `json:"estimated_tokens"`
}

// Message roles
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)
