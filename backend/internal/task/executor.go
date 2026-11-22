package task

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"log"
	"strings"

	"internal/vision"
	actionpkg "useless-agent/internal/action"
	"useless-agent/internal/config"
	imagepkg "useless-agent/internal/image"
	"useless-agent/internal/llm"
	"useless-agent/internal/mouse"
	"useless-agent/internal/ocr"
	"useless-agent/internal/screenshot"
	"useless-agent/internal/storage"
	"useless-agent/internal/token"
	"useless-agent/pkg/x11"
)

const (
	maxTotalIterations   int64 = 40
	maxSubtaskIterations int64 = 12
	maxReplanAttempts          = 3
)

// ScreenObservation holds a complete snapshot of the desktop state for one step.
type ScreenObservation struct {
	Screenshot         image.Image
	Grayscale          *image.Gray
	ColorsDistribution string
	X11WindowsJSON     string
	DetectedWindows    string
	OCRResultsJSON     string
	TextChangesJSON    string
	TextChangesSummary string
	BoundingBoxesJSON  string
	CursorPositionJSON string
	OCRNearCursorJSON  string
}

// ActionBatchResult captures the outcome of executing one LLM-generated action batch.
type ActionBatchResult struct {
	ReportedComplete bool // True when the subtask explicitly issued stopIteration ("st").
}

// CompletionReport carries the context when a subtask or task reports completion via callback.
type CompletionReport struct {
	TaskID            string
	SubtaskID         int
	SubtaskDesc       string
	Iteration         int64
	ActionsJSON       string
	ColorsBefore      string
	PrevCursorPosJSON string
}

// ExecutionDeps encapsulates external dependencies of the execution engine for modularity and testability.
type ExecutionDeps struct {
	PrepareArtifactsDir   func(taskID string) (string, error)
	CaptureObservation    func(ctx context.Context, task *Task, previousOCR string, iteration int64, includeCursorOCR bool) (*ScreenObservation, error)
	GetCursorPositionJSON func() (string, error)
	BreakGoalIntoSubtasks func(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error)
	ReplanSubtasks        func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, verifierFeedback, ocrJSON, windowsJSON string) ([]SubTask, error)
	SendMessageToLLM      func(ctx context.Context, provider, model, thinkingLevel, artifactsDir, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands string, iteration int64, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution string) ([]actionpkg.Action, string, error)
	ExecuteActions        func(ctx context.Context, task *Task, subtaskID int, actions []actionpkg.Action, onComplete func()) (ActionBatchResult, error)
	VerifySubtask         func(ctx context.Context, provider, model, thinkingLevel, subtaskGoal, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string)
	VerifyMainGoal        func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string)
	UpdateTaskStatus      func(taskID, status, message string)
	UpdateSubtask         func(taskID string, subtaskID int, description string, isActive bool, actions []actionpkg.Action)
	BroadcastUpdate       func(updateType string, data interface{})
	CleanupUserAssist     func(taskID string)
	GetUserAssist         func(taskID string) *UserAssistMessage
}

func defaultExecutionDeps() ExecutionDeps {
	return ExecutionDeps{
		PrepareArtifactsDir:   storage.GetTaskArtifactsDir,
		CaptureObservation:    captureScreenObservation,
		GetCursorPositionJSON: getCursorPositionJSON,
		BreakGoalIntoSubtasks: breakGoalIntoSubtasks,
		ReplanSubtasks:        replanSubtasks,
		SendMessageToLLM:      sendMessageToLLM,
		ExecuteActions:        executeSubtaskActions,
		VerifySubtask:         isGoalAchieved,
		VerifyMainGoal:        isMainGoalAchieved,
		UpdateTaskStatus:      UpdateTaskStatus,
		UpdateSubtask:         UpdateSubtask,
		BroadcastUpdate:       BroadcastExecutionEngineUpdate,
		CleanupUserAssist:     CleanupUserAssistMessages,
		GetUserAssist:         GetUserAssistMessage,
	}
}

// Engine orchestrates task planning, subtask execution, callback-driven verification, and replanning.
type Engine struct {
	deps ExecutionDeps
}

// NewEngine constructs a new execution Engine with the provided dependencies.
func NewEngine(deps ExecutionDeps) *Engine {
	return &Engine{deps: deps}
}

// ExecuteTask executes a task using the default production execution engine.
func ExecuteTask(task *Task) {
	NewEngine(defaultExecutionDeps()).Run(task)
}

type executionState struct {
	task              *Task
	goal              string
	artifactsDir      string
	iteration         int64
	nextSubtaskID     int
	previousOCRText   string
	prevActionsJSON   string
	prevCursorPosJSON string
	completedSubtasks []string
	lastObs           *ScreenObservation
	lastReport        CompletionReport
	terminalReported  bool
}

// Run executes the full AGI loop for a task and guarantees a terminal status is reported to the frontend.
func (e *Engine) Run(task *Task) {
	log.Printf("=== EXECUTING TASK %s ===", task.ID)
	log.Printf("Task message: %s (provider: %s, model: %s)", task.Message, task.Provider, task.Model)

	state := &executionState{
		task:          task,
		goal:          task.Message,
		iteration:     1,
		nextSubtaskID: 1,
	}

	defer func() {
		e.deps.CleanupUserAssist(task.ID)
		if r := recover(); r != nil {
			log.Printf("Panic during task %s execution: %v", task.ID, r)
			if !state.terminalReported {
				e.reportFailed(state, fmt.Sprintf("Internal execution error: %v", r))
			}
			return
		}
		if !state.terminalReported {
			if task.Context.Err() != nil {
				e.reportCanceled(state, "at exit")
			} else {
				e.reportFailed(state, "Task ended without reaching a terminal completion state")
			}
		}
	}()

	if dir, err := e.deps.PrepareArtifactsDir(task.ID); err != nil {
		log.Printf("Warning: failed to prepare artifacts dir for task %s: %v", task.ID, err)
	} else {
		state.artifactsDir = dir
		log.Printf("Task artifacts directory: %s", dir)
	}

	if e.isCanceled(state, "before planning") {
		return
	}

	state.prevCursorPosJSON, _ = e.deps.GetCursorPositionJSON()

	subtasks, ok := e.planInitialSubtasks(state)
	if !ok {
		return
	}

	for replanAttempt := 0; replanAttempt <= maxReplanAttempts; replanAttempt++ {
		if e.isCanceled(state, "at start of AGI loop") {
			return
		}

		for _, subtask := range subtasks {
			completed, aborted := e.runSubtask(state, subtask)
			if aborted {
				return
			}
			if !completed {
				if state.iteration > maxTotalIterations {
					e.reportFailed(state, fmt.Sprintf("Task exceeded maximum iteration limit (%d) before achieving main goal", maxTotalIterations))
					return
				}
				// Stop executing stale static subtasks and let the main-goal verifier + replanner adapt.
				break
			}
		}

		if e.isCanceled(state, "before main goal verification") {
			return
		}

		mainAchieved, verdictDesc, nextPrompt, err := e.onTaskReportedComplete(state)
		if err != nil {
			if e.isCanceled(state, "during main goal verification") {
				return
			}
			e.reportFailed(state, fmt.Sprintf("Main goal verification failed: %v", err))
			return
		}

		log.Printf("Main goal verdict (achieved=%v, replanAttempt=%d/%d): %s", mainAchieved, replanAttempt, maxReplanAttempts, verdictDesc)
		if mainAchieved {
			e.reportCompleted(state)
			return
		}

		if state.iteration > maxTotalIterations || replanAttempt >= maxReplanAttempts {
			e.reportFailed(state, fmt.Sprintf("Main goal not achieved after %d replan attempts: %s", replanAttempt, verdictDesc))
			return
		}

		nextSubtasks, ok := e.replan(state, verdictDesc, nextPrompt)
		if !ok {
			return
		}
		subtasks = nextSubtasks
	}
}

func (e *Engine) planInitialSubtasks(state *executionState) ([]SubTask, bool) {
	task := state.task
	subtasks, err := e.deps.BreakGoalIntoSubtasks(task.Context, task.Provider, task.Model, task.ThinkingLevel, state.goal)
	if err != nil {
		if e.isCanceled(state, "during goal breakdown") {
			return nil, false
		}
		log.Printf("Failed to break down goal into subtasks: %v. Falling back to single subtask.", err)
		subtasks = []SubTask{{Id: 1, Description: state.goal}}
	}
	if len(subtasks) == 0 {
		log.Printf("Goal breakdown returned 0 subtasks for task %s; falling back to main goal.", task.ID)
		subtasks = []SubTask{{Id: 1, Description: state.goal}}
	}

	e.registerSubtasks(state, subtasks)
	return subtasks, true
}

func (e *Engine) replan(state *executionState, verdictDesc, nextPrompt string) ([]SubTask, bool) {
	task := state.task
	feedback := verdictDesc
	if strings.TrimSpace(nextPrompt) != "" {
		feedback = strings.TrimSpace(verdictDesc + "\nSuggested next step: " + nextPrompt)
	}

	var ocrJSON, windowsJSON string
	if state.lastObs != nil {
		ocrJSON = state.lastObs.OCRResultsJSON
		windowsJSON = state.lastObs.DetectedWindows
	}

	nextSubtasks, err := e.deps.ReplanSubtasks(
		task.Context,
		task.Provider,
		task.Model,
		task.ThinkingLevel,
		state.goal,
		state.completedSubtasks,
		feedback,
		ocrJSON,
		windowsJSON,
	)
	if err != nil {
		if e.isCanceled(state, "during replanning") {
			return nil, false
		}
		log.Printf("Replanning failed: %v", err)
	}
	if len(nextSubtasks) == 0 {
		fallbackDesc := strings.TrimSpace(nextPrompt)
		if fallbackDesc == "" {
			fallbackDesc = state.goal
		}
		nextSubtasks = []SubTask{{Description: fallbackDesc}}
	}

	e.registerSubtasks(state, nextSubtasks)
	return nextSubtasks, true
}

func (e *Engine) registerSubtasks(state *executionState, subtasks []SubTask) {
	for i := range subtasks {
		subtasks[i].Id = state.nextSubtaskID
		state.nextSubtaskID++
		e.deps.UpdateSubtask(state.task.ID, subtasks[i].Id, subtasks[i].Description, false, []actionpkg.Action{})
	}
}

// runSubtask runs a single subtask until it explicitly reports completion (via stopIteration callback)
// and passes LLM verification, or until its iteration budget is exhausted.
func (e *Engine) runSubtask(state *executionState, subtask SubTask) (completed bool, aborted bool) {
	task := state.task
	var (
		nextPrompt string
		promptLog  = []PromptLog{{Iteration: 0, Message: subtask.Description}}
	)

	for subtaskIter := int64(0); subtaskIter < maxSubtaskIterations && state.iteration <= maxTotalIterations; subtaskIter++ {
		if e.isCanceled(state, "at start of subtask iteration") {
			return false, true
		}

		beforeObs, err := e.deps.CaptureObservation(task.Context, task, state.previousOCRText, state.iteration, false)
		if err != nil {
			if e.isCanceled(state, "during pre-action observation") {
				return false, true
			}
			e.reportFailed(state, fmt.Sprintf("Failed to capture screenshot: %v", err))
			return false, true
		}
		state.previousOCRText = beforeObs.OCRResultsJSON

		log.Printf("iteration %d (subtask %d, attempt %d/%d), main goal: %s", state.iteration, subtask.Id, subtaskIter+1, maxSubtaskIterations, state.goal)
		log.Printf("iteration %d, current subtask: %s", state.iteration, subtask.Description)

		userAssist := e.deps.GetUserAssist(task.ID)
		activePrompt := buildSubtaskPrompt(state.goal, subtask.Description, nextPrompt, userAssist)
		historyContext := buildHistoryContext(state.prevActionsJSON, promptLog)

		actions, actionsJSON, llmErr := e.deps.SendMessageToLLM(
			task.Context,
			task.Provider,
			task.Model,
			task.ThinkingLevel,
			state.artifactsDir,
			activePrompt,
			beforeObs.BoundingBoxesJSON,
			beforeObs.OCRResultsJSON,
			beforeObs.TextChangesSummary,
			historyContext,
			state.iteration,
			state.prevCursorPosJSON,
			beforeObs.DetectedWindows,
			beforeObs.X11WindowsJSON,
			beforeObs.ColorsDistribution,
		)

		e.emitSubtaskState(task.ID, subtask, true, actions)

		if llmErr != nil {
			if e.isCanceled(state, "during LLM communication") {
				return false, true
			}
			e.reportFailed(state, "Failed to communicate with LLM")
			return false, true
		}

		// Execute actions and only trigger subtask verification via callback when stopIteration ("st") is issued.
		reportedComplete := false
		batchResult, execErr := e.deps.ExecuteActions(task.Context, task, subtask.Id, actions, func() {
			reportedComplete = true
		})
		if execErr != nil {
			if e.isCanceled(state, "during action execution") {
				return false, true
			}
			log.Printf("Error executing actions for subtask %d: %v", subtask.Id, execErr)
		}
		if batchResult.ReportedComplete {
			reportedComplete = true
		}

		report := CompletionReport{
			TaskID:            task.ID,
			SubtaskID:         subtask.Id,
			SubtaskDesc:       subtask.Description,
			Iteration:         state.iteration,
			ActionsJSON:       actionsJSON,
			ColorsBefore:      beforeObs.ColorsDistribution,
			PrevCursorPosJSON: state.prevCursorPosJSON,
		}
		state.lastReport = report
		state.prevActionsJSON = actionsJSON
		state.iteration++

		// Do NOT verify prematurely if the subtask has not signaled completion yet.
		if !reportedComplete {
			if cursorPos, err := e.deps.GetCursorPositionJSON(); err == nil {
				state.prevCursorPosJSON = cursorPos
			}
			log.Printf("Subtask %d has not reported completion (no stopIteration); continuing execution.", subtask.Id)
			continue
		}

		// Subtask explicitly reported completion -> invoke verification callback.
		verified, verdictDesc, retryPrompt, afterObs, verifyErr := e.onSubtaskReportedComplete(state, report)
		if verifyErr != nil {
			if e.isCanceled(state, "during subtask verification") {
				return false, true
			}
			e.reportFailed(state, fmt.Sprintf("Subtask verification failed: %v", verifyErr))
			return false, true
		}

		state.lastObs = afterObs
		state.previousOCRText = afterObs.OCRResultsJSON
		state.prevCursorPosJSON = afterObs.CursorPositionJSON

		log.Printf("Subtask %d completion verdict (verified=%v): %s", subtask.Id, verified, verdictDesc)
		if verified {
			state.completedSubtasks = append(state.completedSubtasks, subtask.Description)
			e.emitSubtaskState(task.ID, subtask, false, actions)
			return true, false
		}

		// Subtask reported completion prematurely; feed verifier guidance back for the next iteration.
		nextPrompt = retryPrompt
		if strings.TrimSpace(nextPrompt) == "" {
			nextPrompt = verdictDesc
		}
		if strings.TrimSpace(nextPrompt) != "" {
			promptLog = append(promptLog, PromptLog{Iteration: report.Iteration, Message: nextPrompt})
		}
	}

	log.Printf("Subtask %d (%s) did not complete within iteration budget.", subtask.Id, subtask.Description)
	e.emitSubtaskState(task.ID, subtask, false, []actionpkg.Action{})
	return false, false
}

// onSubtaskReportedComplete is the completion callback invoked when a subtask issues stopIteration ("st").
func (e *Engine) onSubtaskReportedComplete(state *executionState, report CompletionReport) (bool, string, string, *ScreenObservation, error) {
	task := state.task
	afterObs, err := e.deps.CaptureObservation(task.Context, task, state.previousOCRText, state.iteration, true)
	if err != nil {
		return false, "", "", nil, err
	}

	verified, verdictDesc, retryPrompt := e.deps.VerifySubtask(
		task.Context,
		task.Provider,
		task.Model,
		task.ThinkingLevel,
		report.SubtaskDesc,
		afterObs.BoundingBoxesJSON,
		afterObs.OCRResultsJSON,
		afterObs.TextChangesJSON,
		afterObs.TextChangesSummary,
		report.ActionsJSON,
		report.Iteration,
		report.PrevCursorPosJSON,
		afterObs.DetectedWindows,
		afterObs.CursorPositionJSON,
		afterObs.OCRNearCursorJSON,
		report.ColorsBefore,
		afterObs.ColorsDistribution,
	)
	if task.Context.Err() != nil {
		return false, "", "", afterObs, task.Context.Err()
	}
	return verified, verdictDesc, retryPrompt, afterObs, nil
}

// onTaskReportedComplete is the completion callback invoked after a subtask batch finishes to judge the main goal.
func (e *Engine) onTaskReportedComplete(state *executionState) (bool, string, string, error) {
	task := state.task
	obs := state.lastObs
	colorsBefore := state.lastReport.ColorsBefore
	if obs == nil {
		var err error
		obs, err = e.deps.CaptureObservation(task.Context, task, state.previousOCRText, state.iteration, true)
		if err != nil {
			return false, "", "", err
		}
		state.lastObs = obs
		colorsBefore = obs.ColorsDistribution
	}

	verified, verdictDesc, nextPrompt := e.deps.VerifyMainGoal(
		task.Context,
		task.Provider,
		task.Model,
		task.ThinkingLevel,
		state.goal,
		state.completedSubtasks,
		obs.BoundingBoxesJSON,
		obs.OCRResultsJSON,
		obs.TextChangesJSON,
		obs.TextChangesSummary,
		state.lastReport.ActionsJSON,
		state.iteration,
		state.lastReport.PrevCursorPosJSON,
		obs.DetectedWindows,
		obs.CursorPositionJSON,
		obs.OCRNearCursorJSON,
		colorsBefore,
		obs.ColorsDistribution,
	)
	if task.Context.Err() != nil {
		return false, "", "", task.Context.Err()
	}
	return verified, verdictDesc, nextPrompt, nil
}

func (e *Engine) emitSubtaskState(taskID string, subtask SubTask, isActive bool, actions []actionpkg.Action) {
	e.deps.UpdateSubtask(taskID, subtask.Id, subtask.Description, isActive, actions)
	e.deps.BroadcastUpdate("subtaskUpdate", map[string]interface{}{
		"taskId":      taskID,
		"subtaskId":   subtask.Id,
		"description": subtask.Description,
		"isActive":    isActive,
	})
}

func (e *Engine) reportCompleted(state *executionState) {
	log.Println("GOAL ACHIEVED! breaking AGIloop...")
	state.terminalReported = true
	e.deps.UpdateTaskStatus(state.task.ID, "completed", state.task.Message)
	e.deps.BroadcastUpdate("completionEvent", map[string]interface{}{
		"taskId": state.task.ID,
		"event":  "completed",
	})
}

func (e *Engine) reportFailed(state *executionState, reason string) {
	log.Printf("Task %s failed: %s", state.task.ID, reason)
	state.terminalReported = true
	e.deps.UpdateTaskStatus(state.task.ID, "broken", reason)
	e.deps.BroadcastUpdate("completionEvent", map[string]interface{}{
		"taskId": state.task.ID,
		"event":  "broken",
	})
}

func (e *Engine) reportCanceled(state *executionState, stage string) {
	log.Printf("Task %s canceled %s", state.task.ID, stage)
	state.terminalReported = true
	e.deps.UpdateTaskStatus(state.task.ID, "canceled", "Task canceled by user")
	e.deps.BroadcastUpdate("completionEvent", map[string]interface{}{
		"taskId": state.task.ID,
		"event":  "canceled",
	})
}

func (e *Engine) isCanceled(state *executionState, stage string) bool {
	select {
	case <-state.task.Context.Done():
		if !state.terminalReported {
			e.reportCanceled(state, stage)
		}
		return true
	default:
		return false
	}
}

func buildSubtaskPrompt(mainGoal, subtaskDesc, nextPrompt string, userAssist *UserAssistMessage) string {
	var b strings.Builder
	b.WriteString(subtaskDesc)
	if strings.TrimSpace(mainGoal) != "" && strings.TrimSpace(mainGoal) != strings.TrimSpace(subtaskDesc) {
		b.WriteString("\n\nOVERALL MAIN GOAL: ")
		b.WriteString(mainGoal)
	}
	if strings.TrimSpace(nextPrompt) != "" {
		b.WriteString("\n\nPREVIOUS ATTEMPT FEEDBACK / NEXT STEP: ")
		b.WriteString(nextPrompt)
	}
	if userAssist != nil && strings.TrimSpace(userAssist.Message) != "" {
		b.WriteString("\n\nHELPER MESSAGE FROM THE USER: ")
		b.WriteString(userAssist.Message)
	}
	return b.String()
}

func buildHistoryContext(prevActionsJSON string, promptLog []PromptLog) string {
	if strings.TrimSpace(prevActionsJSON) != "" {
		return prevActionsJSON
	}
	promptLogBytes, err := json.Marshal(promptLog)
	if err != nil {
		return "[]"
	}
	return string(promptLogBytes)
}

func captureScreenObservation(ctx context.Context, task *Task, previousOCR string, iteration int64, includeCursorOCR bool) (*ScreenObservation, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	screenshotImg, err := screenshot.CaptureX11Screenshot()
	if err != nil {
		return nil, err
	}

	colorsDist := imagepkg.DominantColorsToJSONString(imagepkg.DominantColors(screenshotImg, 10))
	grayscale := screenshot.ConvertToGrayscale(screenshotImg)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	ocrResults := ocr.OCR(grayscale)

	x11WindowsData, x11Err := getX11WindowsData()
	detectedWindowsJSON := x11WindowsData
	if x11Err != nil {
		log.Printf("Failed to get X11 windows data, falling back to visual window detection: %v", x11Err)
		x11WindowsData = "[]"
		var detectedWindows []string
		for _, ocrElement := range ocrResults {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			ocrElementBB := image.Rect(ocrElement.BoundingBox.XMin, ocrElement.BoundingBox.YMin, ocrElement.BoundingBox.XMax, ocrElement.BoundingBox.YMax)
			windowsJSONString, detectErr := vision.DetectWindow(grayscale, ocrElementBB, ocrElement.Text)
			if detectErr != nil {
				continue
			}
			detectedWindows = append(detectedWindows, windowsJSONString)
		}
		detectedWindowsJSON = "[" + strings.Join(detectedWindows, ",") + "]"
	}

	ocrResultsJSON := ocr.OCRtoJSONString(ocrResults)
	if len(ocrResultsJSON) > 10000 {
		ocrDataMerged := ocr.MergeCloseText(ocrResults, 20, 40)
		ocrResultsJSON = ocr.OCRtoJSONString(ocrDataMerged)
	}

	var textChangesJSON, textChangesSummary string
	if previousOCR != "" {
		textChanges, deltaErr := ocr.GetOCRDelta(previousOCR, ocrResultsJSON)
		if deltaErr != nil {
			log.Printf("Failed to get OCR Delta[iteration: %d]: %v", iteration, deltaErr)
		} else {
			textChangesJSON, deltaErr = ocr.GetOCRDeltaJSONString(textChanges)
			if deltaErr != nil {
				log.Printf("Failed to get OCR Delta JSON string[iteration: %d]: %v", iteration, deltaErr)
			} else if ctx.Err() == nil {
				textChangesSummary, deltaErr = getOCRDeltaAbstractDescription(ctx, task.Provider, task.Model, task.ThinkingLevel, textChangesJSON)
				if deltaErr != nil {
					log.Printf("Failed to get OCR Delta abstract description[iteration: %d]: %v", iteration, deltaErr)
				}
			}
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	boundingBoxesJSON := boundingBoxArrayToJSONString(findBoundingBoxes(screenshotImg))
	cursorPosJSON, _ := getCursorPositionJSON()

	var ocrNearCursor string
	if includeCursorOCR {
		_, cursorY := getCursorPosition()
		rect := image.Rect(0, max(0, cursorY-23), grayscale.Bounds().Max.X, min(cursorY+23, grayscale.Bounds().Max.Y))
		imgUnderCursor := image.NewGray(rect)
		draw.Draw(imgUnderCursor, imgUnderCursor.Bounds(), grayscale, rect.Min, draw.Src)
		ocrNearCursor = ocr.OCRtoJSONString(ocr.OCR(imgUnderCursor))
	}

	return &ScreenObservation{
		Screenshot:         screenshotImg,
		Grayscale:          grayscale,
		ColorsDistribution: colorsDist,
		X11WindowsJSON:     x11WindowsData,
		DetectedWindows:    detectedWindowsJSON,
		OCRResultsJSON:     ocrResultsJSON,
		TextChangesJSON:    textChangesJSON,
		TextChangesSummary: textChangesSummary,
		BoundingBoxesJSON:  boundingBoxesJSON,
		CursorPositionJSON: cursorPosJSON,
		OCRNearCursorJSON:  ocrNearCursor,
	}, nil
}

func executeSubtaskActions(ctx context.Context, task *Task, subtaskID int, actions []actionpkg.Action, onComplete func()) (ActionBatchResult, error) {
	actionExecutor := actionpkg.DefaultExecutor()
	execCtx := &actionpkg.ExecutionContext{
		Actions:  actions,
		Executor: actionExecutor,
	}

	for i, act := range actions {
		if ctx.Err() != nil {
			return ActionBatchResult{}, ctx.Err()
		}

		UpdateAction(task.ID, subtaskID, i, act)

		actionData := actionpkg.ToVerboseMap(act)
		actionData["actionId"] = i
		BroadcastExecutionEngineUpdate("actionUpdate", map[string]interface{}{
			"taskId":      task.ID,
			"subtaskId":   subtaskID,
			"actionIndex": i,
			"action":      actionData,
		})

		log.Printf("Executing action %d: %s", i, actionpkg.FormatVerbose(act))

		execCtx.Index = i
		res, execErr := actionExecutor.Execute(execCtx, &actions[i])
		if execErr != nil {
			log.Printf("Error executing action %s: %v", actionpkg.FormatVerbose(act), execErr)
		}
		if res.StopIteration {
			if onComplete != nil {
				onComplete()
			}
			return ActionBatchResult{ReportedComplete: true}, nil
		}
	}
	return ActionBatchResult{ReportedComplete: false}, nil
}

// Helper adapters to external packages
func getCursorPositionJSON() (string, error) {
	return mouse.GetCursorPositionJSON()
}

func getCursorPosition() (int, int) {
	return mouse.GetCursorPosition()
}

func breakGoalIntoSubtasks(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error) {
	llmSubtasks, err := llm.BreakGoalIntoSubtasks(ctx, provider, model, thinkingLevel, goal, token.AddTokensAndSendUpdate)
	if err != nil {
		return nil, err
	}

	subtasks := make([]SubTask, len(llmSubtasks))
	for i, subtask := range llmSubtasks {
		subtasks[i] = SubTask{
			Id:          subtask.Id,
			Description: subtask.Description,
		}
	}
	return subtasks, nil
}

func replanSubtasks(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, verifierFeedback, ocrJSON, windowsJSON string) ([]SubTask, error) {
	llmSubtasks, err := llm.ReplanSubtasks(ctx, provider, model, thinkingLevel, mainGoal, completedSubtasks, verifierFeedback, ocrJSON, windowsJSON, token.AddTokensAndSendUpdate)
	if err != nil {
		return nil, err
	}

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
	display := *config.Display
	x11WindowsJSON, err := x11.GetX11WindowsWithDisplay(display)
	if err != nil {
		return "[]", err
	}
	return x11WindowsJSON, nil
}

func findBoundingBoxes(img image.Image) []imagepkg.BoundingBox {
	return imagepkg.FindBoundingBoxes(img)
}

func boundingBoxArrayToJSONString(bbArray []imagepkg.BoundingBox) string {
	return imagepkg.BoundingBoxArrayToJSONString(bbArray)
}

func sendMessageToLLM(ctx context.Context, provider, model, thinkingLevel, artifactsDir, prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, x11WindowsData string, colorsDistribution string) ([]actionpkg.Action, string, error) {
	return llm.SendMessageToLLM(ctx, provider, model, thinkingLevel, artifactsDir, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands, iteration, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution)
}

func getOCRDeltaAbstractDescription(ctx context.Context, provider, model, thinkingLevel, ocrDelta string) (string, error) {
	return llm.GetOCRDeltaAbstractDescription(ctx, provider, model, thinkingLevel, ocrDelta, token.AddTokensAndSendUpdate)
}

func isGoalAchieved(ctx context.Context, provider, model, thinkingLevel, goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string) (bool, string, string) {
	return llm.IsGoalAchieved(ctx, provider, model, thinkingLevel, goal, bboxes, ocrJSONString, ocrDelta, ocrDeltaAbstract, prevActionsJSONString, iteration, prevCursorPositionJSONString, allWindowsJSONString, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeAction, colorsDistribution, token.AddTokensAndSendUpdate)
}

func isMainGoalAchieved(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string) (bool, string, string) {
	return llm.IsMainGoalAchieved(ctx, provider, model, thinkingLevel, mainGoal, completedSubtasks, bboxes, ocrJSONString, ocrDelta, ocrDeltaAbstract, prevActionsJSONString, iteration, prevCursorPositionJSONString, allWindowsJSONString, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeAction, colorsDistribution, token.AddTokensAndSendUpdate)
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
