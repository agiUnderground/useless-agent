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

type ZAIProvider struct{}

func NewZAIProvider() *ZAIProvider {
	return &ZAIProvider{}
}

func (p *ZAIProvider) Name() string {
	return "zai"
}

func (p *ZAIProvider) CreateClient(config ProviderConfig) (Client, error) {
	var httpClient *http.Client
	if config.HTTPClient != nil {
		if hc, ok := config.HTTPClient.(*http.Client); ok {
			httpClient = hc
		}
	}

	if httpClient == nil {
		// Set a shorter timeout for streaming to prevent hangs
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

	// Configure the transport with proper timeouts
	if httpTransport, ok := originalTransport.(*http.Transport); ok {
		httpTransport.ResponseHeaderTimeout = 30 * time.Second
		httpTransport.ExpectContinueTimeout = 1 * time.Second
		httpTransport.IdleConnTimeout = 90 * time.Second
		originalTransport = httpTransport
	}

	// Use clean transport without thinking modifications
	httpClient.Transport = originalTransport

	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHTTPClient(httpClient),
	}

	client := openai.NewClient(opts...)

	return &ZAIClient{client: client, config: config}, nil
}

type ZAIClient struct {
	client openai.Client
	config ProviderConfig
}

func (c *ZAIClient) Close() error {
	return nil
}

func (c *ZAIClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	if req.PresencePenalty != 0 {
		oaiReq.PresencePenalty = openai.Float(req.PresencePenalty)
	}

	resp, err := c.client.Chat.Completions.New(ctx, oaiReq)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, fmt.Errorf("received nil response from z.ai API")
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

func (c *ZAIClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (ChatCompletionStream, error) {
	if req.JSONMode && len(req.Messages) > 0 && req.Messages[0].Role == RoleSystem {
		req.Messages[0].Content += " You must respond with valid JSON only. No markdown, no explanations, just clean JSON."
	}

	oaiReq := openai.ChatCompletionNewParams{
		Model:       req.Model,
		Messages:    convertMessagesToOpenAI(req.Messages),
		Temperature: openai.Float(req.Temperature),
		MaxTokens:   openai.Int(int64(req.MaxTokens)),
	}

	if req.PresencePenalty != 0 {
		oaiReq.PresencePenalty = openai.Float(req.PresencePenalty)
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, oaiReq)

	if stream == nil {
		return nil, errors.New("failed to create streaming chat completion: stream is nil")
	}

	return &ZAIStream{stream: stream}, nil
}

func (c *ZAIClient) EstimateTokensFromMessages(messages []Message) *TokenEstimate {
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

type ZAIStream struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

func (s *ZAIStream) Recv() (*ChatCompletionStreamResponse, error) {
	if s.stream == nil {
		return nil, errors.New("stream is nil")
	}

	if !s.stream.Next() {
		err := s.stream.Err()
		if err == nil {
			// Stream completed normally
			return nil, io.EOF
		}
		return nil, err
	}

	resp := s.stream.Current()

	result := &ChatCompletionStreamResponse{}

	// Check for [DONE] marker or empty choices which indicates stream termination
	if len(resp.Choices) == 0 {
		return nil, io.EOF
	}

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

func (s *ZAIStream) Close() error {
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

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
			oaiMessages[i] = openai.UserMessage(msg.Content)
		}
	}
	return oaiMessages
}
