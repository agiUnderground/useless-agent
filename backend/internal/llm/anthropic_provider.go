package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// AnthropicProvider implements the Provider interface for Anthropic Claude
type AnthropicProvider struct{}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider() *AnthropicProvider {
	return &AnthropicProvider{}
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// CreateClient creates a new Anthropic client using the OpenAI-compatible endpoint
func (p *AnthropicProvider) CreateClient(config ProviderConfig) (Client, error) {
	var httpClient *http.Client
	if config.HTTPClient != nil {
		if hc, ok := config.HTTPClient.(*http.Client); ok {
			httpClient = hc
		}
	}

	if httpClient == nil {
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	originalTransport := httpClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}

	if httpTransport, ok := originalTransport.(*http.Transport); ok {
		httpTransport.ResponseHeaderTimeout = 30 * time.Second
		httpTransport.ExpectContinueTimeout = 1 * time.Second
		httpTransport.IdleConnTimeout = 90 * time.Second
		originalTransport = httpTransport
	}

	httpClient.Transport = originalTransport

	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHTTPClient(httpClient),
	}

	client := openai.NewClient(opts...)

	return &AnthropicClient{client: client, config: config}, nil
}

// AnthropicClient implements the Client interface for Anthropic Claude
type AnthropicClient struct {
	client openai.Client
	config ProviderConfig
}

// Close closes the Anthropic client
func (c *AnthropicClient) Close() error {
	return nil
}

// CreateChatCompletion creates a non-streaming chat completion
func (c *AnthropicClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	resp, err := c.client.Chat.Completions.New(ctx, oaiReq)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, fmt.Errorf("received nil response from Anthropic API")
	}

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
func (c *AnthropicClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (ChatCompletionStream, error) {
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, oaiReq)
	if stream == nil {
		return nil, errors.New("failed to create streaming chat completion: stream is nil")
	}

	return &AnthropicStream{stream: stream}, nil
}

// EstimateTokensFromMessages estimates the number of tokens in the messages
func (c *AnthropicClient) EstimateTokensFromMessages(messages []Message) *TokenEstimate {
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

// AnthropicStream implements the ChatCompletionStream interface for Anthropic Claude
type AnthropicStream struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

// Recv receives the next response from the stream
func (s *AnthropicStream) Recv() (*ChatCompletionStreamResponse, error) {
	if s.stream == nil {
		return nil, errors.New("stream is nil")
	}

	if !s.stream.Next() {
		err := s.stream.Err()
		if err == nil {
			return nil, io.EOF
		}
		return nil, err
	}

	resp := s.stream.Current()
	result := &ChatCompletionStreamResponse{}

	if len(resp.Choices) == 0 {
		return nil, io.EOF
	}

	for _, choice := range resp.Choices {
		if choice.FinishReason != "" {
			return result, io.EOF
		}

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

// Close closes the stream
func (s *AnthropicStream) Close() error {
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
