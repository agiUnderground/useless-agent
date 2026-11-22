package task

import (
	"context"
	"time"
)

// Task represents a running task
type Task struct {
	ID            string             `json:"id"`
	Status        string             `json:"status"` // "in-the-queue", "in-progress", "completed", "broken", "canceled"
	Message       string             `json:"message"`
	Provider      string             `json:"provider,omitempty"`
	Model         string             `json:"model,omitempty"`
	ThinkingLevel string             `json:"thinkingLevel,omitempty"`
	SessionID     string             `json:"sessionId,omitempty"`
	CreatedAt     time.Time          `json:"createdAt"`
	Context       context.Context    `json:"-"` // Context for cancellation (not serializable)
	CancelFunc    context.CancelFunc `json:"-"` // Function to cancel the context (not serializable)
}

// TaskUpdate represents a task status update
type TaskUpdate struct {
	Type    string `json:"type"`
	TaskID  string `json:"taskId"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

// SubTask represents a subtask in goal breakdown
type SubTask struct {
	Id          int      `json:"id"`
	Description string   `json:"description"`
	IsActive    bool     `json:"isActive"`
	Actions     []Action `json:"actions"`
}

// Action represents an action being executed
type Action struct {
	Action           string      `json:"action"`
	ActionSequenceID int         `json:"actionSequenceId"`
	Coordinates      interface{} `json:"coordinates"`
	Duration         int         `json:"duration"`
	InputString      string      `json:"inputString"`
	KeyTapString     string      `json:"keyTapString"`
	KeyString        string      `json:"keyString"`
	ActionsRange     interface{} `json:"actionsRange"`
	RepeatTimes      int         `json:"repeatTimes"`
	Description      string      `json:"description"`
}

// ExecutionState represents the current execution engine state
type ExecutionState struct {
	Tasks        []Task                       `json:"tasks"`
	SelectedTask string                       `json:"selectedTask"`
	RunningTask  string                       `json:"runningTask"`
	QueuedTasks  []string                     `json:"queuedTasks"`
	Subtasks     map[string][]SubtaskSnapshot `json:"subtasks"`
}

// SubtaskSnapshot is the serializable live state of a subtask for the execution engine UI.
type SubtaskSnapshot struct {
	ID          int                      `json:"id"`
	Description string                   `json:"description"`
	IsActive    bool                     `json:"isActive"`
	IsCompleted bool                     `json:"isCompleted"`
	Actions     []map[string]interface{} `json:"actions"`
}

// PromptLog represents a log of prompts used in task execution
type PromptLog struct {
	Iteration int64  `json:"iteration"`
	Message   string `json:"message"`
}

// UserAssistMessage represents a user-assist message waiting to be injected
type UserAssistMessage struct {
	TaskID    string    `json:"taskId"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	Injected  bool      `json:"injected"`
}
