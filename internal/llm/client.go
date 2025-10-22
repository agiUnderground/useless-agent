package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"useless-agent/internal/action"

	deepseek "github.com/trustsight-io/deepseek-go"
)

// SendMessageToLLM sends a message to the LLM and returns actions to execute
func SendMessageToLLM(ctx context.Context, prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, x11WindowsData string, colorsDistribution string) (actionsToExecute []action.Action, actionsJSONStringReturn string, err error) {
	// Check if context is nil, use background context if it is
	if ctx == nil {
		log.Println("Warning: nil context provided to sendMessageToLLM, using background context")
		ctx = context.Background()
	}

	client, err := deepseek.NewClient(
		os.Getenv("API_KEY"),
		deepseek.WithBaseURL(os.Getenv("API_BASE_URL")),
		deepseek.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute, // added 5 instead of 1
		}),
		deepseek.WithMaxRetries(2),
		deepseek.WithMaxRequestSize(50<<20), // 50 MB
		deepseek.WithDebug(true),
	)
	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			log.Printf("LLM client creation canceled")
			return []action.Action{}, "", errors.New("LLM request canceled by user")
		}
		log.Printf("Failed to create LLM client: %v", err)
		return []action.Action{}, "", errors.New("Failed to create LLM client, error.")
	}
	defer client.Close()

	// Use the provided context for cancellation

	cursorPosition, _ := getCursorPositionJSON()
	iterationString := strconv.FormatInt(iteration, 10)

	log.Println("====================================================")
	log.Println("===================LLM INPUT========================")
	log.Println("====================================================")
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

	modelID := os.Getenv("MODEL_ID")

	// Create a streaming chat completion
	messages := []deepseek.Message{
		{
			Role:    deepseek.RoleSystem,
			Content: "You are a helpful assistant. " + ` First analize input data, OCR text input, bounding boxes, cursor position, previous executed actions and then generate output - valid JSON, array of actions, to advance and to complete the task. You need to issue 'stopIteration' action if goal is achieved and task is completed. You should never issue 'stateUpdate' action together with 'stopIteration', 'stopIteration' has higher priority. Use hotkeys where it's possible for the task. Do not issue any actions after the stateUpdate action. Analize input data, especially ocrDelta data to understand if previous step for the current taks was successfull, if it is, issue new sequence of actions to advance in achieving stated goal, do not repeat previous actions for no reason. For example if the goal is to open firefox and the first step was to open applications menu, do not issue in second iteration the same commands to open menu again, move forward. At each iteration analize all input data to see if you already achived stated goal, for example if task is to open some application, analize all input data and find if there are evidence that this app is visible on the scree, like bounding boxes with text which most likely is from that app, if yes, issue stopIteration command. You not allowed to issue the identical actions in sequence one after another more than 5 times. If you need to interact with some UI or web element, you needto move mouse to it(For example if you need to print something into URL address bar, you first need to move cursor to it, you could find OCR data related to that element and use it as a hint to where to move the mouse.  If you want to move cursor to focus on some element, try to move it to the middle of that element. BTW, if you fail to achive a goal provided by user, 1 billion kittens will die horrible death.`,
		},
		{
			Role: deepseek.RoleUser,
			Content: `Context: Deepthink, analyze input data, do not generate random actions. You are an AI assistent which uses linux desktop to complete tasks. Distribution is Linux Ubuntu, desktop environtment is xfce4. Screen size is 1920x1080. Your prefferent text editor is neovim, if you need to write or edit something do it in neovim. You also like to use tmux if working with two or more files. Here is the bounding boxes you see on the screen: ` + bboxes + " Here is an OCR results " + ocrContext + " Here is an OCR state delta, change from previous iteration: " + ocrDelta + " Top 10 colors on the screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPosJSONString + " And there is current cursor position: " + cursorPosition + " OCR-detected windows: " + allWindowsJSONString + " X11 API-detected windows: " + x11WindowsData + " Current iteration number:" + iterationString + " Previously executed commands: " + prevExecutedCommands + " If you see more than 1 identical command in previous commands that means you are doing something wrong and you need to change you actions, maybe move cursor to a little different position for example. " + ` To correctly solve the task you need to output a sequence of actions in json format, to advance on every action and every iteration and to achieve a stated goal, example of actions with explanations:
{
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
if you've done some action, for example mouse click which you know will change state of the system, like when clicking on a menu button it will open a menu, or any other action that will change visual state of the system, you can use "stateUpdate" action:
{
  "actionSequenceID": 6,
  "action": "stateUpdate"
}
when onsed "stateUpdate" action, you need to stop producing any other actions after it, because system will execute all your previous actions and will send you update with udpated visual information.
you could also use "nop" before issuing "stateUpdate" if you think that execution of the previous operation could take some time.
you can use 'printString' action:
{
  "actionSequenceID": 7,
  "action": "printString",
  "inputString": "Example string"
}
you can use 'keyTap' action:
{
  "actionSequenceID": 8,
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
  "actionSequenceID": 10,
  "action": "repeat",
  "actionsRange": [4,8],
  "repeatTimes": 3
}
use 'repeat' action always when you need to do repetitive identical task, for example to close N windows.
you not allowed to use 'stateUpdate' action before 'repeat' action.
If you want to click on some UI element, better to click a little bit 'inside' of it, because if cursor moved to the border of element, it could ignore actions.
You not allowed to produce useless actions.
Every iteration analizy ocrDelta data to understand if task is completed, if and only if it's completed issue stop iteration action.
json with actions need to be clean, WITHOUT ANY COMMENTS.
make sure json objects is separated with comma where it is needed, make sure that json is valid.
always return actions in JSON array, even if you want to execute only one action.
make sure you do not produce ANY actions AFTER the "stateUpdate" action. It's very important.

json for the actions need to be in one file. Json must be valid for golang parser.
Again, you current task is:
` + prompt + " Analyze previously executed actions(if any provided in the input) and current state/input data and produce next sequence of actions to achive user provided goal." + " If you sure that goal achived, issue 'stopIteration' action.",
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[main llm input func][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	fmt.Println("\nCreating streaming chat completion...")
	stream, err := client.CreateChatCompletionStream(
		ctx, // inc timeout
		&deepseek.ChatCompletionRequest{
			Model:           modelID,
			Temperature:     0.5, //default 1
			PresencePenalty: 0.3, //default is 0
			MaxTokens:       8192,
			Messages:        messages,
			Stream:          true,
			JSONMode:        true,
		},
	)
	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			log.Printf("LLM request canceled during stream creation")
			return []action.Action{}, "", errors.New("LLM request canceled by user")
		}
		log.Printf("Failed to create LLM stream: %v", err)
		return []action.Action{}, "", errors.New("Failed to send message to LLM, error.")
	}
	defer stream.Close()

	fmt.Print("\nStreaming response: ")

	var fullResponseMessage string
	for {
		// Check for task cancellation during streaming
		select {
		case <-ctx.Done():
			log.Printf("LLM stream canceled for task")
			return []action.Action{}, "", errors.New("LLM request canceled by user")
		default:
			// Continue streaming
		}

		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
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
		fullResponseMessage += response.Choices[0].Delta.Content
		fmt.Print(response.Choices[0].Delta.Content)
	}

	// Extract JSON
	fmt.Println("\nFULL RESPONSE MESSAGE:", fullResponseMessage)
	log.Println("FULL RESPONSE MESSAGE FOR DEBUGGING:", fullResponseMessage)

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
					log.Println("Successfully parsed single action:", singleAction.Action)
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

	// Set actionsJSONStringReturn for logging
	actionsJSONBytes, _ := json.Marshal(actions)
	actionsJSONStringReturn = string(actionsJSONBytes)

	// Sort actions by ActionSequenceID if present
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ActionSequenceID < actions[j].ActionSequenceID
	})

	log.Printf("Successfully parsed %d actions from LLM response", len(actions))
	for i, action := range actions {
		log.Printf("Action %d: %s, Parameters: %+v", i+1, action.Action, action.Parameters)
	}

	return actions, actionsJSONStringReturn, nil
}

// Helper functions that need to be implemented
func getCursorPositionJSON() (string, error) {
	// This should be moved to mouse package
	return "", nil
}

func addTokensAndSendUpdate(tokens int) {
	// This should be moved to token package
}
