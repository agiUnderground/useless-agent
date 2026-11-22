package task

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	actionpkg "useless-agent/internal/action"
	"useless-agent/internal/websocket"
)

// Task status constants.
const (
	StatusQueued     = "in-the-queue"
	StatusInProgress = "in-progress"
	StatusCompleted  = "completed"
	StatusBroken     = "broken"
	StatusCanceled   = "canceled"
)

// Unified task & queue state protected by managerMutex (eliminates lock-order deadlocks).
var (
	managerMutex  sync.Mutex
	tasks         = make(map[string]*Task)
	taskIDCounter int
	taskQueue     = make([]*Task, 0)
	runningTask   *Task

	// taskRunner is the function used to execute a dequeued task (overridable in tests).
	taskRunner = ExecuteTask
)

// User-assist message management globals
var (
	userAssistMessages = make(map[string]*UserAssistMessage)
	userAssistMutex    sync.RWMutex
)

func isTerminalStatus(status string) bool {
	return status == StatusCompleted || status == StatusBroken || status == StatusCanceled
}

func isValidTransition(from, to string) bool {
	if from == to {
		return true
	}
	if isTerminalStatus(from) {
		return false
	}
	if from == StatusInProgress && to == StatusQueued {
		return false
	}
	return true
}

// CreateTask creates a new task in "in-the-queue" status and notifies clients in order.
func CreateTask(message, provider, model, thinkingLevel, sessionID string) *Task {
	ctx, cancelFunc := CreateContext()

	managerMutex.Lock()
	taskIDCounter++
	taskID := fmt.Sprintf("task-%d-%d", taskIDCounter, time.Now().Unix())

	t := &Task{
		ID:            taskID,
		Status:        StatusQueued,
		Message:       message,
		Provider:      provider,
		Model:         model,
		ThinkingLevel: thinkingLevel,
		SessionID:     sessionID,
		CreatedAt:     time.Now(),
		Context:       ctx,
		CancelFunc:    cancelFunc,
	}
	tasks[taskID] = t
	snapshot := *t
	managerMutex.Unlock()

	// Emit initial queued status synchronously before EnqueueTask can transition to in-progress.
	SendTaskUpdate(&snapshot)
	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  snapshot.ID,
		"status":  snapshot.Status,
		"message": snapshot.Message,
		"model":   snapshot.Model,
	})

	return t
}

// UpdateTaskStatus updates the status of a task if the transition is valid and notifies clients.
func UpdateTaskStatus(taskID, status, message string) {
	managerMutex.Lock()
	t, exists := tasks[taskID]
	if !exists {
		managerMutex.Unlock()
		return
	}
	if !isValidTransition(t.Status, status) {
		log.Printf("Ignoring invalid status transition for task %s: %s -> %s", taskID, t.Status, status)
		managerMutex.Unlock()
		return
	}
	t.Status = status
	if message != "" {
		t.Message = message
	}
	snapshot := *t
	managerMutex.Unlock()

	SendTaskUpdate(&snapshot)
	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  snapshot.ID,
		"status":  snapshot.Status,
		"message": snapshot.Message,
		"model":   snapshot.Model,
	})
}

// GetTask retrieves a task by ID.
func GetTask(taskID string) (*Task, bool) {
	managerMutex.Lock()
	defer managerMutex.Unlock()

	t, exists := tasks[taskID]
	return t, exists
}

// CancelTask cancels a running or queued task.
func CancelTask(taskID string) bool {
	managerMutex.Lock()
	t, exists := tasks[taskID]
	if !exists {
		managerMutex.Unlock()
		return false
	}

	if t.Status != StatusInProgress && t.Status != StatusQueued {
		managerMutex.Unlock()
		return false
	}

	if t.Status == StatusQueued {
		for i, queuedTask := range taskQueue {
			if queuedTask.ID == taskID {
				taskQueue = append(taskQueue[:i], taskQueue[i+1:]...)
				break
			}
		}
	}

	cancelFn := t.CancelFunc
	t.Status = StatusCanceled
	snapshot := *t
	managerMutex.Unlock()

	if cancelFn != nil {
		cancelFn()
	}

	SendTaskUpdate(&snapshot)
	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  snapshot.ID,
		"status":  StatusCanceled,
		"message": "Task canceled by user",
		"model":   snapshot.Model,
	})

	return true
}

// EnqueueTask adds a task to the queue and starts processing immediately if idle.
func EnqueueTask(t *Task) {
	managerMutex.Lock()
	if isTerminalStatus(t.Status) {
		managerMutex.Unlock()
		return
	}
	taskQueue = append(taskQueue, t)
	log.Printf("Task %s enqueued (queue length: %d)", t.ID, len(taskQueue))
	shouldStart := runningTask == nil
	managerMutex.Unlock()

	if shouldStart {
		ProcessNextTask()
	}
}

// DequeueNextTask gets the next task from the queue.
func DequeueNextTask() *Task {
	managerMutex.Lock()
	defer managerMutex.Unlock()

	if len(taskQueue) == 0 {
		return nil
	}

	t := taskQueue[0]
	taskQueue = taskQueue[1:]
	log.Printf("Task %s dequeued (remaining: %d)", t.ID, len(taskQueue))
	return t
}

// GetQueueLength returns the current queue length.
func GetQueueLength() int {
	managerMutex.Lock()
	defer managerMutex.Unlock()
	return len(taskQueue)
}

// IsTaskRunning checks if a task is currently running.
func IsTaskRunning() bool {
	managerMutex.Lock()
	defer managerMutex.Unlock()
	return runningTask != nil
}

// ProcessNextTask dequeues the next non-canceled task, transitions it to in-progress, and runs it.
func ProcessNextTask() {
	managerMutex.Lock()
	if runningTask != nil {
		managerMutex.Unlock()
		return
	}

	var next *Task
	for len(taskQueue) > 0 {
		candidate := taskQueue[0]
		taskQueue = taskQueue[1:]
		if isTerminalStatus(candidate.Status) {
			log.Printf("Skipping already-terminal task %s (status: %s)", candidate.ID, candidate.Status)
			continue
		}
		next = candidate
		break
	}

	if next == nil {
		managerMutex.Unlock()
		return
	}

	runningTask = next
	next.Status = StatusInProgress
	snapshot := *next
	runner := taskRunner
	log.Printf("Task %s dequeued and starting (remaining in queue: %d)", next.ID, len(taskQueue))
	managerMutex.Unlock()

	// Emit in-progress status synchronously BEFORE launching the execution goroutine.
	SendTaskUpdate(&snapshot)
	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  snapshot.ID,
		"status":  snapshot.Status,
		"message": snapshot.Message,
		"model":   snapshot.Model,
	})

	go func(taskRef *Task) {
		log.Printf("=== GOROUTINE STARTED for task %s ===", taskRef.ID)
		defer func() {
			log.Printf("=== GOROUTINE ENDING for task %s ===", taskRef.ID)
			managerMutex.Lock()
			if runningTask == taskRef {
				runningTask = nil
			}
			hasMore := len(taskQueue) > 0
			managerMutex.Unlock()

			if hasMore {
				ProcessNextTask()
			}
		}()

		runner(taskRef)
	}(next)
}

// CreateContext creates a new context with cancellation.
func CreateContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// SendTaskUpdate sends a task update via WebSocket.
func SendTaskUpdate(t *Task) {
	websocket.SendTaskUpdate(t.ID, t.Status, t.Message, t.Model, t.ThinkingLevel)
}

// AddUserAssistMessage adds a user-assist message for a task.
func AddUserAssistMessage(taskID, message string) bool {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	managerMutex.Lock()
	t, exists := tasks[taskID]
	status := ""
	if exists {
		status = t.Status
	}
	managerMutex.Unlock()

	if !exists {
		log.Printf("Task %s not found, cannot add user-assist message", taskID)
		return false
	}

	if status != StatusInProgress {
		log.Printf("Task %s is not in progress (status: %s), ignoring user-assist message", taskID, status)
		return false
	}

	if existingMsg, exists := userAssistMessages[taskID]; exists && !existingMsg.Injected {
		log.Printf("Task %s already has a pending user-assist message, replacing it", taskID)
	}

	userAssistMessages[taskID] = &UserAssistMessage{
		TaskID:    taskID,
		Message:   message,
		CreatedAt: time.Now(),
		Injected:  false,
	}

	log.Printf("Added user-assist message for task %s: %s", taskID, message)
	return true
}

// GetUserAssistMessage gets the next user-assist message for a task and marks it as injected.
func GetUserAssistMessage(taskID string) *UserAssistMessage {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	msg, exists := userAssistMessages[taskID]
	if !exists || msg.Injected {
		return nil
	}

	msg.Injected = true
	log.Printf("Retrieved and marked user-assist message as injected for task %s: %s", taskID, msg.Message)
	return msg
}

// CleanupUserAssistMessages removes user-assist messages for completed/canceled tasks.
func CleanupUserAssistMessages(taskID string) {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	delete(userAssistMessages, taskID)
	log.Printf("Cleaned up user-assist messages for task %s", taskID)
}

// GetExecutionState returns the current execution engine state.
func GetExecutionState() *ExecutionState {
	managerMutex.Lock()
	defer managerMutex.Unlock()

	taskList := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if t != nil {
			taskList = append(taskList, *t)
		}
	}

	var runningTaskID string
	if runningTask != nil {
		runningTaskID = runningTask.ID
	}

	queuedTasks := make([]string, 0, len(taskQueue))
	for _, queuedTask := range taskQueue {
		queuedTasks = append(queuedTasks, queuedTask.ID)
	}

	return &ExecutionState{
		Tasks:       taskList,
		RunningTask: runningTaskID,
		QueuedTasks: queuedTasks,
	}
}

// UpdateSubtask sends a subtask update via WebSocket.
func UpdateSubtask(taskID string, subtaskID int, description string, isActive bool, actions []actionpkg.Action) {
	serializableActions := make([]interface{}, len(actions))
	for i, act := range actions {
		serializableActions[i] = actionpkg.ToVerboseMap(act)
	}

	websocket.SendSubtaskUpdate(taskID, subtaskID, description, isActive, serializableActions)
}

// UpdateAction sends an action update via WebSocket.
func UpdateAction(taskID string, subtaskID int, actionIndex int, act actionpkg.Action) {
	websocket.SendActionUpdate(taskID, subtaskID, actionIndex, actionpkg.ToVerboseMap(act))
}

// BroadcastExecutionEngineUpdate broadcasts execution engine updates via WebSocket.
func BroadcastExecutionEngineUpdate(updateType string, data interface{}) {
	websocket.SendExecutionEngineUpdate(updateType, data)
}
