package task

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"log"
	"time"

	"useless-agent/internal/action"
	imagepkg "useless-agent/internal/image"
	llm "useless-agent/internal/llm"
	"useless-agent/internal/mouse"
	"useless-agent/internal/ocr"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/token"
)

func ExecuteTask(task *Task) {
	log.Printf("=== EXECUTING TASK %s ===", task.ID)
	log.Printf("Task message: %s", task.Message)

	var iteration int64 = 1
	prompt := task.Message
	goal := prompt
	previousOCRText := ""
	var promptLog []PromptLog
	promptLog = append(promptLog, PromptLog{Iteration: 0, Message: goal})

	promptLogBytes, _ := json.Marshal(promptLog)
	promptLogJSONString := string(promptLogBytes)

	prevCursorPositionJSONString, _ := mouse.GetCursorPositionJSON()

	subtasks, err := breakGoalIntoSubtasks(goal)
	if err != nil {
		log.Println("Failed to break down goal into subtasks.")
		subtasks = []SubTask{{Id: 0, Description: goal}}
	}

	for _, subtask := range subtasks {
		UpdateSubtask(task.ID, subtask.Id, subtask.Description, false, []action.Action{})
	}

	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  task.ID,
		"status":  task.Status,
		"message": task.Message,
	})

	if len(subtasks) == 0 {
		log.Printf("Task %s has no subtasks, completing immediately", task.ID)
		UpdateTaskStatus(task.ID, "completed", "Task completed successfully - no actions needed")
		BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
			"taskId":  task.ID,
			"status":  "completed",
			"message": "Task completed successfully - no actions needed",
		})
		BroadcastExecutionEngineUpdate("completionEvent", map[string]interface{}{
			"taskId": task.ID,
			"event":  "completed",
		})
		CleanupUserAssistMessages(task.ID)
		return
	}

	for _, subtask := range subtasks {
		for {
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled during execution", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
			}

			if iteration > 40 {
				break
			}

			screenshotImg, err := screenshot.CaptureX11Screenshot()
			if err != nil {
				log.Printf("Failed to capture screenshot for task %s: %s", task.ID, err.Error())
				UpdateTaskStatus(task.ID, "broken", "Failed to capture screenshot")
				return
			}

			colorsDistribution := imagepkg.DominantColorsToJSONString(imagepkg.DominantColors(screenshotImg, 10))
			log.Println("colorsDistribution before actions: ", colorsDistribution)

			grayscaleScreenshot := screenshot.ConvertToGrayscale(screenshotImg)
			ocrResults := ocr.OCR(grayscaleScreenshot)

			ocrResultsJSON := ocr.OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := ocr.MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = ocr.OCRtoJSONString(ocrDataMerged)
			}

			var textChanges ocr.Delta
			var textChangesSummary string
			var textChangesJSON string

			if iteration > 1 {
				textChanges, err = ocr.GetOCRDelta(previousOCRText, ocrResultsJSON)
				if err != nil {
					log.Printf("Failed to get OCR Delta[iteration: %d]: %s", iteration, err)
				}

				textChangesJSON, err = ocr.GetOCRDeltaJSONString(textChanges)
				if err != nil {
					log.Printf("Failed to get OCR Delta JSON string[iteration: %d]: %s", iteration, err)
				}

				textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)
				if err != nil {
					log.Printf("Failed to get OCR Delta abstract description [iteration: %d]: %s", iteration, err)
				}
			}
			previousOCRText = ocrResultsJSON

			boundingBoxesJSON := imagepkg.BoundingBoxArrayToJSONString(imagepkg.FindBoundingBoxes(screenshotImg))

			userAssistMsg := GetUserAssistMessage(task.ID)
			enhancedSubtaskDescription := subtask.Description
			if userAssistMsg != nil {
				log.Printf("Injecting user-assist message for task %s: %s", task.ID, userAssistMsg.Message)
				enhancedSubtaskDescription = subtask.Description + "\n\nHELPER MESSAGE FROM THE USER: " + userAssistMsg.Message
			}

			promptLogBytes, _ = json.Marshal(promptLog)
			promptLogJSONString = string(promptLogBytes)

			var prevActionsJSONString string
			actions, actionsJSONString, err := sendMessageToLLM(task.Context, enhancedSubtaskDescription, boundingBoxesJSON, ocrResultsJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, prevActionsJSONString, "[]", colorsDistribution)

			UpdateSubtask(task.ID, subtask.Id, subtask.Description, true, actions)

			BroadcastExecutionEngineUpdate("subtaskUpdate", map[string]interface{}{
				"taskId":      task.ID,
				"subtaskId":   subtask.Id,
				"description": subtask.Description,
				"isActive":    true,
			})

			if err != nil {
				log.Println("failed to send message to LLM:", err)
				if task.Context.Err() == context.Canceled {
					log.Printf("Task %s was canceled by user during LLM communication", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				} else {
					log.Printf("Task %s failed due to LLM communication error: %v", task.ID, err)
					UpdateTaskStatus(task.ID, "broken", "Failed to communicate with LLM")
				}
				return
			}

			for i, act := range actions {
				UpdateAction(task.ID, subtask.Id, i, act)

				actionData := map[string]interface{}{
					"actionId":    i,
					"action":      act.Action,
					"description": act.Description,
				}

				if act.Coordinates.X != 0 || act.Coordinates.Y != 0 {
					actionData["coordinates"] = act.Coordinates
				}
				if act.InputString != "" {
					actionData["inputString"] = act.InputString
				}
				if act.KeyTapString != "" {
					actionData["keyTapString"] = act.KeyTapString
				}
				if act.Duration != 0 {
					actionData["duration"] = act.Duration
				}

				BroadcastExecutionEngineUpdate("actionUpdate", map[string]interface{}{
					"taskId":      task.ID,
					"subtaskId":   subtask.Id,
					"actionIndex": i,
					"action":      actionData,
				})

				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled before executing action %d", task.ID, i)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
				}

				log.Printf("Preparing to execute action %d: %s", i, actions[i].Action)
				action.SetExecuteFunction(&act)
				time.Sleep(100 * time.Millisecond)

				if act.Execute == nil {
					log.Printf("ERROR: Execute function is nil for action %d: %s", i, act.Action)
					continue
				}

				if act.Action == "stopIteration" {
					log.Printf("Executing stopIteration action")
					act.Execute(&act)
					break
				}

				if act.Action == "repeat" {
					log.Printf("Executing repeat action")
					act.Execute(&act, &actions)
					fmt.Println()
					continue
				}

				log.Printf("Executing regular action: %s", act.Action)
				act.Execute(&act)
				fmt.Println()
			}

			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before second screenshot capture", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
			}

			screenshotImg, err = screenshot.CaptureX11Screenshot()
			if err != nil {
				log.Printf("Failed to capture screenshot for task %s: %s", task.ID, err.Error())
				UpdateTaskStatus(task.ID, "broken", "Failed to capture screenshot")
				return
			}

			grayscaleScreenshot = screenshot.ConvertToGrayscale(screenshotImg)
			ocrResults = ocr.OCR(grayscaleScreenshot)
			ocrResultsJSON = ocr.OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := ocr.MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = ocr.OCRtoJSONString(ocrDataMerged)
			}

			textChanges, err = ocr.GetOCRDelta(previousOCRText, ocrResultsJSON)
			if err != nil {
				log.Printf("Failed to get OCR Delta[iteration: %d]: %s", iteration, err)
			}

			textChangesJSON, err = ocr.GetOCRDeltaJSONString(textChanges)
			if err != nil {
				log.Printf("Failed to get OCR Delta JSON string[iteration: %d]: %s", iteration, err)
			}

			textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)
			if err != nil {
				log.Printf("Failed to get OCR Delta abstract description [iteration: %d]: %s", iteration, err)
			}

			previousOCRText = ocrResultsJSON
			boundingBoxesJSON = imagepkg.BoundingBoxArrayToJSONString(imagepkg.FindBoundingBoxes(screenshotImg))

			currentCursorPosition, _ := mouse.GetCursorPositionJSON()
			_, CursorY := mouse.GetPosition()
			log.Println("image under the cursor bounding box[x,y,x2,y2]:", 0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			rect := image.Rect(0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			imgUnderCursor := image.NewGray(rect)
			draw.Draw(imgUnderCursor, imgUnderCursor.Bounds(), grayscaleScreenshot, rect.Min, draw.Src)
			ocrDataNearTheCursor := ocr.OCRtoJSONString(ocr.OCR(imgUnderCursor))
			log.Println("OCR data near the cursor[46 pix height]:", ocrDataNearTheCursor)

			colorsDistributionBeforeActions := colorsDistribution
			colorsDistribution = imagepkg.DominantColorsToJSONString(imagepkg.DominantColors(screenshotImg, 10))
			log.Println("colorsDistribution after actions: ", colorsDistribution)

			taskCompleted, completionStatus, nextPrompt := isGoalAchieved(subtask.Description, boundingBoxesJSON, ocrResultsJSON, textChangesJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, "[]", currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeActions, colorsDistribution)
			log.Println("Verdict description:", completionStatus)
			if taskCompleted {
				log.Println("Completed task: ", subtask.Description)
				log.Println("TASK COMPLETED! breaking SubTaskLoop...")
				promptLog = nil
				break
			} else {
				log.Println("task not completed, new prompt is:", nextPrompt)
				prompt = nextPrompt
				promptLog = append(promptLog, PromptLog{Iteration: iteration, Message: nextPrompt})
			}

			iteration += 1
			prevActionsJSONString = actionsJSONString
			prevCursorPositionJSONString, _ = mouse.GetCursorPositionJSON()
			time.Sleep(1 * time.Second)
		}
	}

	log.Println("GOAL ACHIEVED! breaking AGIloop...")

	UpdateTaskStatus(task.ID, "completed", task.Message)

	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  task.ID,
		"status":  "completed",
		"message": task.Message,
	})

	BroadcastExecutionEngineUpdate("completionEvent", map[string]interface{}{
		"taskId": task.ID,
		"event":  "completed",
	})

	CleanupUserAssistMessages(task.ID)
}

func breakGoalIntoSubtasks(goal string) ([]SubTask, error) {
	llmSubtasks, err := llm.BreakGoalIntoSubtasks(goal)
	if err != nil {
		return nil, err
	}

	subtasks := make([]SubTask, len(llmSubtasks))
	for i, subtask := range llmSubtasks {
		// Type assert to string since we changed the return type in llm/client_simple.go
		subtaskStr, ok := subtask.(string)
		if !ok {
			subtaskStr = fmt.Sprintf("%v", subtask) // Fallback conversion
		}
		subtasks[i] = SubTask{
			Id:          i + 1, // Use index as ID since LLM returns strings
			Description: subtaskStr,
		}
	}
	return subtasks, nil
}

func sendMessageToLLM(ctx context.Context, prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, x11WindowsData string, colorsDistribution string) ([]action.Action, string, error) {
	llmActions, actionsJSONString, err := llm.SendMessageToLLM(ctx, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands, iteration, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution)
	if err != nil {
		return nil, "", err
	}

	result := make([]action.Action, len(llmActions))
	for i, llmAction := range llmActions {
		result[i] = action.Action{
			ActionSequenceID: llmAction.ActionSequenceID,
			Action:           llmAction.Action,
			Coordinates:      llmAction.Coordinates,
			Duration:         llmAction.Duration,
			InputString:      llmAction.InputString,
			KeyTapString:     llmAction.KeyTapString,
			KeyString:        llmAction.KeyString,
			ActionsRange:     llmAction.ActionsRange,
			RepeatTimes:      llmAction.RepeatTimes,
			Description:      llmAction.Description,
		}
	}

	return result, actionsJSONString, nil
}

func getOCRDeltaAbstractDescription(ocrDelta string) (string, error) {
	return llm.GetOCRDeltaAbstractDescription(ocrDelta, token.AddTokensAndSendUpdate)
}

func isGoalAchieved(goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string) (bool, string, string) {
	return llm.IsGoalAchieved(goal, bboxes, ocrJSONString, ocrDelta, ocrDeltaAbstract, prevActionsJSONString, iteration, prevCursorPositionJSONString, allWindowsJSONString, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeAction, colorsDistribution, token.AddTokensAndSendUpdate)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
