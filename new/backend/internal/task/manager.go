package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"useless-agent/internal/action"
	"useless-agent/internal/websocket"
)

type TaskManager struct {
	tasks          map[string]*Task
	queue          []*Task
	runningTask    *Task
	userAssistMsgs map[string][]*UserAssistMessage
	mu             sync.RWMutex
	taskUpdateChan chan TaskUpdate
	subtaskChan    chan SubTaskUpdate
	actionChan     chan ActionUpdate
	executionChan  chan ExecutionState
}

var (
	manager     *TaskManager
	managerOnce sync.Once
)

func GetManager() *TaskManager {
	managerOnce.Do(func() {
		manager = &TaskManager{
			tasks:          make(map[string]*Task),
			queue:          make([]*Task, 0),
			userAssistMsgs: make(map[string][]*UserAssistMessage),
			taskUpdateChan: make(chan TaskUpdate, 100),
			subtaskChan:    make(chan SubTaskUpdate, 100),
			actionChan:     make(chan ActionUpdate, 100),
			executionChan:  make(chan ExecutionState, 100),
		}
		go manager.processQueue()
		go manager.broadcastUpdates()
	})
	return manager
}

func (tm *TaskManager) CreateTask(message string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	task := &Task{
		ID:         generateID(),
		Status:     "created",
		Message:    message,
		CreatedAt:  time.Now(),
		Context:    ctx,
		CancelFunc: cancel,
	}

	tm.tasks[task.ID] = task
	log.Printf("Created task %s: %s", task.ID, message)
	return task
}

func (tm *TaskManager) EnqueueTask(task *Task) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task.Status = "in-the-queue"
	tm.queue = append(tm.queue, task)

	tm.taskUpdateChan <- TaskUpdate{
		Type:    "taskUpdate",
		TaskID:  task.ID,
		Status:  task.Status,
		Message: task.Message,
	}

	log.Printf("Enqueued task %s", task.ID)
}

func (tm *TaskManager) CancelTask(taskID string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return false
	}

	if task.Status == "completed" || task.Status == "canceled" {
		return false
	}

	task.CancelFunc()
	task.Status = "canceled"

	tm.taskUpdateChan <- TaskUpdate{
		Type:    "taskUpdate",
		TaskID:  taskID,
		Status:  "canceled",
		Message: "Task canceled by user",
	}

	if tm.runningTask != nil && tm.runningTask.ID == taskID {
		tm.runningTask = nil
	}

	tm.removeFromQueue(taskID)
	log.Printf("Canceled task %s", taskID)
	return true
}

func (tm *TaskManager) UpdateTaskStatus(taskID, status, message string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return
	}

	task.Status = status
	if message != "" {
		task.Message = message
	}

	tm.taskUpdateChan <- TaskUpdate{
		Type:    "taskUpdate",
		TaskID:  taskID,
		Status:  status,
		Message: message,
	}

	if status == "completed" || status == "canceled" || status == "broken" {
		if tm.runningTask != nil && tm.runningTask.ID == taskID {
			tm.runningTask = nil
		}
		tm.removeFromQueue(taskID)
	}

	log.Printf("Updated task %s status to %s", taskID, status)
}

func (tm *TaskManager) UpdateSubtask(taskID string, subtaskID int, description string, isActive bool, actions []action.Action) {
	tm.subtaskChan <- SubTaskUpdate{
		TaskID:      taskID,
		SubTaskID:   subtaskID,
		Description: description,
		IsActive:    isActive,
		Actions:     actions,
	}
}

func (tm *TaskManager) UpdateAction(taskID string, subtaskID, actionIndex int, action action.Action) {
	tm.actionChan <- ActionUpdate{
		TaskID:      taskID,
		SubTaskID:   subtaskID,
		ActionIndex: actionIndex,
		Action:      action,
	}
}

func (tm *TaskManager) AddUserAssistMessage(taskID, message string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, exists := tm.tasks[taskID]
	if !exists {
		return false
	}

	if task.Status != "in-progress" {
		return false
	}

	msg := &UserAssistMessage{
		TaskID:    taskID,
		Message:   message,
		CreatedAt: time.Now(),
		Injected:  false,
	}

	if tm.userAssistMsgs[taskID] == nil {
		tm.userAssistMsgs[taskID] = make([]*UserAssistMessage, 0)
	}

	tm.userAssistMsgs[taskID] = append(tm.userAssistMsgs[taskID], msg)
	log.Printf("Added user-assist message for task %s: %s", taskID, message)
	return true
}

func (tm *TaskManager) GetUserAssistMessage(taskID string) *UserAssistMessage {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	msgs, exists := tm.userAssistMsgs[taskID]
	if !exists || len(msgs) == 0 {
		return nil
	}

	for _, msg := range msgs {
		if !msg.Injected {
			msg.Injected = true
			return msg
		}
	}

	return nil
}

func (tm *TaskManager) CleanupUserAssistMessages(taskID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.userAssistMsgs, taskID)
}

func (tm *TaskManager) GetExecutionState() ExecutionState {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var tasks []Task
	for _, task := range tm.tasks {
		tasks = append(tasks, *task)
	}

	var queuedTasks []string
	for _, task := range tm.queue {
		queuedTasks = append(queuedTasks, task.ID)
	}

	runningTaskID := ""
	if tm.runningTask != nil {
		runningTaskID = tm.runningTask.ID
	}

	return ExecutionState{
		Tasks:        tasks,
		SelectedTask: "",
		RunningTask:  runningTaskID,
		QueuedTasks:  queuedTasks,
	}
}

func (tm *TaskManager) processQueue() {
	for {
		tm.mu.Lock()

		if tm.runningTask == nil && len(tm.queue) > 0 {
			task := tm.queue[0]
			tm.queue = tm.queue[1:]
			tm.runningTask = task
			task.Status = "in-progress"

			tm.taskUpdateChan <- TaskUpdate{
				Type:    "taskUpdate",
				TaskID:  task.ID,
				Status:  "in-progress",
				Message: task.Message,
			}

			tm.mu.Unlock()

			go executeTask(task)
		} else {
			tm.mu.Unlock()
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (tm *TaskManager) broadcastUpdates() {
	for {
		select {
		case update := <-tm.taskUpdateChan:
			BroadcastTaskUpdate(update)
		case update := <-tm.subtaskChan:
			BroadcastSubtaskUpdate(update)
		case update := <-tm.actionChan:
			BroadcastActionUpdate(update)
		default:
			tm.executionChan <- tm.GetExecutionState()
			time.Sleep(1 * time.Second)
		}
	}
}

func (tm *TaskManager) removeFromQueue(taskID string) {
	for i, task := range tm.queue {
		if task.ID == taskID {
			tm.queue = append(tm.queue[:i], tm.queue[i+1:]...)
			break
		}
	}
}

func CreateTask(message string) *Task {
	return GetManager().CreateTask(message)
}

func EnqueueTask(task *Task) {
	GetManager().EnqueueTask(task)
}

func CancelTask(taskID string) bool {
	return GetManager().CancelTask(taskID)
}

func UpdateTaskStatus(taskID, status, message string) {
	GetManager().UpdateTaskStatus(taskID, status, message)
}

func UpdateSubtask(taskID string, subtaskID int, description string, isActive bool, actions []action.Action) {
	GetManager().UpdateSubtask(taskID, subtaskID, description, isActive, actions)
}

func UpdateAction(taskID string, subtaskID, actionIndex int, action action.Action) {
	GetManager().UpdateAction(taskID, subtaskID, actionIndex, action)
}

func AddUserAssistMessage(taskID, message string) bool {
	return GetManager().AddUserAssistMessage(taskID, message)
}

func GetUserAssistMessage(taskID string) *UserAssistMessage {
	return GetManager().GetUserAssistMessage(taskID)
}

func CleanupUserAssistMessages(taskID string) {
	GetManager().CleanupUserAssistMessages(taskID)
}

func GetExecutionState() ExecutionState {
	return GetManager().GetExecutionState()
}

func BroadcastTaskUpdate(update TaskUpdate) {
	websocket.SendTaskUpdate(update.TaskID, update.Status, update.Message)
}

func BroadcastSubtaskUpdate(update SubTaskUpdate) {
	websocket.BroadcastMessage("subtaskUpdate", update)
}

func BroadcastActionUpdate(update ActionUpdate) {
	websocket.BroadcastMessage("actionUpdate", update)
}

func BroadcastExecutionEngineUpdate(updateType string, data interface{}) {
	websocket.BroadcastExecutionEngineUpdate(updateType, data)
}

func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func executeTask(task *Task) {
	go ExecuteTask(task)
}
