package task

import (
	"context"
	"strings"
	"testing"

	actionpkg "useless-agent/internal/action"
)

func newTestTask(goal string) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	return &Task{
		ID:         "task-test-1",
		Status:     "in-progress",
		Message:    goal,
		Provider:   "zai",
		Model:      "glm-5.3",
		Context:    ctx,
		CancelFunc: cancel,
	}
}

func newBaseTestDeps(statuses *[]string, completionEvents *[]string) ExecutionDeps {
	return ExecutionDeps{
		PrepareArtifactsDir: func(taskID string) (string, error) { return "/tmp/test-artifacts", nil },
		CaptureObservation: func(ctx context.Context, task *Task, previousOCR string, iteration int64, includeCursorOCR bool) (*ScreenObservation, error) {
			return &ScreenObservation{
				ColorsDistribution: `[]`,
				X11WindowsJSON:     `[]`,
				DetectedWindows:    `[]`,
				OCRResultsJSON:     `[]`,
				BoundingBoxesJSON:  `[]`,
				CursorPositionJSON: `{"x":100,"y":100}`,
			}, nil
		},
		GetCursorPositionJSON: func() (string, error) { return `{"x":100,"y":100}`, nil },
		SendMessageToLLM: func(ctx context.Context, provider, model, thinkingLevel, artifactsDir, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands string, iteration int64, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution string) ([]actionpkg.Action, string, error) {
			return []actionpkg.Action{{Seq: 1, Code: actionpkg.CodeStopIteration}}, `[{"i":1,"a":"st"}]`, nil
		},
		ExecuteActions: func(ctx context.Context, task *Task, subtaskID int, actions []actionpkg.Action, onComplete func()) (ActionBatchResult, error) {
			for _, a := range actions {
				if a.Code == actionpkg.CodeStopIteration {
					if onComplete != nil {
						onComplete()
					}
					return ActionBatchResult{ReportedComplete: true}, nil
				}
			}
			return ActionBatchResult{ReportedComplete: false}, nil
		},
		UpdateTaskStatus: func(taskID, status, message string) {
			*statuses = append(*statuses, status)
		},
		UpdateSubtask: func(taskID string, subtaskID int, description string, isActive bool, actions []actionpkg.Action) {},
		BroadcastUpdate: func(updateType string, data interface{}) {
			if updateType == "completionEvent" {
				if m, ok := data.(map[string]interface{}); ok {
					if ev, ok := m["event"].(string); ok {
						*completionEvents = append(*completionEvents, ev)
					}
				}
			}
		},
		CleanupUserAssist: func(taskID string) {},
		GetUserAssist:     func(taskID string) *UserAssistMessage { return nil },
	}
}

func TestEngine_VerifiesOnlyAfterSubtaskReportsCompletionCallback(t *testing.T) {
	task := newTestTask("open terminal")
	var statuses, events []string
	deps := newBaseTestDeps(&statuses, &events)

	deps.BreakGoalIntoSubtasks = func(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error) {
		return []SubTask{{Id: 1, Description: "click terminal icon"}}, nil
	}

	llmCalls := 0
	deps.SendMessageToLLM = func(ctx context.Context, provider, model, thinkingLevel, artifactsDir, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands string, iteration int64, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution string) ([]actionpkg.Action, string, error) {
		llmCalls++
		if llmCalls == 1 {
			// First batch: move + click, NO stopIteration yet.
			return []actionpkg.Action{
				{Seq: 1, Code: actionpkg.CodeMouseMove, X: 50, Y: 50},
				{Seq: 2, Code: actionpkg.CodeMouseClickLeft},
			}, `[{"i":1,"a":"mm","x":50,"y":50},{"i":2,"a":"cl"}]`, nil
		}
		// Second batch: subtask confirms completion with stopIteration.
		return []actionpkg.Action{{Seq: 1, Code: actionpkg.CodeStopIteration}}, `[{"i":1,"a":"st"}]`, nil
	}

	subtaskVerifyCalls := 0
	deps.VerifySubtask = func(ctx context.Context, provider, model, thinkingLevel, subtaskGoal, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		subtaskVerifyCalls++
		return true, "terminal opened", ""
	}

	deps.VerifyMainGoal = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		return true, "terminal is open", ""
	}

	NewEngine(deps).Run(task)

	if llmCalls != 2 {
		t.Fatalf("expected 2 LLM calls (work + stopIteration), got %d", llmCalls)
	}
	if subtaskVerifyCalls != 1 {
		t.Fatalf("expected VerifySubtask to run ONLY once after stopIteration callback, got %d", subtaskVerifyCalls)
	}
	if len(statuses) == 0 || statuses[len(statuses)-1] != "completed" {
		t.Fatalf("expected final status 'completed', got %v", statuses)
	}
	if len(events) == 0 || events[len(events)-1] != "completed" {
		t.Fatalf("expected completionEvent 'completed', got %v", events)
	}
}

func TestEngine_DoesNotCompletePrematurelyAfterOneSubtask_ReplansToMainGoal(t *testing.T) {
	task := newTestTask("open firefox and navigate to example.com")
	var statuses, events []string
	deps := newBaseTestDeps(&statuses, &events)

	deps.BreakGoalIntoSubtasks = func(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error) {
		return []SubTask{{Id: 1, Description: "click Applications menu"}}, nil
	}

	var executedSubtasks []string
	deps.VerifySubtask = func(ctx context.Context, provider, model, thinkingLevel, subtaskGoal, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		executedSubtasks = append(executedSubtasks, subtaskGoal)
		return true, "subtask done", ""
	}

	mainVerifyCalls := 0
	deps.VerifyMainGoal = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		mainVerifyCalls++
		if mainVerifyCalls == 1 {
			return false, "Applications menu is open, but Firefox is not open yet", "click Web Browser"
		}
		return true, "Firefox is open at example.com", ""
	}

	replanCalls := 0
	deps.ReplanSubtasks = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, verifierFeedback, ocrJSON, windowsJSON string) ([]SubTask, error) {
		replanCalls++
		return []SubTask{
			{Id: 1, Description: "click Web Browser"},
			{Id: 2, Description: "type example.com and press enter"},
		}, nil
	}

	NewEngine(deps).Run(task)

	if mainVerifyCalls != 2 {
		t.Fatalf("expected 2 main goal verification calls, got %d", mainVerifyCalls)
	}
	if replanCalls != 1 {
		t.Fatalf("expected 1 replan call, got %d", replanCalls)
	}
	if len(executedSubtasks) != 3 {
		t.Fatalf("expected 3 subtasks executed, got %d: %v", len(executedSubtasks), executedSubtasks)
	}
	if len(statuses) == 0 || statuses[len(statuses)-1] != "completed" {
		t.Fatalf("expected final status 'completed', got %v", statuses)
	}
}

func TestEngine_PropagatesNextPromptWhenPrematureStopIterationRejected(t *testing.T) {
	task := newTestTask("open terminal")
	var statuses, events []string
	deps := newBaseTestDeps(&statuses, &events)

	deps.BreakGoalIntoSubtasks = func(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error) {
		return []SubTask{{Id: 1, Description: "click terminal icon"}}, nil
	}

	var promptsSeen []string
	deps.SendMessageToLLM = func(ctx context.Context, provider, model, thinkingLevel, artifactsDir, prompt, bboxes, ocrContext, ocrDelta, prevExecutedCommands string, iteration int64, prevCursorPosJSONString, allWindowsJSONString, x11WindowsData, colorsDistribution string) ([]actionpkg.Action, string, error) {
		promptsSeen = append(promptsSeen, prompt)
		return []actionpkg.Action{{Seq: 1, Code: actionpkg.CodeStopIteration}}, `[{"i":1,"a":"st"}]`, nil
	}

	subtaskCalls := 0
	deps.VerifySubtask = func(ctx context.Context, provider, model, thinkingLevel, subtaskGoal, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		subtaskCalls++
		if subtaskCalls == 1 {
			return false, "missed the icon", "click slightly lower at y=1050"
		}
		return true, "terminal opened", ""
	}

	deps.VerifyMainGoal = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		return true, "terminal is open", ""
	}

	NewEngine(deps).Run(task)

	if len(promptsSeen) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(promptsSeen))
	}
	if !strings.Contains(promptsSeen[1], "click slightly lower at y=1050") {
		t.Fatalf("expected second prompt to include retry feedback, got: %s", promptsSeen[1])
	}
}

func TestEngine_FailsBrokenWhenMainGoalNeverAchieved(t *testing.T) {
	task := newTestTask("impossible goal")
	var statuses, events []string
	deps := newBaseTestDeps(&statuses, &events)

	deps.BreakGoalIntoSubtasks = func(ctx context.Context, provider, model, thinkingLevel, goal string) ([]SubTask, error) {
		return []SubTask{{Id: 1, Description: "step 1"}}, nil
	}
	deps.VerifySubtask = func(ctx context.Context, provider, model, thinkingLevel, subtaskGoal, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		return true, "step done", ""
	}
	deps.VerifyMainGoal = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, bboxes, ocrJSON, ocrDelta, ocrDeltaAbstract, prevActionsJSON string, iteration int64, prevCursorPos, allWindowsJSON, currentCursorPos, ocrNearCursor, colorsBefore, colorsAfter string) (bool, string, string) {
		return false, "still not achieved", "try again"
	}
	deps.ReplanSubtasks = func(ctx context.Context, provider, model, thinkingLevel, mainGoal string, completedSubtasks []string, verifierFeedback, ocrJSON, windowsJSON string) ([]SubTask, error) {
		return []SubTask{{Id: 1, Description: "retry step"}}, nil
	}

	NewEngine(deps).Run(task)

	if len(statuses) == 0 || statuses[len(statuses)-1] != "broken" {
		t.Fatalf("expected final status 'broken', got %v", statuses)
	}
	if len(events) == 0 || events[len(events)-1] != "broken" {
		t.Fatalf("expected completionEvent 'broken', got %v", events)
	}
}
