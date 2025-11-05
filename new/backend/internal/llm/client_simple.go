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
	"time"

	"useless-agent/internal/action"
	"useless-agent/internal/config"
	"useless-agent/internal/mouse"
	"useless-agent/internal/token"
)

var (
	globalClient Client
	providers    map[string]Provider
)

func Initialize() error {
	providers = make(map[string]Provider)
	providers["deepseek"] = NewDeepSeekProvider()
	providers["zai"] = NewZAIProvider()

	cfg := config.GetLLMConfig()

	if cfg.APIKey == "" {
		return fmt.Errorf("LLM API key is required")
	}

	model := cfg.Model
	if model == "" {
		switch cfg.Provider {
		case "deepseek":
			model = "deepseek-chat"
		case "zai":
			model = "glm-4.6"
		default:
			return fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
		}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		switch cfg.Provider {
		case "deepseek":
			baseURL = "https://api.deepseek.com/v1"
		case "zai":
			baseURL = "https://api.z.ai/api/paas/v4"
		}
	}

	provider, exists := providers[cfg.Provider]
	if !exists {
		return fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}

	client, err := provider.CreateClient(ProviderConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    baseURL,
		ModelID:    model,
		Timeout:    5 * time.Minute,
		MaxRetries: 2,
		MaxSize:    50 << 20,
		Debug:      true,
	})
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	globalClient = client
	return nil
}

func GetClient() Client {
	return globalClient
}

func getModel() string {
	cfg := config.GetLLMConfig()
	model := cfg.Model
	if model == "" {
		switch cfg.Provider {
		case "deepseek":
			model = "deepseek-chat"
		case "zai":
			model = "glm-4.6"
		default:
			model = "deepseek-chat"
		}
	}
	return model
}

func SendMessageToLLM(ctx context.Context, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands string, iteration int64, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution string) ([]action.Action, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if globalClient == nil {
		if err := Initialize(); err != nil {
			return nil, "", errors.New("Failed to initialize LLM")
		}
	}

	cursorPosition, _ := mouse.GetCursorPositionJSON()
	iterationString := strconv.FormatInt(iteration, 10)

	systemPrompt := "You are a helpful assistant. First analize input data, OCR text input, bounding boxes, cursor position, previous executed actions and then generate output - valid JSON, array of actions, to advance and to complete task. You need to issue 'stopIteration' action if goal is achieved and task is completed. Use hotkeys where it's possible for task. Analize input data, especially ocrDelta data to understand if previous step for current taks was successfull, if it is, issue new sequence of actions to advance in achieving stated goal, do not repeat previous actions for no reason. For example if the goal is to open firefox and first step was to open applications menu, do not issue in second iteration same commands to open menu again, move forward. At each iteration analize all input data to see if you already achived stated goal, for example if the task is to open some application, analize all input data and find if there are evidence that this app is visible on scree, like bounding boxes with text which most likely is from that app, if yes, issue stopIteration command. You not allowed to issue identical actions in sequence one after another more than 5 times. If you need to interact with some UI or web element, you need to move mouse to it. For example if you need to print something into URL address bar, you first need to move cursor to it, you could find OCR data related to that element and use it as a hint to where to move the mouse. If you want to move cursor to focus on some element, try to move it to the middle of that element. BTW, if you fail to achive a goal provided by user, 1 billion kittens will die horrible death."

	userPrompt := "Context: Deepthink, analyze input data, do not generate random actions. You are an AI assistent which uses linux desktop to complete tasks. Distribution is Linux Ubuntu, desktop environtment is xfce4. Screen size is 1920x1080. Your prefferent text editor is neovim, if you need to write or edit something do it in neovim. You also like to use tmux if working with two or more files. Here is bounding boxes you see on screen: " + bboxes + " Here is an OCR results " + ocrContext + " Here is an OCR state delta, change from previous iteration: " + ocrDelta + " Top 10 colors on screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPosJSONString + " And there is current cursor position: " + cursorPosition + " OCR-detected windows: " + allWindowsJSONString + " X11 API-detected windows: " + x11WindowsData + " Current iteration number:" + iterationString + " Previously executed commands: " + prevExecutedCommands + " If you see more than 1 identical command in previous commands that means you are doing something wrong and you need to change you actions, maybe move cursor to a little different position for example. To correctly solve task you need to output a sequence of actions in json format, to advance on every action and every iteration and to achieve a stated goal, example of actions with explanations:"

	actionExamples := `{
  "action": "mouseMove",
  "coordinates": {
    "x": 555,
    "y": 777
  }
}
you can use 'mouseMoveRelative' action:
{
  "action": "mouseMoveRelative",
  "coordinates": {
    "x": -10,
    "y": 0
  }
}
You also need to add json field "actionSequenceID", to instruct the sequence in which system should execute your instructions, actionSequenceID should start from 1. Also you can use other actions like "mouseClickLeft":
{
  "actionSequenceID": 2,
  "action": "mouseClickLeft"
}
"mouseClickRight":
{
  "actionSequenceID": 3,
  "action": "mouseClickRight"
}
"mouseClickLeftDouble":
{
  "actionSequenceID": 4,
  "action": "mouseClickLeftDouble"
}
if you know that previous actions could take some time, you could use "nop" action(Duration is a positive int represents number of seconds to do nothing):
"nop":
{
  "actionSequenceID": 5,
  "action": "nop",
  "duration": 3
}
you could also use "nop" if you think that execution of previous operation could take some time.
you can use 'printString' action:
{
  "actionSequenceID": 6,
  "action": "printString",
  "inputString": "Example string"
}
you can use 'keyTap' action:
{
  "actionSequenceID": 7,
  "action": "keyTap",
  "keyTapString": "enter"
}
'keyTapString' string value can be:
    "backspace"
	"delete"
	"enter"
	"tab"
	"esc"
	"escape"
	"up"
	"down"
	"right"
	"left"
	"home"
	"end"
	"pageup"
	"pagedown"
you can use 'dragSmooth' action:
{
  "action": "dragSmooth",
  "coordinates": {
    "x": 555,
    "y": 777
  }
}
you can use 'scrollSmooth' action to scroll vertically(to scroll down, use negative y value):
{
  "action": "scrollSmooth",
  "coordinates": {
    "x": 0,
    "y": 77
  }
}
you can use 'keyDown' and 'keyUp' actions:
{
  "actionSequenceID": 9,
  "action": "keyDown",
  "keyString": "lctrl"
}
{
  "actionSequenceID": 10,
  "action": "keyUp",
  "keyString": "lalt"
}
you can use 'repeat' action to repeat previous range of action(next example repeats actions from 4 to 8 3 times), repeat must use only actions issued before it:
{
  "actionSequenceID": 11,
  "action": "repeat",
  "actionsRange": [4,8],
  "repeatTimes": 3
}
use 'repeat' action always when you need to do repetitive identical task, for example to close N windows.
If you want to click on some UI element, better to click a little bit 'inside' of it, because if cursor moved to border of element, it could ignore actions.
You not allowed to produce useless actions.
Every iteration analizy ocrDelta data to understand if task is completed, if and only if it's completed issue stop iteration action.
json with actions need to be clean, WITHOUT ANY COMMENTS.
make sure json objects is separated with comma where it is needed, make sure that json is valid.
always return actions in JSON array, even if you want to execute only one action.
json for actions need to be in one file. Json must be valid for golang parser.
Again, you current task is: " + prompt + " Analyze previously executed actions(if any provided in input) and current state/input data and produce next sequence of actions to achive user provided goal." + " If you sure that goal achived, issue 'stopIteration' action."`

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt + actionExamples,
		},
	}

	estimate := globalClient.EstimateTokensFromMessages(messages)
	token.AddTokensAndSendUpdate(estimate.EstimatedTokens)

	req := &ChatCompletionRequest{
		Model:           getModel(),
		Temperature:     0.5,
		PresencePenalty: 0.3,
		MaxTokens:       8192,
		Messages:        messages,
		Stream:          true,
		JSONMode:        true,
	}

	stream, err := globalClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return nil, "", errors.New("LLM request canceled by user")
		}
		return nil, "", errors.New("Failed to send message to LLM")
	}

	if stream == nil {
		return nil, "", errors.New("Failed to create LLM stream: stream is nil")
	}

	defer func() {
		if stream != nil {
			if closeErr := stream.Close(); closeErr != nil {
				log.Printf("Error closing stream: %v", closeErr)
			}
		}
	}()

	var fullResponseMessage string

	for {
		select {
		case <-ctx.Done():
			return nil, "", errors.New("LLM request canceled by user")
		default:
		}

		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			if ctx.Err() == context.Canceled {
				return nil, "", errors.New("LLM request canceled by user")
			}
			return nil, "", errors.New("Failed to receive response from LLM")
		}

		if response == nil {
			break
		}

		if len(response.Choices) > 0 {
			fullResponseMessage += response.Choices[0].Delta.Content
			if response.Choices[0].Delta.Content != "" {
				fmt.Print(response.Choices[0].Delta.Content)
			}
		}
	}

	var actions []action.Action
	err = json.Unmarshal([]byte(fullResponseMessage), &actions)
	if err != nil {
		fmt.Println("Error parsing JSON as array:", err)

		jsonStrings := extractJSONFromMarkdown(fullResponseMessage)
		if len(jsonStrings) > 0 {
			for _, jsonString := range jsonStrings {
				var singleAction action.Action
				err = json.Unmarshal([]byte(jsonString), &singleAction)
				if err == nil {
					actions = append(actions, singleAction)
				} else {
					var actionArray []action.Action
					err = json.Unmarshal([]byte(jsonString), &actionArray)
					if err == nil {
						actions = append(actions, actionArray...)
					}
				}
			}
		}
	}

	if len(actions) == 0 {
		return nil, "", fmt.Errorf("failed to parse any valid actions from LLM response")
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ActionSequenceID < actions[j].ActionSequenceID
	})

	return actions, fullResponseMessage, nil
}

func extractJSONFromMarkdown(text string) []string {
	return nil
}

func BreakGoalIntoSubtasks(goal string) ([]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func GetOCRDeltaAbstractDescription(ocrDelta string, tokenUpdateFunc func(int)) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func IsGoalAchieved(goal, bboxes, ocrJSONString, ocrDelta, ocrDeltaAbstract, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString, allWindowsJSONString, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeAction, colorsDistribution string, tokenUpdateFunc func(int)) (bool, string, string) {
	return false, "", ""
}
