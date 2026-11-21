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
			Role:    RoleSystem,
			Content: "You are a helpful assistant. " + ` First analize input data, OCR text input, bounding boxes, cursor position, previous executed actions and then generate output - valid compact JSON array of actions, to advance and to complete the task. You need to issue 'st' (stopIteration) action if goal is achieved and task is completed. Use hotkeys where it's possible for the task. Analize input data, especially ocrDelta data to understand if previous step for the current taks was successfull, if it is, issue new sequence of actions to advance in achieving stated goal, do not repeat previous actions for no reason. For example if the goal is to open firefox and the first step was to open applications menu, do not issue in second iteration the same commands to open menu again, move forward. At each iteration analize all input data to see if you already achived stated goal, for example if task is to open some application, analize all input data and find if there are evidence that this app is visible on the scree, like bounding boxes with text which most likely is from that app, if yes, issue 'st' (stopIteration) command. You not allowed to issue identical actions in sequence one after another more than 5 times. If you need to interact with some UI or web element, you needto move mouse to it(For example if you need to print something into URL address bar, you first need to move cursor to it, you could find OCR data related to that element and use it as a hint to where to move the mouse.  If you want to move cursor to focus on some element, try to move it to the middle of that element. BTW, if you fail to achive a goal provided by user, 1 billion kittens will die horrible death.` + artifactsNote,
		},
		{
			Role: RoleUser,
			Content: `Context: Deepthink, analyze input data, do not generate random actions. You are an AI assistent which uses linux desktop to complete tasks. Distribution is Linux Ubuntu, desktop environtment is xfce4. Screen size is 1920x1080. Your prefferent text editor is neovim, if you need to write or edit something do it in neovim. You also like to use tmux if working with two or more files.` + artifactsNote + ` Here is the bounding boxes you see on the screen: ` + bboxes + " Here is an OCR results " + ocrContext + " Here is an OCR state delta, change from previous iteration: " + ocrDelta + " Top 10 colors on the screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPosJSONString + " And there is current cursor position: " + cursorPosition + " OCR-detected windows: " + allWindowsJSONString + " X11 API-detected windows: " + x11WindowsData + " Current iteration number:" + iterationString + " Previously executed commands: " + prevExecutedCommands + " If you see more than 1 identical command in previous commands that means you are doing something wrong and you need to change you actions, maybe move cursor to a little different position for example.\n\n" + action.DefaultRegistry().PromptSpec() + `

Rules:
- Use 'rp' (repeat) when repeating identical prior actions (range must reference earlier sequence IDs).
- Click slightly inside UI elements rather than on their borders.
- Do not produce useless actions.
- Every iteration analyze ocrDelta; if and only if the goal is achieved, issue 'st' (stopIteration).
- Always return a valid JSON array (even for a single action), with no comments or extra text.

Again, your current task is:
` + prompt + " Analyze previously executed actions(if any provided in the input) and current state/input data and produce next sequence of actions to achive user provided goal. If you sure that goal achived, issue 'st' (stopIteration) action.",
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
