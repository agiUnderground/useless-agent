package action

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCompactJSONUnmarshalAndVerboseFormatting(t *testing.T) {
	raw := `[
		{"i":1,"a":"mm","x":555,"y":777},
		{"i":2,"a":"cl"},
		{"i":3,"a":"tp","t":"hello"},
		{"i":4,"a":"kt","k":"enter"},
		{"i":5,"a":"w","d":2},
		{"i":6,"a":"rp","r":[1,4],"n":3},
		{"i":7,"a":"st"}
	]`

	var actions []Action
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(actions) != 7 {
		t.Fatalf("expected 7 actions, got %d", len(actions))
	}

	got := FormatVerbose(actions[0])
	want := "[#1] mouseMove (x=555, y=777)"
	if got != want {
		t.Errorf("FormatVerbose(actions[0]) = %q, want %q", got, want)
	}

	if !actions[6].IsStop() {
		t.Errorf("expected actions[6].IsStop() to be true")
	}

	vm := ToVerboseMap(actions[3])
	if vm["action"] != "keyTap" || vm["keyTapString"] != "enter" || vm["actionSequenceId"] != 4 {
		t.Errorf("unexpected verbose map: %+v", vm)
	}
}

func TestRegistrySupportsCompactAndVerboseNames(t *testing.T) {
	r := DefaultRegistry()
	if r.ResolveName("mm") != "mouseMove" {
		t.Errorf("expected mm -> mouseMove, got %q", r.ResolveName("mm"))
	}
	if r.ResolveCode("mouseMove") != "mm" {
		t.Errorf("expected mouseMove -> mm, got %q", r.ResolveCode("mouseMove"))
	}
	if !strings.Contains(r.PromptSpec(), `"a":"mm"`) {
		t.Errorf("expected PromptSpec to include compact example")
	}
}

func TestExecutorWithMockActionsAndVerboseLogs(t *testing.T) {
	r := NewRegistry()
	var executed []string
	var logs []string

	r.Register(Definition{
		Code:       CodeMouseMove,
		Name:       "mouseMove",
		FormatArgs: func(a *Action) string { return fmt.Sprintf("x=%d, y=%d", a.X, a.Y) },
		Execute: func(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
			executed = append(executed, fmt.Sprintf("mm:%d,%d", a.X, a.Y))
			return ExecutionResult{}, nil
		},
	})
	r.Register(Definition{
		Code: CodeMouseClickLeft,
		Name: "mouseClickLeft",
		Execute: func(_ *ExecutionContext, _ *Action) (ExecutionResult, error) {
			executed = append(executed, "cl")
			return ExecutionResult{}, nil
		},
	})
	r.Register(Definition{
		Code:       CodeRepeat,
		Name:       "repeat",
		FormatArgs: func(a *Action) string { return fmt.Sprintf("range=%v, times=%d", a.Range, a.Repeat) },
		Execute:    execRepeat,
	})
	r.Register(Definition{
		Code:    CodeStopIteration,
		Name:    "stopIteration",
		Execute: execStopIteration,
	})

	exec := NewExecutor(r)
	exec.logf = func(format string, args ...interface{}) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	raw := `[
		{"i":1,"a":"mm","x":100,"y":200},
		{"i":2,"a":"cl"},
		{"i":3,"a":"rp","r":[1,2],"n":2},
		{"i":4,"a":"st"}
	]`
	var actions []Action
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	ctx := &ExecutionContext{Actions: actions, Executor: exec}
	stopped := false
	for i := range actions {
		ctx.Index = i
		res, err := exec.Execute(ctx, &actions[i])
		if err != nil {
			t.Fatalf("execute action %d failed: %v", i, err)
		}
		if res.StopIteration {
			stopped = true
			break
		}
	}

	if !stopped {
		t.Fatalf("expected StopIteration to be true")
	}

	// 1st run (mm, cl) + 2 repeats of (mm, cl) = 6 mock executions
	wantExec := []string{"mm:100,200", "cl", "mm:100,200", "cl", "mm:100,200", "cl"}
	if strings.Join(executed, "|") != strings.Join(wantExec, "|") {
		t.Fatalf("executed = %v, want %v", executed, wantExec)
	}

	// Verify logs use verbose human-readable names, not compact codes
	joinedLogs := strings.Join(logs, "\n")
	for _, wantSubstr := range []string{
		"Executing [#1] mouseMove (x=100, y=200)",
		"Executing [#2] mouseClickLeft",
		"Executing [#3] repeat (range=[1 2], times=2)",
		"Executing [#4] stopIteration",
	} {
		if !strings.Contains(joinedLogs, wantSubstr) {
			t.Errorf("expected logs to contain %q, got:\n%s", wantSubstr, joinedLogs)
		}
	}
}
