package x11

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

// X11WindowInfo represents comprehensive window information from X11
type X11WindowInfo struct {
	Windows []X11Window `json:"windows"`
}

// X11Window represents a single window with all its properties
type X11Window struct {
	ID         uint32         `json:"id"`
	Title      string         `json:"title"`
	Class      string         `json:"class"`
	Name       string         `json:"name"`
	Position   WindowPosition `json:"position"`
	Size       WindowSize     `json:"size"`
	Buttons    []WindowButton `json:"buttons"`
	Visible    bool           `json:"visible"`
	Desktop    int            `json:"desktop"`
	State      []string       `json:"state"`
	WindowType string         `json:"windowType"`
	PID        int            `json:"pid,omitempty"`
}

// WindowPosition represents the window position
type WindowPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// WindowSize represents the window dimensions
type WindowSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WindowButton represents window control buttons
type WindowButton struct {
	Name     string         `json:"name"`
	Position WindowPosition `json:"position"`
	Size     WindowSize     `json:"size"`
	Type     string         `json:"type"` // "close", "minimize", "maximize", "roll-up"
}

// GetX11Windows retrieves all visible windows using X11 APIs
func GetX11Windows() (string, error) {
	// Connect to X11 server
	conn, err := xgb.NewConn()
	if err != nil {
		return "", fmt.Errorf("failed to connect to X11 server: %w", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Get all windows
	root := screen.Root
	tree, err := xproto.QueryTree(conn, root).Reply()
	if err != nil {
		return "", fmt.Errorf("failed to query window tree: %w", err)
	}

	var windows []X11Window

	// Iterate through all top-level windows
	for _, windowID := range tree.Children {
		window, err := getWindowInfo(conn, windowID)
		if err != nil {
			log.Printf("Error getting window info for %d: %v", windowID, err)
			continue
		}

		// Only include visible windows with titles
		if window.Visible && window.Title != "" {
			windows = append(windows, window)
		}
	}

	// Create the final result
	result := X11WindowInfo{
		Windows: windows,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal window data to JSON: %w", err)
	}

	return string(jsonData), nil
}

// getWindowInfo retrieves detailed information about a single window
func getWindowInfo(conn *xgb.Conn, windowID xproto.Window) (X11Window, error) {
	window := X11Window{
		ID:      uint32(windowID),
		Visible: true,
	}

	// Get window geometry
	geom, err := xproto.GetGeometry(conn, xproto.Drawable(windowID)).Reply()
	if err != nil {
		return window, fmt.Errorf("failed to get window geometry: %w", err)
	}

	window.Position = WindowPosition{
		X: int(geom.X),
		Y: int(geom.Y),
	}
	window.Size = WindowSize{
		Width:  int(geom.Width),
		Height: int(geom.Height),
	}

	// Get window attributes
	attr, err := xproto.GetWindowAttributes(conn, windowID).Reply()
	if err != nil {
		return window, fmt.Errorf("failed to get window attributes: %w", err)
	}

	window.Visible = attr.MapState == xproto.MapStateViewable

	// Get window name (title)
	if name, err := getWindowName(conn, windowID); err == nil {
		window.Title = name
	}

	// Get window class and name
	if class, name, err := getWindowClass(conn, windowID); err == nil {
		window.Class = class
		window.Name = name
	}

	// Get window state
	if states, err := getWindowState(conn, windowID); err == nil {
		window.State = states
	}

	// Get window type
	if wType, err := getWindowType(conn, windowID); err == nil {
		window.WindowType = wType
	}

	// Get desktop/workspace
	if desktop, err := getWindowDesktop(conn, windowID); err == nil {
		window.Desktop = desktop
	}

	// Get PID if available
	if pid, err := getWindowPID(conn, windowID); err == nil {
		window.PID = pid
	}

	// Detect window buttons (this is approximate since X11 doesn't expose button info directly)
	window.Buttons = detectWindowButtons(window.Position, window.Size)

	return window, nil
}

// getWindowName retrieves the window title/name
func getWindowName(conn *xgb.Conn, windowID xproto.Window) (string, error) {
	// Try _NET_WM_NAME first (UTF-8)
	nameAtom, err := xproto.InternAtom(conn, true, uint16(len("_NET_WM_NAME")), "_NET_WM_NAME").Reply()
	if err != nil {
		return "", err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, nameAtom.Atom, xproto.AtomAny, 0, (1<<32)-1).Reply()
	if err == nil && prop.ValueLen > 0 {
		return string(prop.Value), nil
	}

	// Fallback to WM_NAME
	wmNameAtom, err := xproto.InternAtom(conn, true, uint16(len("WM_NAME")), "WM_NAME").Reply()
	if err != nil {
		return "", err
	}

	prop, err = xproto.GetProperty(conn, false, windowID, wmNameAtom.Atom, xproto.AtomAny, 0, (1<<32)-1).Reply()
	if err == nil && prop.ValueLen > 0 {
		return string(prop.Value), nil
	}

	return "", fmt.Errorf("window name not found")
}

// getWindowClass retrieves the window class and instance name
func getWindowClass(conn *xgb.Conn, windowID xproto.Window) (string, string, error) {
	classAtom, err := xproto.InternAtom(conn, true, uint16(len("WM_CLASS")), "WM_CLASS").Reply()
	if err != nil {
		return "", "", err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, classAtom.Atom, xproto.AtomString, 0, (1<<32)-1).Reply()
	if err != nil || prop.ValueLen == 0 {
		return "", "", fmt.Errorf("window class not found")
	}

	// WM_CLASS contains two null-terminated strings: instance and class
	parts := strings.Split(string(prop.Value), "\x00")
	if len(parts) >= 2 {
		return parts[1], parts[0], nil // class, instance
	} else if len(parts) == 1 {
		return parts[0], parts[0], nil
	}

	return "", "", fmt.Errorf("invalid WM_CLASS format")
}

// getWindowState retrieves the window state (maximized, minimized, etc.)
func getWindowState(conn *xgb.Conn, windowID xproto.Window) ([]string, error) {
	stateAtom, err := xproto.InternAtom(conn, true, uint16(len("_NET_WM_STATE")), "_NET_WM_STATE").Reply()
	if err != nil {
		return nil, err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, stateAtom.Atom, xproto.AtomAtom, 0, (1<<32)-1).Reply()
	if err != nil || prop.ValueLen == 0 {
		return []string{}, nil
	}

	var states []string
	for i := 0; i < int(prop.ValueLen); i++ {
		atomID := xproto.Atom(uint32(prop.Value[i*4])<<24 | uint32(prop.Value[i*4+1])<<16 | uint32(prop.Value[i*4+2])<<8 | uint32(prop.Value[i*4+3]))

		if atomName, err := getAtomName(conn, atomID); err == nil {
			states = append(states, atomName)
		}
	}

	return states, nil
}

// getWindowType retrieves the window type
func getWindowType(conn *xgb.Conn, windowID xproto.Window) (string, error) {
	typeAtom, err := xproto.InternAtom(conn, true, uint16(len("_NET_WM_WINDOW_TYPE")), "_NET_WM_WINDOW_TYPE").Reply()
	if err != nil {
		return "", err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, typeAtom.Atom, xproto.AtomAtom, 0, (1<<32)-1).Reply()
	if err != nil || prop.ValueLen == 0 {
		return "NORMAL", nil // Default type
	}

	// Get the first window type atom
	atomID := xproto.Atom(uint32(prop.Value[0])<<24 | uint32(prop.Value[1])<<16 | uint32(prop.Value[2])<<8 | uint32(prop.Value[3]))

	if atomName, err := getAtomName(conn, atomID); err == nil {
		return atomName, nil
	}

	return "NORMAL", nil
}

// getWindowDesktop retrieves the desktop/workspace number
func getWindowDesktop(conn *xgb.Conn, windowID xproto.Window) (int, error) {
	desktopAtom, err := xproto.InternAtom(conn, true, uint16(len("_NET_WM_DESKTOP")), "_NET_WM_DESKTOP").Reply()
	if err != nil {
		return 0, err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, desktopAtom.Atom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || prop.ValueLen == 0 {
		return 0, nil // Default to desktop 0
	}

	if len(prop.Value) >= 4 {
		desktop := uint32(prop.Value[0])<<24 | uint32(prop.Value[1])<<16 | uint32(prop.Value[2])<<8 | uint32(prop.Value[3])
		return int(desktop), nil
	}

	return 0, nil
}

// getWindowPID retrieves the process ID of the window
func getWindowPID(conn *xgb.Conn, windowID xproto.Window) (int, error) {
	pidAtom, err := xproto.InternAtom(conn, true, uint16(len("_NET_WM_PID")), "_NET_WM_PID").Reply()
	if err != nil {
		return 0, err
	}

	prop, err := xproto.GetProperty(conn, false, windowID, pidAtom.Atom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || prop.ValueLen == 0 {
		return 0, nil
	}

	if len(prop.Value) >= 4 {
		pid := uint32(prop.Value[0])<<24 | uint32(prop.Value[1])<<16 | uint32(prop.Value[2])<<8 | uint32(prop.Value[3])
		return int(pid), nil
	}

	return 0, nil
}

// getAtomName retrieves the name of an X11 atom
func getAtomName(conn *xgb.Conn, atom xproto.Atom) (string, error) {
	name, err := xproto.GetAtomName(conn, atom).Reply()
	if err != nil {
		return "", err
	}
	return name.Name, nil
}

// detectWindowButtons approximates window button positions based on window geometry
func detectWindowButtons(pos WindowPosition, size WindowSize) []WindowButton {
	var buttons []WindowButton

	// Standard window button size and positioning
	buttonWidth := 24
	buttonHeight := 24
	buttonSpacing := 8
	headerHeight := 32

	// Only add buttons if window is large enough to have a title bar
	if size.Height < headerHeight+50 || size.Width < buttonWidth*4 {
		return buttons
	}

	// Calculate button positions (right-aligned)
	rightEdge := pos.X + size.Width - 8

	// Close button (red, usually rightmost)
	buttons = append(buttons, WindowButton{
		Name: "close",
		Position: WindowPosition{
			X: rightEdge - buttonWidth,
			Y: pos.Y + (headerHeight-buttonHeight)/2,
		},
		Size: WindowSize{
			Width:  buttonWidth,
			Height: buttonHeight,
		},
		Type: "close",
	})

	// Maximize button (green)
	rightEdge -= buttonWidth + buttonSpacing
	buttons = append(buttons, WindowButton{
		Name: "maximize",
		Position: WindowPosition{
			X: rightEdge - buttonWidth,
			Y: pos.Y + (headerHeight-buttonHeight)/2,
		},
		Size: WindowSize{
			Width:  buttonWidth,
			Height: buttonHeight,
		},
		Type: "maximize",
	})

	// Minimize button (yellow)
	rightEdge -= buttonWidth + buttonSpacing
	buttons = append(buttons, WindowButton{
		Name: "minimize",
		Position: WindowPosition{
			X: rightEdge - buttonWidth,
			Y: pos.Y + (headerHeight-buttonHeight)/2,
		},
		Size: WindowSize{
			Width:  buttonWidth,
			Height: buttonHeight,
		},
		Type: "minimize",
	})

	// Roll-up/Shade button (if space permits)
	if size.Width > buttonWidth*5 {
		rightEdge -= buttonWidth + buttonSpacing
		buttons = append(buttons, WindowButton{
			Name: "roll-up",
			Position: WindowPosition{
				X: rightEdge - buttonWidth,
				Y: pos.Y + (headerHeight-buttonHeight)/2,
			},
			Size: WindowSize{
				Width:  buttonWidth,
				Height: buttonHeight,
			},
			Type: "roll-up",
		})
	}

	return buttons
}
