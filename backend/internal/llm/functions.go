package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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
func GetOCRDeltaAbstractDescription(providerName, modelName, thinkingLevel, ocrDelta string, addTokensAndSendUpdate func(int)) (abstractDescription string, err error) {
	// Get LLM client for the task's provider/model
	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("LLM client not available (%s/%s): %v", providerName, modelName, err)
		return "", err
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "Output only valid JSON.",
		},
		{
			Role: RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11).
Analyze the OCR state delta below and summarize what became visible, what disappeared, and what changed.
- Filter out obviously misrecognized OCR noise; compute each section's bounding_box using only the remaining valid elements.
- Merge horizontally adjacent words into a single label (e.g., "Xfce" and "Terminal" -> "Xfce Terminal").
- Add a brief "note" to each section ("added", "removed", "modified") describing what the elements represent.
- Base the output strictly on the provided OCR delta. Do not invent or assume elements not present in the input. If nothing changed in a section, leave its elements empty.

Output format (preserve structure and key names, replace all values):
{
  "added": {
    "count": 0,
    "elements": [],
    "bounding_box": {"xMin": 0, "yMin": 0, "xMax": 0, "yMax": 0},
    "note": ""
  },
  "removed": {
    "count": 0,
    "elements": [],
    "bounding_box": {"xMin": 0, "yMin": 0, "xMax": 0, "yMax": 0},
    "note": ""
  },
  "modified": {
    "count": 0,
    "elements": [],
    "bounding_box": {"xMin": 0, "yMin": 0, "xMax": 0, "yMax": 0},
    "note": ""
  }
}

Input OCR delta: ` + ocrDelta,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[getOCRDeltaAbstractDescription][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:         model,
			Temperature:   1.0,
			MaxTokens:     8192,
			Messages:      messages,
			Stream:        false,
			JSONMode:      true,
			ThinkingLevel: thinkingLevel,
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
func BreakGoalIntoSubtasks(providerName, modelName, thinkingLevel, goal string, addTokensAndSendUpdate func(int)) ([]SubTask, error) {
	// Get LLM client for the task's provider/model
	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("LLM client not available (%s/%s): %v", providerName, modelName, err)
		return nil, err
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: `Output only valid JSON. Return a JSON array of objects with this exact structure: [{"id": int, "description": string}]. Always return an array, even if there is only one task.`,
		},
		{
			Role:    RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11). Break down the user's goal into small, easily verifiable steps. Do not split very simple goals (for example, "press alt + F4 hotkeys"). Return a JSON array, not a single object. Example: [{"id": 1, "description": "click on applications menu button"}, {"id": 2, "description": "click on 'web browser' submenu or similar (e.g. 'internet' -> 'firefox')"}, {"id": 3, "description": "move cursor to the middle of Firefox header"}, {"id": 4, "description": "drag firefox window by header to the left side of the screen"}]. Goal: ` + goal,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[breakGoalIntoSubtasks][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:         model,
			Temperature:   0.5,
			MaxTokens:     2000,
			Messages:      messages,
			Stream:        false,
			JSONMode:      true,
			ThinkingLevel: thinkingLevel,
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
func IsGoalAchieved(providerName, modelName, thinkingLevel, goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string, addTokensAndSendUpdate func(int)) (bool, string, string) {
	// Get LLM client for the task's provider/model
	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("LLM client not available (%s/%s): %v", providerName, modelName, err)
		return false, err.Error(), ""
	}

	messages := []Message{
		{
			Role: RoleSystem,
			Content: `Output only valid JSON with this exact structure: {"isGoalAchieved": boolean, "description": string, "newPrompt": string}.
- "description": brief factual explanation of why the goal is or is not achieved, based strictly on the input data.
- "newPrompt": if the goal is not achieved yet, provide concise next-step instructions that can be executed without needing another state update. If previous actions caused an undesirable state, include instructions to recover. Leave empty if the goal is achieved.
- Rely closely on OCR data, OCR delta, and the OCR delta summary to determine whether the expected changes appeared on screen.`,
		},
		{
			Role: RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11).
Goal: ` + goal + `

Current state:
- OCR delta: ` + ocrDelta + `
- OCR delta summary: ` + ocrDeltaAbstract + `
- Bounding boxes: ` + bboxes + `
- OCR data: ` + ocrJSONString + `
- Previous top 10 screen colors: ` + colorsDistributionBeforeAction + `
- Current top 10 screen colors: ` + colorsDistribution + `
- Previous cursor position: ` + prevCursorPositionJSONString + `
- Current cursor position: ` + currentCursorPosition + `
- OCR data near cursor (full width, 23px above to 23px below cursor): ` + ocrDataNearTheCursor + `
- Detected windows: ` + allWindowsJSONString + `
- Previous actions: ` + prevActionsJSONString + `
- Iteration: ` + strconv.FormatInt(iteration, 10) + `

Evaluate whether the desired outcome of the goal has been achieved based on the evidence in OCR data, OCR delta, and the OCR delta summary (treat OCR and input data as reliable). Focus on whether the intended outcome occurred (for example, whether the target app launched after clicking a menu item), and do not overcomplicate simple high-success actions like mouse clicks.`,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[isGoalAchieved][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&ChatCompletionRequest{
			Model:         model,
			Temperature:   0.3,
			MaxTokens:     2000,
			Messages:      messages,
			Stream:        false,
			JSONMode:      true,
			ThinkingLevel: thinkingLevel,
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
