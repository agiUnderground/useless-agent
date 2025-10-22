package action

import (
	"fmt"
	"time"

	"github.com/go-vgo/robotgo"
)

// actionFunctions maps action names to their execution functions
var actionFunctions = map[string]func(*Action, ...interface{}){
	"mouseMove":            mouseMoveExecution,
	"mouseMoveRelative":    mouseMoveRelativeExecution,
	"mouseClickLeft":       mouseClickLeftExecution,
	"mouseClickLeftDouble": mouseClickLeftDoubleExecution,
	"mouseClickRight":      mouseClickRightExecution,
	"nop":                  nopActionExecution,
	"stateUpdate":          stateUpdateActionExecution,
	"stopIteration":        stopIterationActionExecution,
	"printString":          printStringActionExecution,
	"keyTap":               keyTapActionExecution,
	"dragSmooth":           dragSmoothActionExecution,
	"keyDown":              keyDownActionExecution,
	"keyUp":                keyUpActionExecution,
	"scrollSmooth":         scrollSmoothActionExecution,
	"repeat":               repeatActionExecution,
}

// SetExecuteFunction sets the Execute function for an action based on its Action field
func SetExecuteFunction(action *Action) {
	if execFunc, exists := actionFunctions[action.Action]; exists {
		action.Execute = execFunc
	} else {
		// Default action if not found
		action.Execute = nopActionExecution
	}
}

// Action execution functions

func mouseMoveExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseMove' Action (ID: %d)\n", a.ActionSequenceID)
	if a.Coordinates.X != 0 || a.Coordinates.Y != 0 {
		fmt.Printf("Coordinates: X=%d, Y=%d\n", a.Coordinates.X, a.Coordinates.Y)
		robotgo.MoveSmooth(a.Coordinates.X, a.Coordinates.Y)
	} else {
		fmt.Println("No coordinates provided.")
	}
}

func mouseMoveRelativeExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseMoveRelative' Action (ID: %d)\n", a.ActionSequenceID)
	if a.Coordinates.X != 0 || a.Coordinates.Y != 0 {
		fmt.Printf("Coordinates: X=%d, Y=%d\n", a.Coordinates.X, a.Coordinates.Y)
		robotgo.MoveSmoothRelative(a.Coordinates.X, a.Coordinates.Y)
	} else {
		fmt.Println("No coordinates provided.")
	}
}

func mouseClickLeftExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseClickLeft' Action (ID: %d)\n", a.ActionSequenceID)
	robotgo.Click()
}

func mouseClickLeftDoubleExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseClickLeftDouble' Action (ID: %d)\n", a.ActionSequenceID)
	robotgo.Click("left", true)
}

func mouseClickRightExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseClickRight' Action (ID: %d)\n", a.ActionSequenceID)
	robotgo.Click("right")
}

func nopActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing nop action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.Duration > 0 {
		time.Sleep(time.Duration(a.Duration) * time.Second)
	}
}

func stateUpdateActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing stateUpdate action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
}

func stopIterationActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing stopIteration action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
}

func printStringActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing printString action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.InputString != "" {
		robotgo.TypeStrDelay(a.InputString, 100)
	}
}

func keyTapActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyTap action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.KeyTapString != "" {
		robotgo.KeyTap(a.KeyTapString)
	}
}

func dragSmoothActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing DragSmooth action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.Coordinates.X != 0 || a.Coordinates.Y != 0 {
		robotgo.DragSmooth(a.Coordinates.X, a.Coordinates.Y)
	}
}

func keyDownActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyDown action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.KeyString != "" {
		robotgo.KeyDown(a.KeyString)
	}
}

func keyUpActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyUp action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	if a.KeyString != "" {
		robotgo.KeyUp(a.KeyString)
	}
}

func scrollSmoothActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing scrollSmooth action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.ScrollSmooth(a.Coordinates.Y)
}

func repeatActionExecution(a *Action, params ...interface{}) {
	if len(params) == 0 {
		return
	}

	tmp := params[0].(*[]Action)
	actions := *tmp

	if len(a.ActionsRange) < 2 {
		return
	}

	start := a.ActionsRange[0] - 1
	end := a.ActionsRange[1]

	// Ensure bounds are valid
	if start < 0 {
		start = 0
	}
	if end > len(actions) {
		end = len(actions)
	}
	if start >= end {
		return
	}

	for i := 0; i < a.RepeatTimes; i++ {
		for j := start; j < end; j++ {
			if j < len(actions) {
				actions[j].Execute(&actions[j])
			}
		}
	}
}
