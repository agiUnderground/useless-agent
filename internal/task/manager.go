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

// Task management globals
var (
	tasks         = make(map[string]*Task)
	taskMutex     sync.Mutex
	taskIDCounter int
)

// Queue management globals
var (
	taskQueue   = make([]*Task, 0) // Simple queue of tasks
	queueMutex  sync.RWMutex
	runningTask *Task // Currently running task
	queueBusy   bool  // Flag to prevent concurrent queue processing
)

// User-assist message management globals
var (
	userAssistMessages = make(map[string]*UserAssistMessage) // Map of task ID to user-assist message
	userAssistMutex    sync.RWMutex
)

// CreateTask creates a new task
func CreateTask(message string) *Task {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	taskIDCounter++
	taskID := fmt.Sprintf("task-%d-%d", taskIDCounter, time.Now().Unix())

	// Create context with cancellation
	ctx, cancelFunc := CreateContext()

	task := &Task{
		ID:         taskID,
		Status:     "in-the-queue", // Tasks start in queue
		Message:    message,
		CreatedAt:  time.Now(),
		Context:    ctx,
		CancelFunc: cancelFunc,
	}

	tasks[taskID] = task

	// Send execution engine update for task creation
	BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
		"taskId":  taskID,
		"status":  task.Status,
		"message": task.Message,
	})

	return task
}

// UpdateTaskStatus updates the status of a task
func UpdateTaskStatus(taskID, status, message string) {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	if task, exists := tasks[taskID]; exists {
		task.Status = status
		if message != "" {
			task.Message = message
		}
		SendTaskUpdate(task)

		// CRITICAL FIX: Send execution engine update for status changes
		// This ensures broken state and other status changes are reported to execution engine
		BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
			"taskId":  taskID,
			"status":  status,
			"message": message,
		})
	}
}

// GetTask retrieves a task by ID
func GetTask(taskID string) (*Task, bool) {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	task, exists := tasks[taskID]
	return task, exists
}

// CancelTask cancels a task
func CancelTask(taskID string) bool {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	if task, exists := tasks[taskID]; exists {
		if task.Status == "in-progress" {
			// Immediate cancellation using context
			if task.CancelFunc != nil {
				task.CancelFunc() // This cancels the context immediately
			}

			task.Status = "canceled"
			SendTaskUpdate(task)

			// CRITICAL FIX: Send execution engine update for task completion
			// This ensures pointer is updated when task is canceled
			BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
				"taskId":  taskID,
				"status":  "canceled",
				"message": "Task canceled by user",
			})

			return true
		} else if task.Status == "in-the-queue" {
			// For queued tasks, we can Cancel them immediately by removing from queue
			// Remove from queue if it's there
			queueMutex.Lock()
			for i, queuedTask := range taskQueue {
				if queuedTask.ID == taskID {
					// Remove from queue slice
					taskQueue = append(taskQueue[:i], taskQueue[i+1:]...)
					break
				}
			}
			queueMutex.Unlock()

			// Also cancel the context
			if task.CancelFunc != nil {
				task.CancelFunc()
			}

			task.Status = "canceled"
			SendTaskUpdate(task)

			// CRITICAL FIX: Send execution engine update for queued task cancellation
			// This ensures execution engine squares are updated when queued task is canceled
			BroadcastExecutionEngineUpdate("taskUpdate", map[string]interface{}{
				"taskId":  taskID,
				"status":  "canceled",
				"message": "Task canceled by user",
			})

			return true
		}
	}
	return false
}

// EnqueueTask adds a task to the queue
func EnqueueTask(task *Task) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	// Add task to the global queue
	taskQueue = append(taskQueue, task)
	log.Printf("Task %s enqueued (queue length: %d)", task.ID, len(taskQueue))

	// Start processing if no task is currently running
	if runningTask == nil && !queueBusy {
		go ProcessNextTask()
	}
}

// DequeueNextTask gets the next task from the queue
func DequeueNextTask() *Task {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	if len(taskQueue) == 0 {
		return nil
	}

	// Get the first task (FIFO)
	task := taskQueue[0]
	taskQueue = taskQueue[1:]

	log.Printf("Task %s dequeued (remaining: %d)", task.ID, len(taskQueue))
	return task
}

// GetQueueLength returns the current queue length
func GetQueueLength() int {
	queueMutex.RLock()
	defer queueMutex.RUnlock()
	return len(taskQueue)
}

// IsTaskRunning checks if a task is currently running
func IsTaskRunning() bool {
	queueMutex.RLock()
	defer queueMutex.RUnlock()
	return runningTask != nil
}

// ProcessNextTask processes the next task in the queue
func ProcessNextTask() {
	log.Printf("=== PROCESS NEXT TASK ===")

	queueMutex.Lock()

	// Prevent concurrent queue processing
	if queueBusy {
		log.Printf("Queue already busy, exiting")
		queueMutex.Unlock()
		return
	}

	// Set busy flag
	queueBusy = true

	// Check if there's already a task running
	if runningTask != nil {
		log.Printf("Task %s already running", runningTask.ID)
		queueBusy = false
		queueMutex.Unlock()
		return
	}

	// Get the next task from queue (inline to avoid deadlock)
	var task *Task
	if len(taskQueue) == 0 {
		log.Printf("No tasks in queue")
		queueBusy = false
		queueMutex.Unlock()
		return
	}

	// Get the first task (FIFO)
	task = taskQueue[0]
	taskQueue = taskQueue[1:]
	log.Printf("Task %s dequeued (remaining: %d)", task.ID, len(taskQueue))

	// Mark this task as running
	runningTask = task

	log.Printf("Starting task %s", task.ID)

	// Update task status to in-progress
	UpdateTaskStatus(task.ID, "in-progress", task.Message)

	// Store task reference before unlocking
	taskRef := task

	queueMutex.Unlock()

	// Execute the task - launch goroutine outside of mutex lock
	go func() {
		log.Printf("=== GOROUTINE STARTED for task %s ===", taskRef.ID)
		defer func() {
			log.Printf("=== GOROUTINE ENDING for task %s ===", taskRef.ID)
			// Clear the running task when done
			queueMutex.Lock()
			runningTask = nil
			queueBusy = false
			queueMutex.Unlock()

			log.Printf("Task %s completed, processing next task", taskRef.ID)

			// CRITICAL FIX: Add small delay before processing next task
			// This prevents race condition where completion events interfere with next task status
			go func() {
				time.Sleep(100 * time.Millisecond) // 100ms delay to allow completion event to be processed
				ProcessNextTask()
			}()
		}()

		ExecuteTask(taskRef)
	}()

	log.Printf("=== PROCESS NEXT TASK COMPLETED ===")
}

// CreateContext creates a new context with cancellation
func CreateContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// SendTaskUpdate sends a task update via WebSocket
func SendTaskUpdate(task *Task) {
	websocket.SendTaskUpdate(task.ID, task.Status, task.Message)
}

// AddUserAssistMessage adds a user-assist message for a task
func AddUserAssistMessage(taskID, message string) bool {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	// Check if task exists and is in progress
	taskMutex.Lock()
	task, exists := tasks[taskID]
	taskMutex.Unlock()

	if !exists {
		log.Printf("Task %s not found, cannot add user-assist message", taskID)
		return false
	}

	if task.Status != "in-progress" {
		log.Printf("Task %s is not in progress (status: %s), ignoring user-assist message", taskID, task.Status)
		return false
	}

	// Check if there's already a non-injected user-assist message for this task
	if existingMsg, exists := userAssistMessages[taskID]; exists && !existingMsg.Injected {
		log.Printf("Task %s already has a pending user-assist message, replacing it", taskID)
	}

	// Add or update the user-assist message
	userAssistMessages[taskID] = &UserAssistMessage{
		TaskID:    taskID,
		Message:   message,
		CreatedAt: time.Now(),
		Injected:  false,
	}

	log.Printf("Added user-assist message for task %s: %s", taskID, message)
	return true
}

// GetUserAssistMessage gets the next user-assist message for a task and marks it as injected
func GetUserAssistMessage(taskID string) *UserAssistMessage {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	msg, exists := userAssistMessages[taskID]
	if !exists || msg.Injected {
		return nil
	}

	// Mark as injected
	msg.Injected = true
	log.Printf("Retrieved and marked user-assist message as injected for task %s: %s", taskID, msg.Message)

	return msg
}

// CleanupUserAssistMessages removes user-assist messages for completed/canceled tasks
func CleanupUserAssistMessages(taskID string) {
	userAssistMutex.Lock()
	defer userAssistMutex.Unlock()

	delete(userAssistMessages, taskID)
	log.Printf("Cleaned up user-assist messages for task %s", taskID)
}

// GetExecutionState returns the current execution engine state
func GetExecutionState() *ExecutionState {
	taskMutex.Lock()
	defer taskMutex.Unlock()

	// Create a slice of tasks for the execution state
	tasks := make([]Task, 0, len(tasks))
	i := 0
	for _, task := range tasks {
		tasks[i] = task
		i++
	}

	// Get selected and running tasks
	var selectedTask string
	var runningTaskID string
	var queuedTasks []string

	queueMutex.RLock()
	if runningTask != nil {
		runningTaskID = runningTask.ID
	}

	// Add queued tasks to execution state
	for _, queuedTask := range taskQueue {
		queuedTasks = append(queuedTasks, queuedTask.ID)
	}
	queueMutex.RUnlock()

	return &ExecutionState{
		Tasks:        tasks,
		SelectedTask: selectedTask,
		RunningTask:  runningTaskID,
		QueuedTasks:  queuedTasks,
	}
}

// UpdateSubtask sends a subtask update via WebSocket
func UpdateSubtask(taskID string, subtaskID int, description string, isActive bool, actions []actionpkg.Action) {
	// Convert actions to a more serializable format
	serializableActions := make([]interface{}, len(actions))
	for i, action := range actions {
		serializableActions[i] = action
	}

	websocket.SendSubtaskUpdate(taskID, subtaskID, description, isActive, serializableActions)
}

// UpdateAction sends an action update via WebSocket
func UpdateAction(taskID string, subtaskID int, actionIndex int, action actionpkg.Action) {
	// Convert action to a more serializable format
	var serializableAction map[string]interface{}

	serializableAction = map[string]interface{}{
		"action":           action.Action,
		"actionSequenceId": action.ActionSequenceID,
		"coordinates":      action.Coordinates,
		"duration":         action.Duration,
		"inputString":      action.InputString,
		"keyTapString":     action.KeyTapString,
		"keyString":        action.KeyString,
		"actionsRange":     action.ActionsRange,
		"repeatTimes":      action.RepeatTimes,
		"parameters":       action.Parameters,
		"description":      action.Description,
	}

	websocket.SendActionUpdate(taskID, subtaskID, actionIndex, serializableAction)
}

// BroadcastExecutionEngineUpdate broadcasts execution engine updates via WebSocket
func BroadcastExecutionEngineUpdate(updateType string, data interface{}) {
	websocket.SendExecutionEngineUpdate(updateType, data)
}
