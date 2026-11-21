package action

import (
	"fmt"
	"log"
	"time"

	"github.com/go-vgo/robotgo"
)

const maxRepeatDepth = 3

// Executor runs actions looked up from a Registry and logs them in verbose form by default.
type Executor struct {
	registry *Registry
	logf     func(format string, args ...interface{})
}

// NewExecutor creates an Executor backed by the given Registry.
func NewExecutor(registry *Registry) *Executor {
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &Executor{
		registry: registry,
		logf:     log.Printf,
	}
}

// DefaultExecutor returns an Executor using DefaultRegistry and standard logging.
func DefaultExecutor() *Executor {
	return NewExecutor(DefaultRegistry())
}

// Execute runs a single action, logging its verbose human-readable form by default.
func (e *Executor) Execute(ctx *ExecutionContext, a *Action) (ExecutionResult, error) {
	if ctx == nil {
		ctx = &ExecutionContext{}
	}
	if ctx.Executor == nil {
		ctx.Executor = e
	}

	def, ok := e.registry.Lookup(a.Code)
	if !ok {
		e.logf("Unknown action %s, treating as nop", FormatVerboseWithRegistry(e.registry, *a))
		return execNop(ctx, a)
	}

	e.logf("Executing %s", FormatVerboseWithRegistry(e.registry, *a))
	return def.Execute(ctx, a)
}

func registerBuiltinActions(r *Registry) {
	r.Register(Definition{
		Code:        CodeMouseMove,
		Name:        "mouseMove",
		Description: "move cursor to absolute (x,y)",
		ExampleJSON: `{"i":1,"a":"mm","x":555,"y":777}`,
		Execute:     execMouseMove,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("x=%d, y=%d", a.X, a.Y) },
	})
	r.Register(Definition{
		Code:        CodeMouseMoveRelative,
		Name:        "mouseMoveRelative",
		Description: "move cursor by relative offset (x,y)",
		ExampleJSON: `{"i":2,"a":"mr","x":-10,"y":0}`,
		Execute:     execMouseMoveRelative,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("dx=%d, dy=%d", a.X, a.Y) },
	})
	r.Register(Definition{
		Code:        CodeMouseClickLeft,
		Name:        "mouseClickLeft",
		Description: "left mouse click",
		ExampleJSON: `{"i":3,"a":"cl"}`,
		Execute:     execMouseClickLeft,
	})
	r.Register(Definition{
		Code:        CodeMouseClickLeftDouble,
		Name:        "mouseClickLeftDouble",
		Description: "double left mouse click",
		ExampleJSON: `{"i":4,"a":"cd"}`,
		Execute:     execMouseClickLeftDouble,
	})
	r.Register(Definition{
		Code:        CodeMouseClickRight,
		Name:        "mouseClickRight",
		Description: "right mouse click",
		ExampleJSON: `{"i":5,"a":"cr"}`,
		Execute:     execMouseClickRight,
	})
	r.Register(Definition{
		Code:        CodeDragSmooth,
		Name:        "dragSmooth",
		Description: "drag cursor smoothly to (x,y)",
		ExampleJSON: `{"i":6,"a":"dr","x":555,"y":777}`,
		Execute:     execDragSmooth,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("x=%d, y=%d", a.X, a.Y) },
	})
	r.Register(Definition{
		Code:        CodeScrollSmooth,
		Name:        "scrollSmooth",
		Description: "scroll vertically by y (negative=down)",
		ExampleJSON: `{"i":7,"a":"sc","y":-77}`,
		Execute:     execScrollSmooth,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("y=%d", a.Y) },
	})
	r.Register(Definition{
		Code:        CodePrintString,
		Name:        "printString",
		Description: "type text string t",
		ExampleJSON: `{"i":8,"a":"tp","t":"hello"}`,
		Execute:     execPrintString,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("text=%q", a.Text) },
	})
	r.Register(Definition{
		Code:        CodeKeyTap,
		Name:        "keyTap",
		Description: "tap key k",
		ExampleJSON: `{"i":9,"a":"kt","k":"enter"}`,
		Execute:     execKeyTap,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("key=%q", a.Key) },
	})
	r.Register(Definition{
		Code:        CodeKeyDown,
		Name:        "keyDown",
		Description: "hold key k down",
		ExampleJSON: `{"i":10,"a":"kd","k":"lctrl"}`,
		Execute:     execKeyDown,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("key=%q", a.Key) },
	})
	r.Register(Definition{
		Code:        CodeKeyUp,
		Name:        "keyUp",
		Description: "release key k",
		ExampleJSON: `{"i":11,"a":"ku","k":"lctrl"}`,
		Execute:     execKeyUp,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("key=%q", a.Key) },
	})
	r.Register(Definition{
		Code:        CodeNop,
		Name:        "nop",
		Description: "wait d seconds",
		ExampleJSON: `{"i":12,"a":"w","d":3}`,
		Execute:     execNop,
		FormatArgs: func(a *Action) string {
			if a.Dur > 0 {
				return fmt.Sprintf("duration=%ds", a.Dur)
			}
			return ""
		},
	})
	r.Register(Definition{
		Code:        CodeRepeat,
		Name:        "repeat",
		Description: "repeat prior 1-based action range r=[start,end] n times",
		ExampleJSON: `{"i":13,"a":"rp","r":[4,8],"n":3}`,
		Execute:     execRepeat,
		FormatArgs:  func(a *Action) string { return fmt.Sprintf("range=%v, times=%d", a.Range, a.Repeat) },
	})
	r.Register(Definition{
		Code:        CodeStopIteration,
		Name:        "stopIteration",
		Description: "stop iteration when goal is achieved",
		ExampleJSON: `{"i":14,"a":"st"}`,
		Execute:     execStopIteration,
	})
}

func execMouseMove(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.X != 0 || a.Y != 0 {
		robotgo.MoveSmooth(a.X, a.Y)
	}
	return ExecutionResult{}, nil
}

func execMouseMoveRelative(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.X != 0 || a.Y != 0 {
		robotgo.MoveSmoothRelative(a.X, a.Y)
	}
	return ExecutionResult{}, nil
}

func execMouseClickLeft(_ *ExecutionContext, _ *Action) (ExecutionResult, error) {
	robotgo.Click()
	return ExecutionResult{}, nil
}

func execMouseClickLeftDouble(_ *ExecutionContext, _ *Action) (ExecutionResult, error) {
	robotgo.Click("left", true)
	return ExecutionResult{}, nil
}

func execMouseClickRight(_ *ExecutionContext, _ *Action) (ExecutionResult, error) {
	robotgo.Click("right")
	return ExecutionResult{}, nil
}

func execDragSmooth(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.X != 0 || a.Y != 0 {
		robotgo.DragSmooth(a.X, a.Y)
	}
	return ExecutionResult{}, nil
}

func execScrollSmooth(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	robotgo.ScrollSmooth(a.Y)
	return ExecutionResult{}, nil
}

func execPrintString(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.Text != "" {
		robotgo.TypeStrDelay(a.Text, 100)
	}
	return ExecutionResult{}, nil
}

func execKeyTap(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.Key != "" {
		robotgo.KeyTap(a.Key)
	}
	return ExecutionResult{}, nil
}

func execKeyDown(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.Key != "" {
		robotgo.KeyDown(a.Key)
	}
	return ExecutionResult{}, nil
}

func execKeyUp(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.Key != "" {
		robotgo.KeyUp(a.Key)
	}
	return ExecutionResult{}, nil
}

func execNop(_ *ExecutionContext, a *Action) (ExecutionResult, error) {
	if a.Dur > 0 {
		time.Sleep(time.Duration(a.Dur) * time.Second)
	}
	return ExecutionResult{}, nil
}

func execStopIteration(_ *ExecutionContext, _ *Action) (ExecutionResult, error) {
	return ExecutionResult{StopIteration: true}, nil
}

func execRepeat(ctx *ExecutionContext, a *Action) (ExecutionResult, error) {
	if ctx == nil || ctx.Executor == nil || len(ctx.Actions) == 0 {
		return ExecutionResult{}, nil
	}
	if ctx.Depth >= maxRepeatDepth {
		return ExecutionResult{}, fmt.Errorf("repeat nesting depth exceeded (%d)", maxRepeatDepth)
	}
	if len(a.Range) < 2 || a.Repeat <= 0 {
		return ExecutionResult{}, nil
	}

	start := a.Range[0] - 1
	end := a.Range[1]
	if start < 0 {
		start = 0
	}
	// Prevent repeating self or future actions
	if ctx.Index > 0 && end > ctx.Index {
		end = ctx.Index
	}
	if end > len(ctx.Actions) {
		end = len(ctx.Actions)
	}
	if start >= end {
		return ExecutionResult{}, nil
	}

	nestedCtx := &ExecutionContext{
		Actions:  ctx.Actions,
		Executor: ctx.Executor,
		Depth:    ctx.Depth + 1,
	}

	for i := 0; i < a.Repeat; i++ {
		for j := start; j < end; j++ {
			nestedCtx.Index = j
			res, err := ctx.Executor.Execute(nestedCtx, &ctx.Actions[j])
			if err != nil {
				return res, err
			}
			if res.StopIteration {
				return res, nil
			}
		}
	}

	return ExecutionResult{}, nil
}
