package service

import (
	"useless-agent/internal/task"
)

type TaskService struct {
	taskManager *task.TaskManager
}

func NewTaskService() *TaskService {
	return &TaskService{
		taskManager: task.GetManager(),
	}
}

func (ts *TaskService) CreateTask(message string) *task.Task {
	return ts.taskManager.CreateTask(message)
}

func (ts *TaskService) EnqueueTask(task *task.Task) {
	ts.taskManager.EnqueueTask(task)
}

func (ts *TaskService) CancelTask(taskID string) bool {
	return ts.taskManager.CancelTask(taskID)
}

func (ts *TaskService) GetExecutionState() task.ExecutionState {
	return ts.taskManager.GetExecutionState()
}
