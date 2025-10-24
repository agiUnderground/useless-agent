package llm

import (
	"context"
	"fmt"
	"net/http"

	deepseek "github.com/trustsight-io/deepseek-go"
)

// DeepSeekProvider implements the Provider interface for DeepSeek
type DeepSeekProvider struct{}

// NewDeepSeekProvider creates a new DeepSeek provider
func NewDeepSeekProvider() *DeepSeekProvider {
	return &DeepSeekProvider{}
}

// Name returns the provider name
func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

// CreateClient creates a new DeepSeek client with the given configuration
func (p *DeepSeekProvider) CreateClient(config ProviderConfig) (Client, error) {
	// Convert HTTPClient interface to concrete type if provided
	var httpClient *http.Client
	if config.HTTPClient != nil {
		if hc, ok := config.HTTPClient.(*http.Client); ok {
			httpClient = hc
		}
	}

	// Set default timeout if not provided
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	client, err := deepseek.NewClient(
		config.APIKey,
		deepseek.WithBaseURL(config.BaseURL),
		deepseek.WithHTTPClient(httpClient),
		deepseek.WithMaxRetries(config.MaxRetries),
		deepseek.WithMaxRequestSize(config.MaxSize),
		deepseek.WithDebug(config.Debug),
	)
	if err != nil {
		return nil, err
	}

	return &DeepSeekClient{client: client}, nil
}

// DeepSeekClient implements the Client interface for DeepSeek
type DeepSeekClient struct {
	client *deepseek.Client
}

// Close closes the DeepSeek client
func (c *DeepSeekClient) Close() error {
	return c.client.Close()
}

// CreateChatCompletion creates a non-streaming chat completion
func (c *DeepSeekClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Convert our request to DeepSeek request
	dsReq := &deepseek.ChatCompletionRequest{
		Model:           req.Model,
		Temperature:     req.Temperature,
		PresencePenalty: req.PresencePenalty,
		MaxTokens:       req.MaxTokens,
		Messages:        convertMessagesToDeepSeek(req.Messages),
		Stream:          req.Stream,
		JSONMode:        req.JSONMode,
	}

	resp, err := c.client.CreateChatCompletion(ctx, dsReq)
	if err != nil {
		return nil, err
	}

	// Convert DeepSeek response to our response
	result := &ChatCompletionResponse{}
	for _, choice := range resp.Choices {
		result.Choices = append(result.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			Message: struct {
				Content string `json:"content"`
			}{
				Content: choice.Message.Content,
			},
		})
	}

	return result, nil
}

// CreateChatCompletionStream creates a streaming chat completion
func (c *DeepSeekClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (ChatCompletionStream, error) {
	// Convert our request to DeepSeek request
	dsReq := &deepseek.ChatCompletionRequest{
		Model:           req.Model,
		Temperature:     req.Temperature,
		PresencePenalty: req.PresencePenalty,
		MaxTokens:       req.MaxTokens,
		Messages:        convertMessagesToDeepSeek(req.Messages),
		Stream:          req.Stream,
		JSONMode:        req.JSONMode,
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, dsReq)
	if err != nil {
		return nil, err
	}

	return &DeepSeekStream{stream: stream}, nil
}

// EstimateTokensFromMessages estimates the number of tokens in the messages
func (c *DeepSeekClient) EstimateTokensFromMessages(messages []Message) *TokenEstimate {
	dsMessages := convertMessagesToDeepSeek(messages)
	estimate := c.client.EstimateTokensFromMessages(dsMessages)
	return &TokenEstimate{
		EstimatedTokens: estimate.EstimatedTokens,
	}
}

// DeepSeekStream implements the ChatCompletionStream interface for DeepSeek
type DeepSeekStream struct {
	stream interface{} // Will be set to deepseek.ChatCompletionStream
}

// Recv receives the next response from the stream
func (s *DeepSeekStream) Recv() (*ChatCompletionStreamResponse, error) {
	// Type assert to get the actual stream type
	if streamer, ok := s.stream.(interface {
		Recv() (*deepseek.StreamResponse, error)
	}); ok {
		resp, err := streamer.Recv()
		if err != nil {
			return nil, err
		}

		// Convert DeepSeek response to our response
		result := &ChatCompletionStreamResponse{}
		for _, choice := range resp.Choices {
			result.Choices = append(result.Choices, struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			}{
				Delta: struct {
					Content string `json:"content"`
				}{
					Content: choice.Delta.Content,
				},
			})
		}

		return result, nil
	}

	return nil, fmt.Errorf("stream does not support Recv method")
}

// Close closes the stream
func (s *DeepSeekStream) Close() error {
	// Type assert to get the actual stream type
	if streamer, ok := s.stream.(interface{ Close() error }); ok {
		return streamer.Close()
	}

	return fmt.Errorf("stream does not support Close method")
}

// Helper function to convert our messages to DeepSeek messages
func convertMessagesToDeepSeek(messages []Message) []deepseek.Message {
	dsMessages := make([]deepseek.Message, len(messages))
	for i, msg := range messages {
		role := deepseek.RoleUser // default
		switch msg.Role {
		case RoleSystem:
			role = deepseek.RoleSystem
		case RoleUser:
			role = deepseek.RoleUser
		case RoleAssistant:
			role = deepseek.RoleAssistant
		}

		dsMessages[i] = deepseek.Message{
			Role:    role,
			Content: msg.Content,
		}
	}
	return dsMessages
}
