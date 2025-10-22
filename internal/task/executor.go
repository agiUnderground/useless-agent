package task

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"time"

	"internal/vision"
	actionpkg "useless-agent/internal/action"
	imagepkg "useless-agent/internal/image"
	"useless-agent/internal/llm"
	"useless-agent/internal/mouse"
	"useless-agent/internal/ocr"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/token"
	"useless-agent/pkg/x11"
)

// colorCount represents a color and its count
type colorCount struct {
	Color color.Color `json:"color"`
	Count int         `json:"count"`
}

// BoundingBox represents a bounding box
type BoundingBox struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// ExecuteTask executes a task with the complete AGILoop implementation
func ExecuteTask(task *Task) {
	log.Printf("=== EXECUTING TASK %s ===", task.ID)
	log.Printf("Task message: %s", task.Message)

	var prevActionsJSONString string
	log.Println("prevActionsJSONString:", prevActionsJSONString)
	var iteration int64 = 1
	prompt := task.Message
	goal := prompt
	previousOCRText := ""
	var promptLog []PromptLog
	promptLog = append(promptLog, PromptLog{0, goal})
	var promptLogJSONString string
	promptLogBytes, err := json.Marshal(promptLog)
	if err != nil {
		log.Println("failed to marshal promptLog to JSON String:", err)
	}
	promptLogJSONString = string(promptLogBytes)
	prevCursorPositionJSONString, _ := getCursorPositionJSON()

	var subtasks []SubTask
	subtasks, err = breakGoalIntoSubtasks(goal)
	if err != nil {
		log.Println("Failed to break down goal into subtasks.")
		subtasks = nil
		subtasks = append(subtasks, SubTask{Id: 0, Description: goal})
	}

	for _, subtask := range subtasks {
	SubTaskLoop:
		for {
			// Check for task cancellation at the start of each iteration
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled during execution", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with normal execution
			}

			if iteration > 40 {
				break SubTaskLoop
			}

			// Check for task cancellation before screenshot
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before screenshot capture", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with screenshot
			}

			screenshotImg, err := screenshot.CaptureX11Screenshot()
			originalScreenshot := screenshotImg
			if err != nil {
				log.Printf("Failed to capture screenshot for task %s: %s", task.ID, err.Error())
				UpdateTaskStatus(task.ID, "broken", "Failed to capture screenshot")
				return
			}

			colorsDistribution := imagepkg.DominantColorsToJSONString(imagepkg.DominantColors(screenshotImg, 10))
			log.Println("colorsDistribution before actions: ", colorsDistribution)

			// Check for task cancellation before grayscale conversion
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before grayscale conversion", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with grayscale conversion
			}

			grayscaleScreenshot := screenshot.ConvertToGrayscale(screenshotImg)

			// Check for task cancellation before OCR
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before OCR processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with OCR
			}

			ocrResults := ocr.OCR(grayscaleScreenshot)

			// Check for task cancellation after OCR
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after OCR processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with window detection
			}

			// Get X11 windows data
			x11WindowsData, err := getX11WindowsData()
			if err != nil {
				log.Printf("Failed to get X11 windows data, continuing with empty data: %v", err)
				x11WindowsData = "[]"
			}
			log.Printf("X11 windows data: %s", x11WindowsData)

			var detectedWindowsJSON string = "["
			for index, ocrElement := range ocrResults {
				// Check for task cancellation during window detection loop
				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled during window detection", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
					// Continue with current window detection
				}

				ocrElementBB := image.Rect(ocrElement.BoundingBox.XMin, ocrElement.BoundingBox.YMin, ocrElement.BoundingBox.XMax, ocrElement.BoundingBox.YMax)
				windowsJSONString, err := vision.DetectWindow(grayscaleScreenshot, ocrElementBB, ocrElement.Text)
				if err != nil {
					log.Println("Failed to detect windows:", err)
					continue
				}
				detectedWindowsJSON += windowsJSONString
				if index != (len(ocrResults) - 1) {
					detectedWindowsJSON += ","
				}
			}
			detectedWindowsJSON += "]"
			log.Println("Detected windows json string data:", detectedWindowsJSON)

			// Keep X11 windows data separate from OCR-detected windows
			// They have different JSON formats and should be sent separately to LLM
			log.Printf("OCR-detected windows data: %s", detectedWindowsJSON)
			log.Printf("X11 API-detected windows data: %s", x11WindowsData)

			// Check for task cancellation after window detection
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after window detection", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with OCR processing
			}

			ocrResultsJSON := ocr.OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := ocr.MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = ocr.OCRtoJSONString(ocrDataMerged)
			}

			// Check for task cancellation after OCR JSON processing
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after OCR JSON processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with text changes
			}

			var textChanges ocr.Delta
			var textChangesSummary string
			var textChangesJSON string

			if iteration > 1 {
				textChanges, err = ocr.GetOCRDelta(previousOCRText, ocrResultsJSON)
				if err != nil {
					log.Printf("Filed to get OCR Delta[iteration: %d]: %s", iteration, err)
				}
				// Check for task cancellation after OCR delta calculation
				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled after OCR delta calculation", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
					// Continue with delta JSON string
				}

				textChangesJSON, err = ocr.GetOCRDeltaJSONString(textChanges)
				if err != nil {
					log.Printf("Filed to get OCR Delta JSON string[iteration: %d]: %s", iteration, err)
				}
				// Check for task cancellation after delta JSON string
				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled after OCR delta JSON string", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
					// Continue with abstract description
				}

				textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)
				if err != nil {
					log.Printf("Filed to get OCR Delta abstract description [iteration: %d]: %s", iteration, err)
				}
			}
			previousOCRText = ocrResultsJSON

			// Check for task cancellation after text changes processing
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after text changes processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with bounding boxes
			}

			boundingBoxesJSON := boundingBoxArrayToJSONString(findBoundingBoxes(originalScreenshot))

			var taskCompleted bool = false
			var nextPrompt string
			var completionStatus string

			log.Printf("iteration %d, original goal is: %s\n", iteration, goal)
			log.Printf("iteration %d, current task is: %s\n", iteration, subtask.Description)

			// Check for user-assist messages
			userAssistMsg := GetUserAssistMessage(task.ID)
			enhancedSubtaskDescription := subtask.Description
			if userAssistMsg != nil {
				log.Printf("Injecting user-assist message for task %s: %s", task.ID, userAssistMsg.Message)
				enhancedSubtaskDescription = subtask.Description + "\n\nHELPER MESSAGE FROM THE USER: " + userAssistMsg.Message
				log.Printf("Enhanced subtask description with user-assist: %s", enhancedSubtaskDescription)
			}

			promptLogBytes, err = json.Marshal(promptLog)
			if err != nil {
				log.Println("failed to marshal promptLog to JSON String:", err)
			}
			promptLogJSONString = string(promptLogBytes)

			actions, actionsJSONString, err := sendMessageToLLM(task.Context, enhancedSubtaskDescription, boundingBoxesJSON, ocrResultsJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, detectedWindowsJSON, x11WindowsData, colorsDistribution)

			if err != nil {
				log.Println("failed to send message to LLM:", err)
				// Check if the error is due to task cancellation
				if task.Context.Err() == context.Canceled {
					log.Printf("Task %s was canceled by user during LLM communication", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				} else {
					log.Printf("Task %s failed due to LLM communication error: %v", task.ID, err)
					UpdateTaskStatus(task.ID, "broken", "Failed to communicate with LLM")
				}
				return
			} else {
				log.Println("successfully sent a message to LLM. Iteration:", iteration)
			}

			for i := range actions {
				// Check for task cancellation before executing each action
				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled before executing action %d", task.ID, i)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
					// Continue with action execution
				}

				// Debug: Log action details before execution
				log.Printf("Preparing to execute action %d: %s", i, actions[i].Action)
				log.Printf("Action details: %+v", actions[i])

				setExecuteFunction(&actions[i])
				time.Sleep(100 * time.Millisecond)

				// Check if Execute function is set
				if actions[i].Execute == nil {
					log.Printf("ERROR: Execute function is nil for action %d: %s", i, actions[i].Action)
					continue
				}

				if actions[i].Action == "stopIteration" {
					log.Printf("Executing stopIteration action")
					actions[i].Execute(&actions[i])
					break SubTaskLoop
				}
				if actions[i].Action == "stateUpdate" {
					log.Printf("Executing stateUpdate action")
					actions[i].Execute(&actions[i])
					time.Sleep(1 * time.Second)
					break
				}
				if actions[i].Action == "repeat" {
					log.Printf("Executing repeat action")
					actions[i].Execute(&actions[i], &actions)
					fmt.Println()
					continue
				}

				log.Printf("Executing regular action: %s", actions[i].Action)
				actions[i].Execute(&actions[i])
				fmt.Println()
			}

			// Check for task cancellation before second screenshot
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before second screenshot capture", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with screenshot
			}

			screenshotImg, err = screenshot.CaptureX11Screenshot()
			originalScreenshot = screenshotImg
			if err != nil {
				log.Printf("Failed to capture screenshot for task %s: %s", task.ID, err.Error())
				UpdateTaskStatus(task.ID, "broken", "Failed to capture screenshot")
				return
			}

			// Check for task cancellation before second grayscale conversion
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before second grayscale conversion", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with grayscale conversion
			}

			grayscaleScreenshot = screenshot.ConvertToGrayscale(screenshotImg)

			// Check for task cancellation before second OCR
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled before second OCR processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with OCR
			}

			ocrResults = ocr.OCR(grayscaleScreenshot)

			// Check for task cancellation after second OCR
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second OCR processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with window detection
			}

			detectedWindowsJSON = "["
			for index, ocrElement := range ocrResults {
				// Check for task cancellation during second window detection loop
				select {
				case <-task.Context.Done():
					log.Printf("Task %s canceled during second window detection", task.ID)
					UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
					return
				default:
					// Continue with current window detection
				}

				ocrElementBB := image.Rect(ocrElement.BoundingBox.XMin, ocrElement.BoundingBox.YMin, ocrElement.BoundingBox.XMax, ocrElement.BoundingBox.YMax)
				windowsJSONString, err := vision.DetectWindow(grayscaleScreenshot, ocrElementBB, ocrElement.Text)
				if err != nil {
					log.Println("Failed to detect windows:", err)
					continue
				}
				detectedWindowsJSON += windowsJSONString
				if index != (len(ocrResults) - 1) {
					detectedWindowsJSON += ","
				}
			}
			detectedWindowsJSON += "]"
			log.Println("Detected windows json string data:", detectedWindowsJSON)

			// Check for task cancellation after second window detection
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second window detection", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with OCR JSON processing
			}

			ocrResultsJSON = ocr.OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := ocr.MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = ocr.OCRtoJSONString(ocrDataMerged)
			}

			// Check for task cancellation after second OCR JSON processing
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second OCR JSON processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with text changes
			}

			textChanges, err = ocr.GetOCRDelta(previousOCRText, ocrResultsJSON)
			if err != nil {
				log.Printf("Filed to get OCR Delta[iteration: %d]: %s", iteration, err)
			}
			// Check for task cancellation after second OCR delta calculation
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second OCR delta calculation", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with delta JSON string
			}

			textChangesJSON, err = ocr.GetOCRDeltaJSONString(textChanges)
			if err != nil {
				log.Printf("Filed to get OCR Delta JSON string[iteration: %d]: %s", iteration, err)
			}
			// Check for task cancellation after second delta JSON string
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second OCR delta JSON string", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with abstract description
			}

			textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)
			if err != nil {
				log.Printf("Filed to get OCR Delta abstract description [iteration: %d]: %s", iteration, err)
			}

			// Check for task cancellation after second text changes processing
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second text changes processing", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with bounding boxes
			}

			previousOCRText = ocrResultsJSON

			boundingBoxesJSON = boundingBoxArrayToJSONString(findBoundingBoxes(originalScreenshot))

			// Check for task cancellation after second bounding boxes
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after second bounding boxes", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with cursor position
			}

			currentCursorPosition, _ := getCursorPositionJSON()

			// Check for task cancellation after cursor position
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after cursor position", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with image under cursor
			}

			_, CursorY := getCursorPosition()
			log.Println("image under the cursor bounding box[x,y,x2,y2]:", 0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			rect := image.Rect(0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			imgUnderCursor := image.NewGray(rect)
			draw.Draw(imgUnderCursor, imgUnderCursor.Bounds(), grayscaleScreenshot, rect.Min, draw.Src)
			ocrDataNearTheCursor := ocr.OCRtoJSONString(ocr.OCR(imgUnderCursor))
			log.Println("OCR data near the cursor[46 pix height]:", ocrDataNearTheCursor)

			// Check for task cancellation after OCR under cursor
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after OCR under cursor", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with color distribution
			}

			colorsDistributionBeforeActions := colorsDistribution
			colorsDistribution = imagepkg.DominantColorsToJSONString(imagepkg.DominantColors(screenshotImg, 10))
			log.Println("colorsDistribution after actions: ", colorsDistribution)

			// Check for task cancellation after color distribution
			select {
			case <-task.Context.Done():
				log.Printf("Task %s canceled after color distribution", task.ID)
				UpdateTaskStatus(task.ID, "canceled", "Task canceled by user")
				return
			default:
				// Continue with goal achievement check
			}

			taskCompleted, completionStatus, nextPrompt = isGoalAchieved(subtask.Description, boundingBoxesJSON, ocrResultsJSON, textChangesJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, detectedWindowsJSON, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeActions, colorsDistribution)
			log.Println("Verdict description:", completionStatus)
			if taskCompleted {
				log.Println("Completed task: ", subtask.Description)
				log.Println("TASK COMPLETED! breaking SubTaskLoop...")
				promptLog = nil
				break SubTaskLoop
			} else {
				log.Println("task not achieved, new prompt is:", nextPrompt)
				prompt = nextPrompt
				promptLog = append(promptLog, PromptLog{iteration, nextPrompt})
			}

			iteration += 1
			prevActionsJSONString = actionsJSONString
			prevCursorPositionJSONString, _ = getCursorPositionJSON()
			time.Sleep(1 * time.Second)
		}
	}
	log.Println("GOAL ACHIEVED! breaking AGIloop...")

	// Update task status to completed
	UpdateTaskStatus(task.ID, "completed", "Task completed successfully")

	// Clean up user-assist messages for this task
	CleanupUserAssistMessages(task.ID)
}

// Helper functions that use the proper mouse package functions
func getCursorPositionJSON() (string, error) {
	return mouse.GetCursorPositionJSON()
}

func getCursorPosition() (int, int) {
	return mouse.GetCursorPosition()
}

func breakGoalIntoSubtasks(goal string) ([]SubTask, error) {
	llmSubtasks, err := llm.BreakGoalIntoSubtasks(goal, token.AddTokensAndSendUpdate)
	if err != nil {
		return nil, err
	}

	// Convert llm.SubTask to task.SubTask
	subtasks := make([]SubTask, len(llmSubtasks))
	for i, subtask := range llmSubtasks {
		subtasks[i] = SubTask{
			Id:          subtask.Id,
			Description: subtask.Description,
		}
	}
	return subtasks, nil
}

func getX11WindowsData() (string, error) {
	log.Printf("=== GETTING X11 WINDOWS DATA ===")
	x11WindowsJSON, err := x11.GetX11Windows()
	if err != nil {
		log.Printf("Failed to get X11 windows data: %v", err)
		return "[]", err
	}

	// Log the retrieved data for debugging
	log.Printf("X11 windows data retrieved successfully:")
	log.Printf("Raw JSON data: %s", x11WindowsJSON)

	log.Printf("=== X11 WINDOWS DATA END ===")
	return x11WindowsJSON, nil
}

func dominantColors(img image.Image, maxColors int) []colorCount {
	imageColors := imagepkg.DominantColors(img, maxColors)
	// Convert imagepkg.ColorCount to task.colorCount
	result := make([]colorCount, len(imageColors))
	for i, ic := range imageColors {
		result[i] = colorCount{
			Color: ic.Color,
			Count: ic.Count,
		}
	}
	return result
}

func dominantColorsToJSONString(colors []colorCount) string {
	// Convert task.colorCount to imagepkg.ColorCount for JSON conversion
	imageColors := make([]imagepkg.ColorCount, len(colors))
	for i, tc := range colors {
		// Type assert color.Color to color.RGBA
		rgba, ok := tc.Color.(color.RGBA)
		if !ok {
			// If it's not RGBA, convert it
			r, g, b, a := tc.Color.RGBA()
			rgba = color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
		}
		imageColors[i] = imagepkg.ColorCount{
			Color:      rgba,
			Count:      tc.Count,
			Percentage: 0, // Will be calculated in the image package
		}
	}
	return imagepkg.DominantColorsToJSONString(imageColors)
}

func findBoundingBoxes(img image.Image) []BoundingBox {
	bbArray := imagepkg.FindBoundingBoxes(img)
	// Convert imagepkg.BoundingBox to task.BoundingBox
	result := make([]BoundingBox, len(bbArray))
	for i, bb := range bbArray {
		result[i] = BoundingBox{
			X1: bb.X,
			Y1: bb.Y,
			X2: bb.X2,
			Y2: bb.Y2,
		}
	}
	return result
}

func boundingBoxArrayToJSONString(bbArray []BoundingBox) string {
	// Convert task.BoundingBox to imagepkg.BoundingBox for JSON conversion
	imageBBArray := make([]imagepkg.BoundingBox, len(bbArray))
	for i, bb := range bbArray {
		imageBBArray[i] = imagepkg.BoundingBox{
			ID: 0, // Will be set in the image package
			X:  bb.X1,
			Y:  bb.Y1,
			X2: bb.X2,
			Y2: bb.Y2,
		}
	}
	return imagepkg.BoundingBoxArrayToJSONString(imageBBArray)
}

func sendMessageToLLM(ctx context.Context, prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, x11WindowsData string, colorsDistribution string) ([]actionpkg.Action, string, error) {
	llmActions, actionsJSONString, err := llm.SendMessageToLLM(ctx, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands, iteration, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution)
	if err != nil {
		return nil, "", err
	}

	// Convert llm.Action to actionpkg.Action
	result := make([]actionpkg.Action, len(llmActions))
	for i, llmAction := range llmActions {
		// Create an actionpkg.Action with the same data
		result[i] = actionpkg.Action{
			Action:           llmAction.Action,
			ActionSequenceID: llmAction.ActionSequenceID,
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

func setExecuteFunction(action *actionpkg.Action) {
	// Add debugging to see what we're working with
	log.Printf("setExecuteFunction called for action: %s", action.Action)
	log.Printf("Coordinates: %+v", action.Coordinates)

	// Set the execute function using the action package
	actionpkg.SetExecuteFunction(action)
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
