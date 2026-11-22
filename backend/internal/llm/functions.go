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
func GetOCRDeltaAbstractDescription(ctx context.Context, providerName, modelName, thinkingLevel, ocrDelta string, addTokensAndSendUpdate func(int)) (abstractDescription string, err error) {
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
		ctx,
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
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices for OCR delta abstract")
	}

	deltaJSONString := extractFirstJSONObject(resp.Choices[0].Message.Content)
	if deltaJSONString == "" {
		deltaJSONString = resp.Choices[0].Message.Content
	}

	fmt.Println("ocr abstract Delta:")
	fmt.Println(deltaJSONString)
	fmt.Println("ocr abstract Delta end:")
	return deltaJSONString, nil
}

// BreakGoalIntoSubtasks breaks down a goal into smaller subtasks
func BreakGoalIntoSubtasks(ctx context.Context, providerName, modelName, thinkingLevel, goal string, addTokensAndSendUpdate func(int)) ([]SubTask, error) {
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
		ctx,
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
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices for subtask breakdown")
	}

	subtasks, err := parseSubtasksJSON(resp.Choices[0].Message.Content)
	if err != nil {
		log.Printf("failed to parse subtasks JSON: %v; using goal as fallback subtask", err)
		return []SubTask{{Id: 1, Description: goal}}, nil
	}

	log.Printf("Successfully parsed %d subtasks: %+v\n", len(subtasks), subtasks)
	return subtasks, nil
}

// IsGoalAchieved checks if the goal has been achieved
func IsGoalAchieved(ctx context.Context, providerName, modelName, thinkingLevel, goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string, addTokensAndSendUpdate func(int)) (bool, string, string) {
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
		ctx,
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
	if len(resp.Choices) == 0 {
		return false, "LLM returned no choices", ""
	}

	data, err := parseVerdictJSON(resp.Choices[0].Message.Content)
	if err != nil {
		log.Printf("failed to parse verdict JSON: %v", err)
	}
	return data.IsGoalAchieved, data.Description, data.NewPrompt
}

// IsMainGoalAchieved strictly checks whether the ENTIRE main user goal is achieved,
// not merely an intermediate subtask.
func IsMainGoalAchieved(ctx context.Context, providerName, modelName, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string, addTokensAndSendUpdate func(int)) (bool, string, string) {
	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("LLM client not available (%s/%s): %v", providerName, modelName, err)
		return false, err.Error(), ""
	}

	messages := []Message{
		{
			Role: RoleSystem,
			Content: `Output only valid JSON with this exact structure: {"isGoalAchieved": boolean, "description": string, "newPrompt": string}.
- "isGoalAchieved" must be true ONLY if the ENTIRE main user goal is fully achieved, verified from the current screen state.
- WARNING: Do NOT set "isGoalAchieved": true merely because one intermediate subtask or step succeeded while the overall main goal remains unfinished. A partially completed plan means the main goal is NOT achieved.
- "description": factual explanation of which parts of the main goal are already satisfied and which parts are still missing.
- "newPrompt": if the main goal is not fully achieved, provide concise instructions for the missing work; leave empty if fully achieved.
- Base the verdict strictly on the current screen evidence: OCR data, OCR delta, windows, colors, cursor position and previous actions.`,
		},
		{
			Role: RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11).
Main goal: ` + mainGoal + `
Completed subtasks (may be only part of the main goal): ` + strconv.Quote(strings.Join(completedSubtasks, "\n")) + `

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

Strictly evaluate whether the ENTIRE main user goal is completely achieved from the current screen state. Completed subtasks are evidence, not proof of completion.`,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[isMainGoalAchieved][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		ctx,
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
		log.Printf("Failed to create LLM completion for main goal achievement check: %v", err)
		return false, "Failed to create LLM completion", ""
	}
	if len(resp.Choices) == 0 {
		return false, "LLM returned no choices", ""
	}

	data, err := parseVerdictJSON(resp.Choices[0].Message.Content)
	if err != nil {
		log.Printf("failed to parse main goal verdict JSON: %v", err)
	}
	return data.IsGoalAchieved, data.Description, data.NewPrompt
}

// ReplanSubtasks generates the remaining subtasks needed to finish the main goal.
func ReplanSubtasks(ctx context.Context, providerName, modelName, thinkingLevel, mainGoal string, completedSubtasks []string, verifierFeedback string, ocrJSONString string, windowsJSONString string, addTokensAndSendUpdate func(int)) ([]SubTask, error) {
	client, model, err := GetClientFor(providerName, modelName)
	if err != nil {
		log.Printf("LLM client not available (%s/%s): %v", providerName, modelName, err)
		return nil, err
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: `Output only valid JSON. Return a JSON array of remaining subtasks with this exact structure: [{"id": int, "description": string}]. Only include concrete subtasks that are still needed to finish the main goal; return an empty array if nothing remains.`,
		},
		{
			Role: RoleUser,
			Content: `Environment: Ubuntu Linux, xfce4 desktop (X11).
Main goal: ` + mainGoal + `
Already completed subtasks: ` + strconv.Quote(strings.Join(completedSubtasks, "\n")) + `
Verifier feedback about what is still missing: ` + verifierFeedback + `

Current screen state:
- OCR data: ` + ocrJSONString + `
- Detected windows: ` + windowsJSONString + `

Plan only the remaining concrete subtasks required to finish the main goal. Do not repeat completed subtasks unless the verifier feedback says a previous step must be recovered or redone.`,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[replanSubtasks][input]: %d\n", estimate.EstimatedTokens)
	addTokensAndSendUpdate(estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		ctx,
		&ChatCompletionRequest{
			Model:         model,
			Temperature:   0.4,
			MaxTokens:     2000,
			Messages:      messages,
			Stream:        false,
			JSONMode:      true,
			ThinkingLevel: thinkingLevel,
		},
	)
	if err != nil {
		log.Printf("Failed to create LLM completion for subtask replanning: %v", err)
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices for subtask replanning")
	}

	subtasks, err := parseSubtasksJSON(resp.Choices[0].Message.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse replanned subtasks JSON: %w", err)
	}
	return subtasks, nil
}

// Helper functions

// extractJSONFromMarkdown extracts JSON blocks (arrays or objects) from content.
// It respects string boundaries and escapes, and handles fenced markdown blocks.
func extractJSONFromMarkdown(content string) []string {
	var jsonStrings []string
	lines := strings.Split(content, "\n")
	inJSONBlock := false
	var currentJSON strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inJSONBlock && (strings.HasPrefix(trimmed, "```json") || strings.EqualFold(trimmed, "```")) {
			inJSONBlock = true
			continue
		}
		if inJSONBlock && strings.HasPrefix(trimmed, "```") {
			inJSONBlock = false
			jsonStr := strings.TrimSpace(currentJSON.String())
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

	if len(jsonStrings) > 0 {
		return jsonStrings
	}

	// Find the first complete JSON value (object or array) anywhere in the content.
	if s := scanJSONValue(content); s != "" {
		jsonStrings = append(jsonStrings, s)
	}
	return jsonStrings
}

// scanJSONValue returns the first balanced JSON object or array starting at or after from,
// ignoring braces/brackets inside strings.
func scanJSONValue(content string) string {
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c != '{' && c != '[' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for j := i; j < len(content); j++ {
			ch := content[j]
			if inString {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return content[i : j+1]
				}
			}
		}
		// Unbalanced; try next opening char.
	}
	return ""
}

func extractFirstJSONObject(content string) string {
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		if s := scanBalanced(content[i:], '{', '}', '[', ']'); s != "" {
			return s
		}
	}
	return ""
}

// scanBalanced returns the balanced value starting at s[0], or "" if unbalanced.
func scanBalanced(s string, open, close, otherOpen, otherClose byte) string {
	depth := 0
	inString := false
	escaped := false
	for j := 0; j < len(s); j++ {
		ch := s[j]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open, otherOpen:
			depth++
		case close, otherClose:
			depth--
			if depth == 0 {
				return s[:j+1]
			}
		}
	}
	return ""
}

func cleanJSONString(jsonStr string) string {
	// Remove any non-JSON content before and after the first complete JSON value.
	return scanJSONValue(jsonStr)
}

// parseSubtasksJSON parses subtasks from an array, a wrapped object, or a single object.
func parseSubtasksJSON(content string) ([]SubTask, error) {
	s := content
	if len(s) > 0 && (s[0] == '"' || s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		var raw string
		if err := json.Unmarshal([]byte(s), &raw); err == nil {
			s = raw
		}
	}
	if extracted := scanJSONValue(s); extracted != "" {
		s = extracted
	} else {
		s = strings.TrimSpace(s)
	}

	var subtasks []SubTask
	if err := json.Unmarshal([]byte(s), &subtasks); err == nil {
		return normalizeSubtasks(subtasks), nil
	}

	type wrapped struct {
		Subtasks []SubTask `json:"subtasks"`
		Steps    []SubTask `json:"steps"`
		Tasks    []SubTask `json:"tasks"`
	}
	var w wrapped
	if err := json.Unmarshal([]byte(s), &w); err == nil {
		if len(w.Subtasks) > 0 {
			return normalizeSubtasks(w.Subtasks), nil
		}
		if len(w.Steps) > 0 {
			return normalizeSubtasks(w.Steps), nil
		}
		if len(w.Tasks) > 0 {
			return normalizeSubtasks(w.Tasks), nil
		}
	}

	var single SubTask
	if err := json.Unmarshal([]byte(s), &single); err == nil {
		return normalizeSubtasks([]SubTask{single}), nil
	}

	return nil, fmt.Errorf("no valid subtasks JSON found in: %s", truncateForLog(content, 200))
}

func normalizeSubtasks(subtasks []SubTask) []SubTask {
	result := make([]SubTask, 0, len(subtasks))
	for _, st := range subtasks {
		st.Description = strings.TrimSpace(st.Description)
		if st.Description == "" {
			continue
		}
		if st.Id <= 0 {
			st.Id = len(result) + 1
		}
		result = append(result, st)
	}
	return result
}

func parseVerdictJSON(content string) (Verdict, error) {
	s := extractFirstJSONObject(content)
	if s == "" {
		return Verdict{}, fmt.Errorf("no valid JSON object found in verdict response: %s", truncateForLog(content, 200))
	}
	var v Verdict
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return Verdict{}, fmt.Errorf("failed to unmarshal verdict JSON: %w", err)
	}
	return v, nil
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
