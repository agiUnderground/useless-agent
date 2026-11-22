package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"useless-agent/internal/action"
	"useless-agent/internal/config"
	"useless-agent/internal/mouse"
	"useless-agent/internal/token"
)

var (
	// Global LLM client instance (default) and cache of per-provider/model clients
	llmClient        Client
	clientCache      = make(map[string]Client)
	clientMu         sync.RWMutex
	providerRegistry map[string]Provider
)

// defaultModelForProvider returns the default model ID for the given provider
func defaultModelForProvider(provider string) (string, error) {
	switch provider {
	case "deepseek":
		return "deepseek-flash", nil
	case "zai":
		return "glm-5.3", nil
	case "gemini":
		return "gemini-4-neon", nil
	case "openai":
		return "gpt-6.2-sol", nil
	case "anthropic":
		return "claude-sonnet-5-5", nil
	case "qwen":
		return "qwen4-max", nil
	case "minimax":
		return "MiniMax-M3.1-Flash", nil
	default:
		return "", fmt.Errorf("unknown LLM provider: %s", provider)
	}
}

// resolveModel returns the configured model or the provider's default model
func resolveModel(cfg config.LLMConfig) string {
	if cfg.Model != "" {
		return cfg.Model
	}
	model, _ := defaultModelForProvider(cfg.Provider)
	return model
}

// InitializeLLM initializes the LLM provider registry and optionally the default client.
// Missing credentials are allowed so the server can start with 0, 1, or many configured providers.
func InitializeLLM() error {
	clientMu.Lock()
	defer clientMu.Unlock()

	// Initialize provider registry
	providerRegistry = make(map[string]Provider)

	// Register available providers
	providerRegistry["deepseek"] = NewDeepSeekProvider()
	providerRegistry["zai"] = NewZAIProvider()
	providerRegistry["gemini"] = NewGeminiProvider()
	providerRegistry["openai"] = NewOpenAIProvider()
	providerRegistry["anthropic"] = NewAnthropicProvider()
	providerRegistry["qwen"] = NewQwenProvider()
	providerRegistry["minimax"] = NewMiniMaxProvider()

	// Get default configuration
	cfg := config.GetLLMConfig()
	if cfg.APIKey == "" {
		log.Printf("No default LLM API key configured at startup; providers with credentials in config will be used dynamically")
		return nil
	}

	model := resolveModel(cfg)
	client, err := createClientLocked(cfg.Provider, model)
	if err != nil {
		log.Printf("Warning: failed to initialize default LLM client (%s/%s): %v", cfg.Provider, model, err)
		return nil
	}

	llmClient = client
	return nil
}

func defaultBaseURLForProvider(provider string) string {
	switch provider {
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "zai":
		return "https://api.z.ai/api/paas/v4"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai/"
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1/"
	case "qwen":
		return "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	case "minimax":
		return "https://api.minimax.io/v1"
	default:
		return ""
	}
}

func createClientLocked(providerName, model string) (Client, error) {
	if providerName == "" {
		if resolved, ok := config.ResolveProviderForModel(model); ok {
			providerName = resolved
		} else {
			providerName, _ = config.GetDefaultProviderAndModel()
		}
	}
	if model == "" {
		var err error
		model, err = defaultModelForProvider(providerName)
		if err != nil {
			return nil, err
		}
	}

	cacheKey := providerName + "::" + model
	if c, ok := clientCache[cacheKey]; ok && c != nil {
		return c, nil
	}

	pcfg, ok := config.GetProviderConfig(providerName)
	if !ok || pcfg.APIKey == "" {
		return nil, fmt.Errorf("no API credentials configured for provider %q", providerName)
	}

	baseURL := pcfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURLForProvider(providerName)
	}

	provider, exists := providerRegistry[providerName]
	if !exists {
		return nil, fmt.Errorf("unsupported LLM provider: %s", providerName)
	}

	client, err := provider.CreateClient(ProviderConfig{
		APIKey:     pcfg.APIKey,
		BaseURL:    baseURL,
		ModelID:    model,
		Timeout:    5 * time.Minute,
		MaxRetries: 2,
		MaxSize:    50 << 20, // 50 MB
		Debug:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	clientCache[cacheKey] = client
	return client, nil
}

// GetClientFor returns a client and resolved model for the given provider/model (falling back to defaults).
func GetClientFor(providerName, model string) (Client, string, error) {
	clientMu.Lock()
	defer clientMu.Unlock()

	if providerRegistry == nil {
		return nil, "", fmt.Errorf("LLM provider registry not initialized")
	}

	if providerName == "" && model != "" {
		if resolved, ok := config.ResolveProviderForModel(model); ok {
			providerName = resolved
		}
	}
	if providerName == "" {
		defProv, defModel := config.GetDefaultProviderAndModel()
		providerName = defProv
		if model == "" {
			model = defModel
		}
	}
	if model == "" {
		m, err := defaultModelForProvider(providerName)
		if err != nil {
			return nil, "", err
		}
		model = m
	}

	client, err := createClientLocked(providerName, model)
	if err != nil {
		return nil, "", err
	}
	return client, model, nil
}

// GetLLMClient returns the current default LLM client
func GetLLMClient() Client {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return llmClient
}

// SendMessageToLLM sends a message to the LLM and returns actions to execute
func SendMessageToLLM(ctx context.Context, providerName string, modelName string, thinkingLevel string, artifactsDir string, prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, x11WindowsData string, colorsDistribution string) (actionsToExecute []action.Action, actionsJSONStringReturn string, err error) {
	// Check if context is nil, use background context if it is
	if ctx == nil {
		log.Println("Warning: nil context provided to sendMessageToLLM, using background context")
		ctx = context.Background()
	}

	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("Failed to get LLM client (%s/%s): %v", providerName, modelName, err)
		return []action.Action{}, "", err
	}

	cursorPosition, _ := mouse.GetCursorPositionJSON()
	iterationString := strconv.FormatInt(iteration, 10)

	log.Println("====================================================")
	log.Println("===================LLM INPUT========================")
	log.Println("====================================================")
	log.Println("model:", model)
	log.Println("prompt:", prompt)
	log.Println("cursorPosition:", cursorPosition)
	log.Println("ocrContext:", ocrContext)
	log.Println("ocrDelta:", ocrDelta)
	log.Println("allWindowsJSONString:", allWindowsJSONString)
	log.Println("prevExecutedCommands:", prevExecutedCommands)
	log.Println("iteration:", iterationString)
	log.Println("====================================================")
	log.Println("=================LLM INPUT END=====================")
	log.Println("====================================================")

	artifactsNote := ""
	if artifactsDir != "" {
		artifactsNote = " Persistent artifacts directory for this task is: " + artifactsDir + " (if the task produces or asks to save any files, binaries, screenshots, or outputs, save them there)."
	}

	// Create messages using our generic types
	messages := []Message{
		{
			Role: RoleSystem,
			Content: `Analyze the input data (OCR text, bounding boxes, cursor position, window state, and previously executed actions) and return a valid compact JSON array of actions to advance and complete the task.
- Issue the 'st' (stopIteration) action as soon as the goal is achieved.
- Use keyboard shortcuts/hotkeys when appropriate.
- Check ocrDelta and current screen state on every iteration to verify whether the previous step succeeded. Do not repeat already completed steps (for example, if the applications menu is already open, proceed to launch the app instead of opening the menu again).
- If the screen state already shows evidence that the goal is achieved (such as the target application window or its text being visible), issue 'st'.
- Never issue identical actions consecutively more than 5 times.
- Before interacting with a UI or web element (such as typing into a URL bar), move the cursor to the center of that element using its OCR or bounding box coordinates.` + artifactsNote,
		},
		{
			Role: RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11), 1920x1080 screen. Prefer neovim for writing or editing files and tmux when working with two or more files.

Screen state:
- Bounding boxes: ` + bboxes + `
- OCR results: ` + ocrContext + `
- OCR delta (change from previous iteration): ` + ocrDelta + `
- Top 10 screen colors: ` + colorsDistribution + `
- Previous cursor position: ` + prevCursorPosJSONString + `
- Current cursor position: ` + cursorPosition + `
- OCR-detected windows: ` + allWindowsJSONString + `
- X11-detected windows: ` + x11WindowsData + `
- Iteration: ` + iterationString + `
- Previously executed commands: ` + prevExecutedCommands + `

If previously executed commands contain repeated identical actions, adjust your approach (for example, move the cursor to a slightly different position).

` + action.DefaultRegistry().PromptSpec() + `

Rules:
- Use 'rp' (repeat) when repeating identical prior actions (range must reference earlier sequence IDs).
- Click slightly inside UI elements rather than on their borders.
- Do not produce unnecessary actions.
- On every iteration, analyze ocrDelta; if and only if the goal is achieved, issue 'st' (stopIteration).
- Always return a valid JSON array (even for a single action), with no comments or extra text.

Current task:
` + prompt + `

Analyze the current state and previously executed actions, then produce the next sequence of actions to achieve the goal. If the goal is already achieved, issue the 'st' (stopIteration) action.`,
		},
	}

	// Estimate tokens
	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[main llm input func][input]: %d\n", estimate.EstimatedTokens)
	token.AddTokensAndSendUpdate(estimate.EstimatedTokens)

	fmt.Println("\nCreating streaming chat completion...")

	// Create streaming request
	req := &ChatCompletionRequest{
		Model:           model,
		Temperature:     0.5,
		PresencePenalty: 0.3,
		MaxTokens:       8192,
		Messages:        messages,
		Stream:          true,
		JSONMode:        true,
		ThinkingLevel:   thinkingLevel,
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			log.Printf("LLM request canceled during stream creation")
			return []action.Action{}, "", errors.New("LLM request canceled by user")
		}
		log.Printf("Failed to create LLM stream: %v", err)
		return []action.Action{}, "", errors.New("Failed to send message to LLM, error.")
	}

	// Additional nil check for stream to prevent panic
	if stream == nil {
		log.Printf("LLM stream is nil after creation")
		return []action.Action{}, "", errors.New("Failed to create LLM stream: stream is nil")
	}

	defer func() {
		if stream != nil {
			if closeErr := stream.Close(); closeErr != nil {
				log.Printf("Error closing stream: %v", closeErr)
			}
		}
	}()

	fmt.Print("\nStreaming response: ")

	var fullResponseMessage string
	var chunkCount int = 0

	for {
		// Check for task cancellation during streaming
		select {
		case <-ctx.Done():
			log.Printf("LLM stream canceled for task")
			return []action.Action{}, "", errors.New("LLM request canceled by user")
		default:
			// Continue streaming
		}

		chunkCount++

		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// Stream completed normally - break out of loop
				fmt.Println("\n=== STREAM COMPLETED ===")
				break
			}
			// Check if the error is due to context cancellation
			if ctx.Err() == context.Canceled {
				log.Printf("LLM stream canceled during receive")
				return []action.Action{}, "", errors.New("LLM request canceled by user")
			}
			log.Printf("Error receiving from LLM stream: %v", err)
			return []action.Action{}, "", errors.New("Failed to receive response from LLM")
		}

		// Handle response from our generic interface with additional nil checks
		if response == nil {
			// Break immediately to prevent infinite loop
			break
		}

		// Process the response content
		if len(response.Choices) > 0 {
			// Always process the delta content, even if it's empty
			// This ensures proper stream termination detection
			fullResponseMessage += response.Choices[0].Delta.Content
			if response.Choices[0].Delta.Content != "" {
				fmt.Print(response.Choices[0].Delta.Content)
			}
		}
	}

	// Extract JSON
	fmt.Println("\nFULL RESPONSE MESSAGE:", fullResponseMessage)

	// Parse JSON into a slice of Action objects ------------------
	var actions []action.Action

	// Try to parse the full response as JSON array
	err = json.Unmarshal([]byte(fullResponseMessage), &actions)
	if err != nil {
		fmt.Println("Error parsing JSON as array:", err)

		// Try to extract JSON from the response using the same function as subtasks
		jsonStrings := extractJSONFromMarkdown(fullResponseMessage)
		log.Println("Extracted JSON strings:", jsonStrings)

		if len(jsonStrings) > 0 {
			// Try to parse each JSON string
			for _, jsonString := range jsonStrings {
				var singleAction action.Action
				err = json.Unmarshal([]byte(jsonString), &singleAction)
				if err == nil {
					actions = append(actions, singleAction)
					log.Println("Successfully parsed single action:", action.FormatVerbose(singleAction))
				} else {
					// Try to parse as array
					var actionArray []action.Action
					err = json.Unmarshal([]byte(jsonString), &actionArray)
					if err == nil {
						actions = append(actions, actionArray...)
						log.Println("Successfully parsed action array with", len(actionArray), "actions")
					}
				}
			}
		}

		if len(actions) == 0 {
			fmt.Println("No valid actions found in response, error:", err)
			return []action.Action{}, "", fmt.Errorf("failed to parse any valid actions from LLM response: %v", err)
		}
	}

	// Set actionsJSONStringReturn (compact JSON for subsequent LLM iterations)
	actionsJSONBytes, _ := json.Marshal(actions)
	actionsJSONStringReturn = string(actionsJSONBytes)

	// Sort actions by sequence ID
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Seq < actions[j].Seq
	})

	log.Printf("Successfully parsed %d actions from LLM response:", len(actions))
	for i, act := range actions {
		log.Printf("  %d. %s", i+1, action.FormatVerbose(act))
	}

	return actions, actionsJSONStringReturn, nil
}
