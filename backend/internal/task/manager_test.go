package task

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	actionpkg "useless-agent/internal/action"
)

func resetManagerForTest() {
	managerMutex.Lock()
	tasks = make(map[string]*Task)
	taskIDCounter = 0
	taskQueue = make([]*Task, 0)
	runningTask = nil
	taskSubtasks = make(map[string][]SubtaskSnapshot)
	taskRunner = ExecuteTask
	managerMutex.Unlock()
}

func TestGetExecutionStateIsJSONSerializableWithLiveSubtasks(t *testing.T) {
	resetManagerForTest()
	defer resetManagerForTest()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	running := &Task{
		ID:         "t-1",
		Status:     StatusInProgress,
		Message:    "serialize me",
		Context:    ctx,
		CancelFunc: cancel,
		CreatedAt:  time.Now(),
	}
	managerMutex.Lock()
	tasks["t-1"] = running
	runningTask = running
	managerMutex.Unlock()

	UpdateSubtask("t-1", 1, "first subtask", true, []actionpkg.Action{{Seq: 1, Code: "tp", X: 10, Y: 20}})
	UpdateAction("t-1", 1, 0, actionpkg.Action{Seq: 1, Code: "cl", X: 5, Y: 6})

	state := GetExecutionState()

	// Regression: Context/CancelFunc used to make this handler return 500.
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("execution state must be JSON serializable: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("execution state JSON invalid: %v", err)
	}

	subs := state.Subtasks["t-1"]
	if len(subs) != 1 {
		t.Fatalf("expected 1 subtask snapshot, got %d", len(subs))
	}
	if !subs[0].IsActive || subs[0].IsCompleted {
		t.Fatalf("expected active, not-completed subtask, got %+v", subs[0])
	}
	if len(subs[0].Actions) != 1 {
		t.Fatalf("expected 1 recorded action, got %d", len(subs[0].Actions))
	}

	// Terminal tasks can no longer report an actively-executing subtask.
	managerMutex.Lock()
	running.Status = StatusCompleted
	managerMutex.Unlock()
	state = GetExecutionState()
	if subs := state.Subtasks["t-1"]; len(subs) != 1 || subs[0].IsActive || !subs[0].IsCompleted {
		t.Fatalf("expected completed subtask snapshot after task completes, got %+v", state.Subtasks["t-1"])
	}
}

func TestManager_SecondTaskTransitionsToInProgressAfterFirstCompletes(t *testing.T) {
	resetManagerForTest()
	defer resetManagerForTest()

	var (
		mu             sync.Mutex
		observedStates = make(map[string][]string)
		doneCh         = make(chan string, 2)
	)

	taskRunner = func(tsk *Task) {
		mu.Lock()
		observedStates[tsk.ID] = append(observedStates[tsk.ID], tsk.Status)
		mu.Unlock()
		UpdateTaskStatus(tsk.ID, StatusCompleted, tsk.Message)
		doneCh <- tsk.ID
	}

	t1 := CreateTask("first task", "zai", "glm-5.3", "", "sess-1")
	EnqueueTask(t1)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task 1")
	}

	// Wait briefly for task 1's goroutine defer to clear runningTask
	deadline := time.Now().Add(2 * time.Second)
	for IsTaskRunning() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	t2 := CreateTask("second task", "zai", "glm-5.3", "", "sess-1")
	EnqueueTask(t2)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task 2")
	}

	mu.Lock()
	t2RunnerStates := observedStates[t2.ID]
	mu.Unlock()

	if len(t2RunnerStates) != 1 || t2RunnerStates[0] != StatusInProgress {
		t.Fatalf("expected task 2 to be %q when started, got %v", StatusInProgress, t2RunnerStates)
	}

	gotT2, ok := GetTask(t2.ID)
	if !ok || gotT2.Status != StatusCompleted {
		t.Fatalf("expected task 2 final status %q, got %q", StatusCompleted, gotT2.Status)
	}
}

func TestManager_QueuedTaskStartsAutomaticallyWhenFirstCompletes(t *testing.T) {
	resetManagerForTest()
	defer resetManagerForTest()

	releaseT1 := make(chan struct{})
	t1Started := make(chan struct{})
	t2Started := make(chan string, 1)
	t2Done := make(chan struct{})

	taskRunner = func(tsk *Task) {
		if tsk.Message == "first" {
			close(t1Started)
			<-releaseT1
			UpdateTaskStatus(tsk.ID, StatusCompleted, tsk.Message)
			return
		}
		t2Started <- tsk.Status
		UpdateTaskStatus(tsk.ID, StatusCompleted, tsk.Message)
		close(t2Done)
	}

	t1 := CreateTask("first", "zai", "glm-5.3", "", "sess-1")
	EnqueueTask(t1)
	<-t1Started

	t2 := CreateTask("second", "zai", "glm-5.3", "", "sess-1")
	EnqueueTask(t2)

	// While t1 is running, t2 must remain in-the-queue.
	if got, _ := GetTask(t2.ID); got.Status != StatusQueued {
		t.Fatalf("expected t2 to be %q while t1 runs, got %q", StatusQueued, got.Status)
	}
	if qLen := GetQueueLength(); qLen != 1 {
		t.Fatalf("expected queue length 1, got %d", qLen)
	}

	// Finish t1 -> t2 should automatically dequeue and transition to in-progress.
	close(releaseT1)

	select {
	case st := <-t2Started:
		if st != StatusInProgress {
			t.Fatalf("expected t2 status %q when started, got %q", StatusInProgress, st)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for t2 to start")
	}

	<-t2Done
}

func TestManager_TerminalStatusCannotBeOverwritten(t *testing.T) {
	resetManagerForTest()
	defer resetManagerForTest()

	t1 := CreateTask("test", "zai", "glm-5.3", "", "sess-1")
	UpdateTaskStatus(t1.ID, StatusInProgress, t1.Message)
	UpdateTaskStatus(t1.ID, StatusCompleted, t1.Message)

	// Stale/late non-terminal updates must be ignored.
	UpdateTaskStatus(t1.ID, StatusQueued, t1.Message)
	UpdateTaskStatus(t1.ID, StatusInProgress, t1.Message)

	got, _ := GetTask(t1.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("expected terminal status %q to be preserved, got %q", StatusCompleted, got.Status)
	}
}
