package task

import (
	"context"
	"time"
)

// Task represents a running task
type Task struct {
	ID         string
	Status     string // "in-the-queue", "in-progress", "completed", "broken", "canceled"
	Message    string
	CreatedAt  time.Time
	Context    context.Context    // Context for cancellation
	CancelFunc context.CancelFunc // Function to cancel the context
}

// TaskUpdate represents a task status update
type TaskUpdate struct {
	Type    string `json:"type"`
	TaskID  string `json:"taskId"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// SubTask represents a subtask in goal breakdown
type SubTask struct {
	Id          int    `json:"id"`
	Description string `json:"description"`
}

// PromptLog represents a log of prompts used in task execution
type PromptLog struct {
	Iteration int64  `json:"iteration"`
	Message   string `json:"message"`
}
