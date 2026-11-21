package x11

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	Active     bool           `json:"active"`
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

// setDisplay sets the DISPLAY environment variable
func setDisplay(display string) error {
	if display == "" {
		return nil
	}

	// Set DISPLAY environment variable for X11 connection
	return os.Setenv("DISPLAY", display)
}

// GetX11Windows retrieves all visible windows using X11 APIs
func GetX11Windows() (string, error) {
	return GetX11WindowsWithDisplay("")
}

// GetX11WindowsWithDisplay retrieves all visible windows using X11 APIs with specified display
func GetX11WindowsWithDisplay(display string) (string, error) {
	// Set DISPLAY environment variable if specified
	if display != "" {
		// Note: xgb.NewConn() uses DISPLAY environment variable internally
		// We need to set it before creating the connection
		if err := setDisplay(display); err != nil {
			return "", fmt.Errorf("failed to set display: %w", err)
		}
	}

	// Connect to X11 server
	conn, err := xgb.NewConn()
	if err != nil {
		return "", fmt.Errorf("failed to connect to X11 server: %w", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Get all managed windows via EWMH (_NET_CLIENT_LIST_STACKING / _NET_CLIENT_LIST), falling back to QueryTree
	root := screen.Root
	windowIDs, err := getClientWindowList(conn, root)
	if err != nil {
		tree, treeErr := xproto.QueryTree(conn, root).Reply()
		if treeErr != nil {
			return "", fmt.Errorf("failed to query window tree: %w", treeErr)
		}
		windowIDs = tree.Children
	}

	activeWindowID, _ := getActiveWindow(conn, root)

	var windows []X11Window

	// Iterate through all windows
	for _, windowID := range windowIDs {
		window, err := getWindowInfo(conn, root, windowID, activeWindowID)
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

// getClientWindowList retrieves managed client windows using EWMH properties.
func getClientWindowList(conn *xgb.Conn, root xproto.Window) ([]xproto.Window, error) {
	for _, atomName := range []string{"_NET_CLIENT_LIST_STACKING", "_NET_CLIENT_LIST"} {
		atom, err := xproto.InternAtom(conn, true, uint16(len(atomName)), atomName).Reply()
		if err != nil || atom.Atom == 0 {
			continue
		}
		prop, err := xproto.GetProperty(conn, false, root, atom.Atom, xproto.AtomWindow, 0, (1<<32)-1).Reply()
		if err != nil || prop.ValueLen == 0 {
			continue
		}
		count := int(prop.ValueLen)
		windows := make([]xproto.Window, 0, count)
		for i := 0; i < count && (i+1)*4 <= len(prop.Value); i++ {
			windows = append(windows, xproto.Window(xgb.Get32(prop.Value[i*4:])))
		}
		if len(windows) > 0 {
			return windows, nil
		}
	}
	return nil, fmt.Errorf("no EWMH client list property found")
}

// getActiveWindow retrieves the currently focused window ID from _NET_ACTIVE_WINDOW.
func getActiveWindow(conn *xgb.Conn, root xproto.Window) (xproto.Window, error) {
	atom, err := xproto.InternAtom(conn, true, uint16(len("_NET_ACTIVE_WINDOW")), "_NET_ACTIVE_WINDOW").Reply()
	if err != nil || atom.Atom == 0 {
		return 0, err
	}
	prop, err := xproto.GetProperty(conn, false, root, atom.Atom, xproto.AtomWindow, 0, 1).Reply()
	if err != nil || prop.ValueLen == 0 || len(prop.Value) < 4 {
		return 0, fmt.Errorf("active window not found")
	}
	return xproto.Window(xgb.Get32(prop.Value)), nil
}

// getFrameExtents retrieves _NET_FRAME_EXTENTS (left, right, top, bottom) for a window.
func getFrameExtents(conn *xgb.Conn, windowID xproto.Window) (left, right, top, bottom int) {
	atom, err := xproto.InternAtom(conn, true, uint16(len("_NET_FRAME_EXTENTS")), "_NET_FRAME_EXTENTS").Reply()
	if err != nil || atom.Atom == 0 {
		return 0, 0, 0, 0
	}
	prop, err := xproto.GetProperty(conn, false, windowID, atom.Atom, xproto.AtomCardinal, 0, 4).Reply()
	if err != nil || prop.ValueLen < 4 || len(prop.Value) < 16 {
		return 0, 0, 0, 0
	}
	return int(xgb.Get32(prop.Value[0:])),
		int(xgb.Get32(prop.Value[4:])),
		int(xgb.Get32(prop.Value[8:])),
		int(xgb.Get32(prop.Value[12:]))
}

// getWindowInfo retrieves detailed information about a single window
func getWindowInfo(conn *xgb.Conn, root xproto.Window, windowID xproto.Window, activeWindowID xproto.Window) (X11Window, error) {
	window := X11Window{
		ID:      uint32(windowID),
		Visible: true,
		Active:  windowID == activeWindowID && activeWindowID != 0,
	}

	// Get window geometry
	geom, err := xproto.GetGeometry(conn, xproto.Drawable(windowID)).Reply()
	if err != nil {
		return window, fmt.Errorf("failed to get window geometry: %w", err)
	}

	// Translate to absolute screen coordinates relative to root
	absX := int(geom.X)
	absY := int(geom.Y)
	if coords, err := xproto.TranslateCoordinates(conn, windowID, root, 0, 0).Reply(); err == nil {
		absX = int(coords.DstX)
		absY = int(coords.DstY)
	}

	// Include WM frame extents if present so position/size cover the title bar and borders
	frameLeft, frameRight, frameTop, frameBottom := getFrameExtents(conn, windowID)
	window.Position = WindowPosition{
		X: absX - frameLeft,
		Y: absY - frameTop,
	}
	window.Size = WindowSize{
		Width:  int(geom.Width) + frameLeft + frameRight,
		Height: int(geom.Height) + frameTop + frameBottom,
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
		for _, s := range states {
			if s == "_NET_WM_STATE_HIDDEN" {
				window.Visible = false
				break
			}
		}
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

	// Detect window buttons using actual WM frame top height when available
	window.Buttons = detectWindowButtons(window.Position, window.Size, frameTop)

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
	for i := 0; i < int(prop.ValueLen) && (i+1)*4 <= len(prop.Value); i++ {
		atomID := xproto.Atom(xgb.Get32(prop.Value[i*4:]))

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
	if err != nil || prop.ValueLen == 0 || len(prop.Value) < 4 {
		return "NORMAL", nil // Default type
	}

	// Get the first window type atom
	atomID := xproto.Atom(xgb.Get32(prop.Value))

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
		desktop := xgb.Get32(prop.Value)
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
		pid := xgb.Get32(prop.Value)
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
func detectWindowButtons(pos WindowPosition, size WindowSize, frameTop int) []WindowButton {
	var buttons []WindowButton

	// Standard window button size and positioning
	buttonWidth := 24
	buttonHeight := 24
	buttonSpacing := 8
	headerHeight := frameTop
	if headerHeight <= 0 {
		headerHeight = 32
	}
	if buttonHeight > headerHeight {
		buttonHeight = headerHeight
	}

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
