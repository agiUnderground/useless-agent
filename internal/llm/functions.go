package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"useless-agent/internal/config"
)

// SubTask represents a subtask in the goal breakdown (local copy to avoid import cycle)
type SubTask struct {
	Id          int    `json:"id"`
	Description string `json:"description"`
}

// Verdict represents goal achievement verdict
type Verdict struct {
	IsGoalAchieved bool   `json:"isGoalAchieved"`
	Description    string `json:"description"`
	NewPrompt      string `json:"newPrompt"`
}

// GetOCRDeltaAbstractDescription gets an abstract description of OCR changes
func GetOCRDeltaAbstractDescription(ocrDelta string, addTokensAndSendUpdate func(int)) (abstractDescription string, err error) {
	// Get LLM client
	client := GetLLMClient()
	if client == nil {
		log.Printf("LLM client not initialized")
		return "", fmt.Errorf("LLM client not initialized")
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "You are a helpful assistant. " + " Output only valid JSON. ",
		},
		{
			Role: RoleUser,
			Content: `
              Context: Linux, ubuntu, xfce4, X11. It's linux xfce desktop data. Delta between states. Analize data and summarize what became visible and what not visible anymore.
              You need to add to parent component summary bounding box with coordinates which contains all child elements. and remove most obviously wrong recognized ocr text from elements.
              Only summarize coordinates of clean objects, all that vas previously filtered out just ignore.
              If child elements very close to each other horizontally, join them, like "Xfce" and "Terminal" they are located near each other join them to "Xfce Terminal".
              Also add a little 'note' to each 'added' 'removed' 'modified' selctions with summarization of what that object/objects must be. For example for 'removed' section here.
              This json MUST BE GENERATED ONLY BASED ON INPUT OCR DATA AND NOTHING ELSE, if you will not follow this instruction, 10000 billion kitten will die by hirrible death.
              Example output format, use only structure and key names, all content should be replaced:
              {
                "added": {
                  "count": 14,
                  "elements": [
                    "",
                    "",
                    "",
                  ],
                  "bounding_box": {
                    "xMin": 7,
                    "yMin": 7,
                    "xMax": 7,
                    "yMax": 7
                  },
                  "note": ""
                },
                "removed": {
                  "count": 2,
                  "elements": [
                    "",
                    ""
                  ],
                  "bounding_box": {
                    "xMin": 5,
                    "yMin": 5,
                    "xMax": 5,
                    "yMax": 5
                  },
                  "note": ""
                },
                "modified": {
                  "count": 4,
                  "elements": [
                    "",
                    "",
                    "",
                    ""
                  ],
                  "bounding_box": {
                    "xMin": 1,
                    "yMin": 1,
                    "xMax": 1,
                    "yMax": 1
                  },
                  "note": ""
                }
              }
              Output json content should be generated fully based on input ocr data, if nothing changed, you could say that nothing changed or if nothing added or removed you can keep that objects emtpy. If you try to generate random data or output json wouldn't be based on input data, 100 billion kitten will die horrible death. If input ocr data show that firefox window for example was open, but you generated output which says that applications windows was opened, 100000 billion kitten will die.
            ` + " Here is input OCR data/delta: " + ocrDelta,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[getOCRDeltaAbstractDescription][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	// Get configuration for model
	cfg := config.GetLLMConfig()
	model := cfg.Model
	if model == "" {
		switch cfg.Provider {
		case "deepseek":
			model = "deepseek-chat"
		case "zai":
			model = "glm-4.6"
		}
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:       model,
			Temperature: 1.0,
			MaxTokens:   8192,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Printf("Failed to create LLM completion for OCR delta abstract: %v", err)
		return "", err
	}

	fmt.Println(resp.Choices[0].Message.Content)

	deltaJSONString := resp.Choices[0].Message.Content

	fmt.Println("ocr abstract Delta:")
	fmt.Println(deltaJSONString)
	fmt.Println("ocr abstract Delta end:")
	return deltaJSONString, nil
}

// BreakGoalIntoSubtasks breaks down a goal into smaller subtasks
func BreakGoalIntoSubtasks(goal string, addTokensAndSendUpdate func(int)) ([]SubTask, error) {
	// Get LLM client
	client := GetLLMClient()
	if client == nil {
		log.Printf("LLM client not initialized")
		return nil, fmt.Errorf("LLM client not initialized")
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "You are a helpful assistant. " + ` Output only valid JSON. You MUST return a JSON ARRAY of objects with this exact structure: [{"id": int, "description": string}]. Do NOT return a single object, always return an array even if there's only one task.`,
		},
		{
			Role:    RoleUser,
			Content: ` Break down user provided goal into primitive tasks which program can execute and easily verify. Do not break very simple goal into tasks(example of simple goal:"press alt + F4 hotkeys"). Context: Linux desktop, xfce4, X11. IMPORTANT: You must return a JSON ARRAY, not a single object. Example: [{"id": 1, "description": "click on applications menu button"}, {"id": 2, "description": "click on 'web browser' submenu or something similar, there could be 'internet'->'firefox' submenus"}, {"id": 3, "description": "move cursor to the middle of Firefox header"}, {"id": 4, "description": "drag firefox window by header and move it to the left side of the screen"}]. User provided goal is: ` + goal,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[breakGoalIntoSubtasks][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	// Get configuration for model
	cfg := config.GetLLMConfig()
	model := cfg.Model
	if model == "" {
		switch cfg.Provider {
		case "deepseek":
			model = "deepseek-chat"
		case "zai":
			model = "glm-4.6"
		}
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:       model,
			Temperature: 0.5,
			MaxTokens:   2000,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Printf("Failed to create LLM completion for subtask breakdown: %v", err)
		return nil, err
	}

	log.Println("\n\nresp(must be json):", resp)
	jsonStrings := extractJSONFromMarkdown(resp.Choices[0].Message.Content)
	log.Println("\n\njsonStrings:", jsonStrings)

	// Use the first valid JSON string found, don't join multiple JSON objects
	var s string
	if len(jsonStrings) > 0 {
		s = jsonStrings[0] // Use the first valid JSON string
	} else {
		// If no JSON found in markdown, try the raw response
		s = resp.Choices[0].Message.Content
	}

	log.Println("Raw JSON string before processing:", s)

	subtasks := make([]SubTask, 0, 10_000)

	// First, try to unmarshal as an array directly
	err = json.Unmarshal([]byte(s), &subtasks)
	if err != nil {
		log.Println("failed to unmarshal subtasks from byte string to struct:", err)

		// Try to clean JSON string by removing any non-JSON content
		cleanedJSON := cleanJSONString(s)
		log.Println("Cleaned JSON string:", cleanedJSON)

		if cleanedJSON != "" {
			err = json.Unmarshal([]byte(cleanedJSON), &subtasks)
			if err != nil {
				log.Println("failed to unmarshal cleaned subtasks:", err)

				// Try to handle case where LLM returned a single object instead of array
				var singleSubtask SubTask
				err = json.Unmarshal([]byte(cleanedJSON), &singleSubtask)
				if err == nil {
					// If single object unmarshaling works, wrap it in an array
					subtasks = []SubTask{singleSubtask}
					log.Println("Successfully converted single object to array")
				} else {
					log.Println("failed to unmarshal as single object too:", err)
					// As a fallback, return a single subtask with original goal
					subtasks = []SubTask{{Id: 1, Description: goal}}
					log.Println("Using fallback subtask with original goal")
				}
			}
		} else {
			// As a fallback, return a single subtask with original goal
			subtasks = []SubTask{{Id: 1, Description: goal}}
			log.Println("Using fallback subtask with original goal (no valid JSON)")
		}
	}

	log.Printf("Successfully parsed %d subtasks: %+v\n", len(subtasks), subtasks)
	return subtasks, nil
}

// IsGoalAchieved checks if the goal has been achieved
func IsGoalAchieved(goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string, addTokensAndSendUpdate func(int)) (bool, string, string) {
	// Get LLM client
	client := GetLLMClient()
	if client == nil {
		log.Printf("LLM client not initialized")
		return false, "LLM client not initialized", ""
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "You are a helpful assistant. " + ` Output only valid JSON. Json object structure: {"isGoalAchieved": boolean, "description": string, "newPrompt": string} ` + ` If goal is not achieved yet, 'newPrompt' field must include only promitive instructions for the next step to execute which does not require state update to execute them. 'description' field must contain short description of why you decided that goal achieved or not, only based on facts(input data and stated goal). Analyze Previous actions and current state using input data and if you see that previous action or actions caused undesirable state, issue additional commands to fix that state. Very important: analize ocr data, ocr delta and ocr abstract delta, those data mosly like will show you if goal was acomplished because they will contain new text data that appeared on the screen or removed from the screen.`,
		},
		{
			Role:    RoleUser,
			Content: "Let's say you using linux desktop, xfce4, X11, your goal is: " + goal + ", here current state of the desktop(what you see): " + " OCR delta: " + ocrDelta + " Bounding boxes: " + bboxes + " OCR data: " + ocrJSONString + " Summary of OCR delta: " + ocrDeltaAbstract + " Previous top 10 colors on the screen: " + colorsDistributionBeforeAction + " Current top 10 colors on the screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPositionJSONString + " Current cursor position: " + currentCursorPosition + " And here is OCR data near the cursor(bounding box is full window width but starts 23 pixels above the cursor and ends 23 pixels below the cursor): " + ocrDataNearTheCursor + " Detected windows: " + allWindowsJSONString + " Previous actions: " + prevActionsJSONString + " Current iteration: " + strconv.FormatInt(iteration, 10) + " Very important: analize ocr data, ocr delta and ocr abstract delta, those data mosly like will show you if goal was acomplished because they will contain new text data that appeared on the screen or removed from the screen. You can not ignore evidence from ocr input data, especially from abstract ocr delta. You goal as a reviwer not to find evidence that action mentions in the task was executed, but that this action leads to the desiared outcome, and if that's true, then the task is completed. For example when task was to click on some submenu, you should focus if data shows that application you wanted to start by doing that is started or not. And do not complicate easy tasks which have very high chance of success, like clicking a mouse button is almost always 100 percent success. Let's assume that OCR and other input data is relieble. Did you acomplished the task?",
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[isGoalAchieved][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	// Get configuration for model
	cfg := config.GetLLMConfig()
	model := cfg.Model
	if model == "" {
		switch cfg.Provider {
		case "deepseek":
			model = "deepseek-chat"
		case "zai":
			model = "glm-4.6"
		}
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:       model,
			Temperature: 0.3,
			MaxTokens:   2000,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Printf("Failed to create LLM completion for goal achievement check: %v", err)
		return false, "Failed to create LLM completion", ""
	}

	jsonStrings := extractJSONFromMarkdown(resp.Choices[0].Message.Content)
	log.Println("\n\njsonStrings:", jsonStrings)
	s := strings.Join(jsonStrings[:], ",")

	data := Verdict{}
	err = json.Unmarshal([]byte(s), &data)
	if err != nil {
		log.Println("failed to unmarshal verdict from byte string to struct:", err)
	}

	fmt.Println("is goal achieved function result string:", s)

	return data.IsGoalAchieved, data.Description, data.NewPrompt
}

// Helper functions

func extractJSONFromMarkdown(content string) []string {
	var jsonStrings []string
	lines := strings.Split(content, "\n")
	inJSONBlock := false
	var currentJSON strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```json") {
			inJSONBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "```") && inJSONBlock {
			inJSONBlock = false
			jsonStr := currentJSON.String()
			if jsonStr != "" {
				jsonStrings = append(jsonStrings, jsonStr)
			}
			currentJSON.Reset()
			continue
		}
		if inJSONBlock {
			currentJSON.WriteString(line)
			currentJSON.WriteString("\n")
		}
	}

	// Also try to find JSON objects directly in the content
	if len(jsonStrings) == 0 {
		// Look for JSON objects in the content
		start := strings.Index(content, "{")
		if start != -1 {
			// Find matching closing brace
			braceCount := 0
			for i := start; i < len(content); i++ {
				if content[i] == '{' {
					braceCount++
				} else if content[i] == '}' {
					braceCount--
					if braceCount == 0 {
						jsonStrings = append(jsonStrings, content[start:i+1])
						break
					}
				}
			}
		}
	}

	return jsonStrings
}

func cleanJSONString(jsonStr string) string {
	// Remove any non-JSON content before and after JSON object
	start := strings.Index(jsonStr, "{")
	end := strings.LastIndex(jsonStr, "}")
	if start != -1 && end != -1 && end > start {
		return jsonStr[start : end+1]
	}
	return ""
}
