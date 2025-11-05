package action

import (
	"fmt"
	"time"

	"useless-agent/internal/mouse"
)

func SetExecuteFunction(a *Action) {
	switch a.Action {
	case "mouseMove":
		a.Execute = executeMouseMove
	case "mouseMoveRelative":
		a.Execute = executeMouseMoveRelative
	case "mouseClickLeft":
		a.Execute = executeMouseClickLeft
	case "mouseClickRight":
		a.Execute = executeMouseClickRight
	case "mouseClickLeftDouble":
		a.Execute = executeMouseClickLeftDouble
	case "nop":
		a.Execute = executeNop
	case "printString":
		a.Execute = executePrintString
	case "keyTap":
		a.Execute = executeKeyTap
	case "dragSmooth":
		a.Execute = executeDragSmooth
	case "scrollSmooth":
		a.Execute = executeScrollSmooth
	case "keyDown":
		a.Execute = executeKeyDown
	case "keyUp":
		a.Execute = executeKeyUp
	case "repeat":
		a.Execute = executeRepeat
	case "stopIteration":
		a.Execute = executeStopIteration
	default:
		a.Execute = executeUnsupported
	}
}

func executeMouseMove(a *Action, args ...interface{}) {
	mouse.Move(a.Coordinates.X, a.Coordinates.Y)
}

func executeMouseMoveRelative(a *Action, args ...interface{}) {
	x, y := mouse.GetPosition()
	mouse.Move(x+a.Coordinates.X, y+a.Coordinates.Y)
}

func executeMouseClickLeft(a *Action, args ...interface{}) {
	mouse.Click("left")
}

func executeMouseClickRight(a *Action, args ...interface{}) {
	mouse.Click("right")
}

func executeMouseClickLeftDouble(a *Action, args ...interface{}) {
	mouse.DoubleClick("left")
}

func executeNop(a *Action, args ...interface{}) {
	if a.Duration > 0 {
		time.Sleep(time.Duration(a.Duration) * time.Second)
	}
}

func executePrintString(a *Action, args ...interface{}) {
	mouse.Type(a.InputString)
}

func executeKeyTap(a *Action, args ...interface{}) {
	mouse.Tap(a.KeyTapString)
}

func executeDragSmooth(a *Action, args ...interface{}) {
	mouse.Drag(a.Coordinates.X, a.Coordinates.Y)
}

func executeScrollSmooth(a *Action, args ...interface{}) {
	mouse.Scroll(a.Coordinates.X, a.Coordinates.Y)
}

func executeKeyDown(a *Action, args ...interface{}) {
	mouse.KeyDown(a.KeyString)
}

func executeKeyUp(a *Action, args ...interface{}) {
	mouse.KeyUp(a.KeyString)
}

func executeRepeat(a *Action, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	actions, ok := args[0].(*[]Action)
	if !ok {
		return
	}

	if len(a.ActionsRange) != 2 {
		return
	}

	start := a.ActionsRange[0] - 1
	end := a.ActionsRange[1]

	if start < 0 || end > len(*actions) || start >= end {
		return
	}

	for i := 0; i < a.RepeatTimes; i++ {
		for j := start; j < end; j++ {
			if (*actions)[j].Execute != nil {
				(*actions)[j].Execute(&(*actions)[j])
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func executeStopIteration(a *Action, args ...interface{}) {
	fmt.Println("Stop iteration requested")
}

func executeUnsupported(a *Action, args ...interface{}) {
	fmt.Printf("Unsupported action: %s\n", a.Action)
}
