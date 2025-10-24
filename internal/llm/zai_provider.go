package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// thinkingTransport is a custom HTTP transport that adds thinking mode parameter
type thinkingTransport struct {
	transport http.RoundTripper
}

// RoundTrip intercepts HTTP requests and adds thinking mode parameter
func (t *thinkingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only modify chat completion requests
	if req.URL.Path == "/chat/completions" && req.Method == "POST" {
		// Read the original body
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			req.Body.Close()

			// Parse the JSON body
			var requestBody map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
				// If we can't parse, just restore the original body
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				return t.transport.RoundTrip(req)
			}

			// Add thinking mode parameter to disable thinking
			requestBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}

			// Marshal the modified body
			modifiedBody, err := json.Marshal(requestBody)
			if err != nil {
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				return t.transport.RoundTrip(req)
			}

			// Create new request with modified body
			req.Body = io.NopCloser(bytes.NewBuffer(modifiedBody))
			req.ContentLength = int64(len(modifiedBody))
		}
	}

	return t.transport.RoundTrip(req)
}

// ZAIProvider implements the Provider interface for z.ai
type ZAIProvider struct{}

// NewZAIProvider creates a new z.ai provider
func NewZAIProvider() *ZAIProvider {
	return &ZAIProvider{}
}

// Name returns the provider name
func (p *ZAIProvider) Name() string {
	return "zai"
}

// CreateClient creates a new z.ai client with the given configuration
func (p *ZAIProvider) CreateClient(config ProviderConfig) (Client, error) {
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

	// Wrap the transport with our custom thinking transport to disable thinking mode
	originalTransport := httpClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	httpClient.Transport = &thinkingTransport{transport: originalTransport}

	// Create OpenAI client options
	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHTTPClient(httpClient),
	}

	client := openai.NewClient(opts...)

	return &ZAIClient{client: client, config: config}, nil
}

// ZAIClient implements the Client interface for z.ai
type ZAIClient struct {
	client openai.Client
	config ProviderConfig
}

// Close closes the z.ai client (OpenAI client doesn't need explicit closing)
func (c *ZAIClient) Close() error {
	return nil // OpenAI client doesn't require explicit closing
}

// CreateChatCompletion creates a non-streaming chat completion
func (c *ZAIClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Convert our request to OpenAI request
	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	// Add presence penalty if specified
	if req.PresencePenalty != 0 {
		oaiReq.PresencePenalty = openai.Float(req.PresencePenalty)
	}

	// Note: Thinking mode is now disabled by default via the custom HTTP transport
	// The thinkingTransport automatically adds {"thinking": {"type": "disabled"}} to all requests

	// Note: JSON mode for z.ai - we rely on system message instructions for now
	// The OpenAI client JSON mode may not be compatible with z.ai API
	// If JSON mode is requested, we enhance the system message
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	resp, err := c.client.Chat.Completions.New(ctx, oaiReq)
	if err != nil {
		return nil, err
	}

	// Debug: log the raw response
	if resp == nil {
		return nil, fmt.Errorf("received nil response from z.ai API")
	}

	// Debug: log response details
	fmt.Printf("ZAI Response - Choices count: %d\n", len(resp.Choices))
	if len(resp.Choices) == 0 {
		return &ChatCompletionResponse{Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{}}, nil
	}

	// Convert OpenAI response to our response
	result := &ChatCompletionResponse{}
	for _, choice := range resp.Choices {
		// Check if choice.Message.Content is empty or nil
		if choice.Message.Content == "" {
			fmt.Printf("Warning: Empty content in choice, skipping...\n")
			continue
		}

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

	// If no valid choices were found, return an empty response instead of nil
	if len(result.Choices) == 0 {
		fmt.Printf("Warning: No valid choices found in z.ai response, returning empty response\n")
		result.Choices = []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{{
			Message: struct {
				Content string `json:"content"`
			}{
				Content: "",
			},
		}}
	}

	return result, nil
}

// CreateChatCompletionStream creates a streaming chat completion
func (c *ZAIClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (ChatCompletionStream, error) {
	// Convert our request to OpenAI request
	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	// Add presence penalty if specified
	if req.PresencePenalty != 0 {
		oaiReq.PresencePenalty = openai.Float(req.PresencePenalty)
	}

	// Note: JSON mode for z.ai - we rely on system message instructions for now
	// The OpenAI client JSON mode may not be compatible with z.ai API
	// If JSON mode is requested, we enhance the system message
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	// Note: Thinking mode is now disabled by default via the custom HTTP transport
	// The thinkingTransport automatically adds {"thinking": {"type": "disabled"}} to all requests

	stream := c.client.Chat.Completions.NewStreaming(ctx, oaiReq)

	// Check if the stream is nil to prevent panic
	if stream == nil {
		return nil, errors.New("failed to create streaming chat completion: stream is nil")
	}

	return &ZAIStream{stream: stream}, nil
}

// EstimateTokensFromMessages estimates the number of tokens in the messages
// Note: OpenAI Go client doesn't provide token estimation, so we'll use a simple approximation
func (c *ZAIClient) EstimateTokensFromMessages(messages []Message) *TokenEstimate {
	// Simple token estimation: roughly 4 characters per token
	totalChars := 0
	for _, msg := range messages {
		totalChars += len(msg.Content)
	}

	estimatedTokens := totalChars / 4
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	return &TokenEstimate{
		EstimatedTokens: estimatedTokens,
	}
}

// ZAIStream implements the ChatCompletionStream interface for z.ai
type ZAIStream struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

// Recv receives the next response from the stream
func (s *ZAIStream) Recv() (*ChatCompletionStreamResponse, error) {
	// Check if stream is nil to prevent panic
	if s.stream == nil {
		return nil, errors.New("stream is nil")
	}

	// Keep trying to get a valid response
	for {
		if !s.stream.Next() {
			// Stream ended, return nil with error (which might be io.EOF)
			return nil, s.stream.Err()
		}

		resp := s.stream.Current()

		// Convert OpenAI response to our response
		result := &ChatCompletionStreamResponse{}

		// Check if resp.Choices is not empty to prevent panic
		if len(resp.Choices) > 0 {
			for _, choice := range resp.Choices {
				// Additional safety checks to prevent panic
				// Check if the choice has valid Delta content
				if choice.Delta.Content != "" {
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
			}
		}

		// If we found valid content, return it
		if len(result.Choices) > 0 {
			return result, nil
		}

		// If this is an empty chunk but we have choices with finish reasons,
		// this might be end of stream
		if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != "" {
			return result, nil
		}

		// Otherwise, this is an empty chunk, continue to the next one
		// This prevents returning empty responses that cause infinite loops
	}
}

// Close closes the stream
func (s *ZAIStream) Close() error {
	if s.stream == nil {
		return nil // Already closed or never initialized
	}
	return s.stream.Close()
}

// Helper function to convert our messages to OpenAI messages
func convertMessagesToOpenAI(messages []Message) []openai.ChatCompletionMessageParamUnion {
	oaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			oaiMessages[i] = openai.SystemMessage(msg.Content)
		case RoleUser:
			oaiMessages[i] = openai.UserMessage(msg.Content)
		case RoleAssistant:
			oaiMessages[i] = openai.AssistantMessage(msg.Content)
		default:
			// Default to user message for unknown roles
			oaiMessages[i] = openai.UserMessage(msg.Content)
		}
	}
	return oaiMessages
}
