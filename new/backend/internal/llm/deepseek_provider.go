package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type DeepSeekProvider struct{}

func NewDeepSeekProvider() *DeepSeekProvider {
	return &DeepSeekProvider{}
}

func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

func (p *DeepSeekProvider) CreateClient(config ProviderConfig) (Client, error) {
	return &DeepSeekClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

type DeepSeekClient struct {
	config ProviderConfig
	client *http.Client
}

func (c *DeepSeekClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (Stream, error) {
	messages := make([]map[string]interface{}, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	requestBody := map[string]interface{}{
		"model":            req.Model,
		"temperature":      req.Temperature,
		"presence_penalty": req.PresencePenalty,
		"max_tokens":       req.MaxTokens,
		"messages":         messages,
		"stream":           req.Stream,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	return &DeepSeekStream{
		resp:   resp,
		reader: resp.Body,
	}, nil
}

func (c *DeepSeekClient) EstimateTokensFromMessages(messages []Message) TokenEstimate {
	totalChars := 0
	for _, msg := range messages {
		totalChars += len(msg.Content)
	}

	estimatedTokens := totalChars / 4
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	return TokenEstimate{
		EstimatedTokens: estimatedTokens,
	}
}

func (c *DeepSeekClient) Close() error {
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	return nil
}

func (c *DeepSeekClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Not implemented for this provider
	return nil, fmt.Errorf("non-streaming completion not implemented for DeepSeek provider")
}

type DeepSeekStream struct {
	resp   *http.Response
	reader io.ReadCloser
}

func (s *DeepSeekStream) Recv() (*ChatCompletionStreamResponse, error) {
	if s.reader == nil {
		return nil, fmt.Errorf("stream reader is nil")
	}

	buf := make([]byte, 1024)
	n, err := s.reader.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}

	line := string(buf[:n])
	if line == "" {
		return nil, nil
	}

	if line == "data: [DONE]" {
		return nil, io.EOF
	}

	if !bytes.HasPrefix(buf, []byte("data: ")) {
		return nil, nil
	}

	jsonData := bytes.TrimPrefix(buf[:n], []byte("data: "))
	var streamResp struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(jsonData, &streamResp); err != nil {
		return nil, err
	}

	if len(streamResp.Choices) == 0 {
		return nil, nil
	}

	return &ChatCompletionStreamResponse{
		Choices: []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		}{
			{
				Delta: struct {
					Content string `json:"content"`
				}{
					Content: streamResp.Choices[0].Delta.Content,
				},
			},
		},
	}, nil
}

func (s *DeepSeekStream) Close() error {
	if s.resp != nil {
		s.resp.Body.Close()
	}
	return nil
}
