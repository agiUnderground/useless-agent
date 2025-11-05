package mouse

import (
	"fmt"

	"github.com/go-vgo/robotgo"
)

func Move(x, y int) {
	robotgo.MoveSmooth(x, y)
}

func MoveRelative(x, y int) {
	currentX, currentY := GetPosition()
	Move(currentX+x, currentY+y)
}

func Click(button string) {
	robotgo.Click(button)
}

func DoubleClick(button string) {
	robotgo.Click(button, true)
}

func Type(text string) {
	robotgo.TypeStr(text)
}

func Tap(key string) {
	robotgo.KeyTap(key)
}

func Drag(x, y int) {
	currentX, currentY := GetPosition()
	robotgo.DragSmooth(currentX, currentY, x, y)
}

func Scroll(x, y int) {
	robotgo.Scroll(x, y)
}

func KeyDown(key string) {
	robotgo.KeyToggle(key, "down")
}

func KeyUp(key string) {
	robotgo.KeyToggle(key, "up")
}

func GetPosition() (int, int) {
	return robotgo.GetMousePos()
}

func GetCursorPositionJSON() (string, error) {
	x, y := GetPosition()
	return fmt.Sprintf(`{"x": %d, "y": %d}`, x, y), nil
}
