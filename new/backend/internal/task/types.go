package task

import (
	"context"
	"time"

	"useless-agent/internal/action"
)

type Task struct {
	ID         string
	Status     string
	Message    string
	CreatedAt  time.Time
	Context    context.Context
	CancelFunc context.CancelFunc
}

type TaskUpdate struct {
	Type    string `json:"type"`
	TaskID  string `json:"taskId"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type SubTask struct {
	Id          int             `json:"id"`
	Description string          `json:"description"`
	IsActive    bool            `json:"isActive"`
	Actions     []action.Action `json:"actions"`
}

type ExecutionState struct {
	Tasks        []Task   `json:"tasks"`
	SelectedTask string   `json:"selectedTask"`
	RunningTask  string   `json:"runningTask"`
	QueuedTasks  []string `json:"queuedTasks"`
}

type PromptLog struct {
	Iteration int64  `json:"iteration"`
	Message   string `json:"message"`
}

type UserAssistMessage struct {
	TaskID    string    `json:"taskId"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	Injected  bool      `json:"injected"`
}

type SubTaskUpdate struct {
	TaskID      string          `json:"taskId"`
	SubTaskID   int             `json:"subtaskId"`
	Description string          `json:"description"`
	IsActive    bool            `json:"isActive"`
	Actions     []action.Action `json:"actions"`
}

type ActionUpdate struct {
	TaskID      string        `json:"taskId"`
	SubTaskID   int           `json:"subtaskId"`
	ActionIndex int           `json:"actionIndex"`
	Action      action.Action `json:"action"`
}
