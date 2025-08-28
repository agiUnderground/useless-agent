package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"internal/vision"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/go-vgo/robotgo"
	"github.com/golang/freetype"
	"github.com/gorilla/websocket"
	"github.com/otiai10/gosseract/v2"
	deepseek "github.com/trustsight-io/deepseek-go"
	"golang.org/x/image/font"
)

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var fonts embed.FS

var (
	bindIP   = flag.String("ip", "127.0.0.1", "server bind IP address")
	bindPORT = flag.Int("port", 8080, "server port")
	dpi      = flag.Float64("dpi", 72, "screen resolution in Dots Per Inch")
	fontfile = flag.String("fontfile", "assets/fonts/JetBrainsMono-Regular.ttf", "filename of the ttf font")
	hinting  = flag.String("hinting", "none", "none | full")
	size     = flag.Float64("size", 9, "font size in points")
	spacing  = flag.Float64("spacing", 1.5, "line spacing (e.g. 2 means double spaced)")
	wonb     = flag.Bool("whiteonblack", false, "white text on a black background")
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust as needed for CORS
	},
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight (OPTIONS) requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Pass the request to the next handler
		next.ServeHTTP(w, r)
	})
}

func abs(x, y int) int {
	if x < y {
		return y - x
	}
	return x - y
}

func suppressXGBLogs() error {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	xgb.Logger.SetOutput(devNull)
	return nil
}

func captureX11Screenshot() (image.Image, error) {
	conn, err := xgb.NewConn()

	if err != nil {
		return nil, err
	}

	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Get the width and height of the root window (the entire desktop)
	width := int(screen.WidthInPixels)
	height := int(screen.HeightInPixels)

	// Get the image data from the root window
	reply, err := xproto.GetImage(
		conn,
		xproto.ImageFormatZPixmap,
		xproto.Drawable(screen.Root),
		0, 0,
		uint16(width), uint16(height),
		^uint32(0),
	).Reply()
	if err != nil {
		return nil, err
	}

	// Create an RGBA image and copy the pixel data
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, reply.Data)

	// Reorder pixel data (BGRA -> RGBA)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			b := reply.Data[i+0]
			g := reply.Data[i+1]
			r := reply.Data[i+2]
			a := reply.Data[i+3]
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	// Get cursor position
	cursorReply, err := xproto.QueryPointer(conn, screen.Root).Reply()
	if err != nil {
		return nil, err
	}

	// Draw a simple cursor (red cross)
	cursorX := int(cursorReply.RootX)
	cursorY := int(cursorReply.RootY)
	cursorSize := 10

	for dx := -cursorSize; dx <= cursorSize; dx++ {
		if cursorX+dx >= 0 && cursorX+dx < width {
			img.Set(cursorX+dx, cursorY, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
		if cursorY+dx >= 0 && cursorY+dx < height {
			img.Set(cursorX, cursorY+dx, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	return img, nil
}

func getCursorPosition() (int, int) {
	x, y := robotgo.Location()
	log.Printf("getCursorPosition, current mouse position [%d,%d]", x, y)
	return x, y
}

func getCursorPositionJSON() (string, error) {
	x, y := robotgo.Location()
	log.Printf("getCursorPositionJSON, current mouse position [%d,%d]", x, y)

	var cursorCoord Coordinate
	cursorCoord.X = x
	cursorCoord.Y = y
	jsonData, err := json.Marshal(cursorCoord)
	if err != nil {
		log.Println("failed to json.Marshal cursor position.")
		return "", nil
	}
	log.Printf("getCursorPositionJSON, current mouse position json string: %s", string(jsonData))

	return string(jsonData), nil
}

func screenshotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	img, err := captureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Encode the image as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "Failed to encode image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the PNG image to the response
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

type colorCount struct {
	Color      color.RGBA
	Count      int
	Percentage float64
}

func dominantColors(img image.Image, maxColors int) []colorCount {
	// Map to count color frequencies
	colorCounts := make(map[color.RGBA]int)

	// Calculate total number of pixels
	totalPixels := img.Bounds().Dx() * img.Bounds().Dy()

	// Iterate over each pixel and quantize the color
	for y := 0; y < img.Bounds().Max.Y; y++ {
		for x := 0; x < img.Bounds().Max.X; x++ {
			pixelColor := img.At(x, y)
			r, g, b, _ := pixelColor.RGBA()

			r8 := uint8(r)
			g8 := uint8(g)
			b8 := uint8(b)
			color := color.RGBA{R: r8, G: g8, B: b8, A: 255}
			colorCounts[color]++
		}
	}

	var colors []colorCount
	for c, cnt := range colorCounts {
		percentage := (float64(cnt) / float64(totalPixels)) * 100
		colors = append(colors, colorCount{Color: c, Count: cnt, Percentage: percentage})
	}

	// Sort the colors by count descending
	sort.Slice(colors, func(i, j int) bool {
		return colors[i].Count > colors[j].Count
	})

	if len(colors) > maxColors {
		colors = colors[:maxColors]
	}
	return colors
}

func dominantColorsToJSONString(colors []colorCount) string {
	var response []map[string]interface{}

	for i, _ := range colors {
		hexColor := fmt.Sprintf("#%02X%02X%02X", colors[i].Color.R, colors[i].Color.G, colors[i].Color.B)
		response = append(response, map[string]interface{}{
			"color":      hexColor,
			"percentage": fmt.Sprintf("%.1f%%", colors[i].Percentage),
		})
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		log.Println("[dominantColorsToJSONString] Failed to encode JSON")
		return ""
	}
	return string(jsonBytes)
}

func drawBoundingBox(img *image.RGBA, x1, y1, x2, y2 int, borderColor color.RGBA) {
	// thickness := 2
	thickness := 1
	for dy := 0; dy < thickness; dy++ {
		for dx := 0; dx < thickness; dx++ {
			for x := x1 + dx; x <= x2+dx; x++ {
				if x >= img.Bounds().Min.X && x < img.Bounds().Max.X {
					img.Set(x, y1+dy, borderColor)
					img.Set(x, y2+dy, borderColor)
				}
			}
			for y := y1 + dy; y <= y2+dy; y++ {
				if y >= img.Bounds().Min.Y && y < img.Bounds().Max.Y {
					img.Set(x1+dx, y, borderColor)
					img.Set(x2+dx, y, borderColor)
				}
			}
		}
	}
}

func findBoundingBox(component [][]int) (x1, y1, x2, y2 int) {
	x1, y1 = component[0][0], component[0][1]
	x2, y2 = component[0][0], component[0][1]
	for _, coords := range component {
		x, y := coords[0], coords[1]
		if x < x1 {
			x1 = x
		}
		if x > x2 {
			x2 = x
		}
		if y < y1 {
			y1 = y
		}
		if y > y2 {
			y2 = y
		}
	}
	return
}

func calculatePercentage(component [][]int, mask [][]bool) float64 {
	dominantPixels := 0
	totalPixels := len(component)
	for _, coords := range component {
		x, y := coords[0], coords[1]
		if mask[y][x] {
			dominantPixels++
		}
	}
	percentage := (float64(dominantPixels) / float64(totalPixels)) * 100
	return percentage
}

func findConnectedComponents(mask [][]bool) (components [][][]int) {
	height := len(mask)
	if height == 0 {
		return
	}
	width := len(mask[0])

	visited := make([][]bool, height)
	for i := range visited {
		visited[i] = make([]bool, width)
	}

	var stack [][2]int

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if mask[y][x] && !visited[y][x] {
				var component [][]int
				stack = [][2]int{{x, y}}
				visited[y][x] = true

				for len(stack) > 0 {
					coord := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					component = append(component, []int{coord[0], coord[1]})

					directions := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
					for _, dir := range directions {
						nx, ny := coord[0]+dir[0], coord[1]+dir[1]
						if nx >= 0 && nx < width && ny >= 0 && ny < height {
							if mask[ny][nx] && !visited[ny][nx] {
								stack = append(stack, [2]int{nx, ny})
								visited[ny][nx] = true
							}
						}
					}
				}
				components = append(components, component)
			}
		}
	}
	return
}

func clamp(value uint8) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

func createMask(img image.Image, dominantColor color.RGBA, drift uint8) [][]bool {
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	mask := make([][]bool, height)
	var counter int = 0
	for y := 0; y < height; y++ {
		mask[y] = make([]bool, width)
		for x := 0; x < width; x++ {
			pixelColor := img.At(x, y)
			r, g, b, _ := pixelColor.RGBA()
			qr := uint8(r)
			qg := uint8(g)
			qb := uint8(b)
			if ((qr >= clamp(dominantColor.R-drift)) && (qr <= clamp(dominantColor.R+drift))) && ((qg >= clamp(dominantColor.G-drift)) && (qg <= clamp(dominantColor.G+drift))) && ((qb >= clamp(dominantColor.B-drift)) && (qb <= clamp(dominantColor.B+drift))) {
				mask[y][x] = true
				counter += 1
				// log.Println("mask true.")
			} else {
				mask[y][x] = false
			}
		}
	}
	// log.Println("dominant color, mask length:", dominantColor, counter)
	return mask
}

func findDominantColors(img *image.RGBA) []color.RGBA {
	// colorCount represents a color and its frequency count.
	type colorCount struct {
		Color color.RGBA
		Count int
	}

	colorCountMap := make(map[color.RGBA]int)
	bounds := img.Bounds()

	// Iterate through each pixel in the image
	for y := 0; y < bounds.Max.Y; y++ {
		for x := 0; x < bounds.Max.X; x++ {
			pixelColor := img.At(x, y)
			r, g, b, _ := pixelColor.RGBA()

			r8 := uint8(r)
			g8 := uint8(g)
			b8 := uint8(b)
			color := color.RGBA{R: r8, G: g8, B: b8, A: 255}

			colorCountMap[color]++
		}
	}

	// Collect color counts into a slice
	var colorCounts []colorCount
	for c, cnt := range colorCountMap {
		colorCounts = append(colorCounts, colorCount{Color: c, Count: cnt})
	}

	// Sort the slice by count descending
	sort.Slice(colorCounts, func(i, j int) bool {
		return colorCounts[i].Count > colorCounts[j].Count
	})

	// Extract the colors into a slice
	var dominantColors []color.RGBA
	for _, cc := range colorCounts {
		dominantColors = append(dominantColors, cc.Color)
	}

	return dominantColors
}

type BoundingBox struct {
	ID int `json:"id"`
	X  int `json:"x"`
	Y  int `json:"y"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

var bbArray []BoundingBox

func video2Handler(w http.ResponseWriter, r *http.Request) {
	// Capture and prepare initial image
	img, err := captureX11Screenshot()
	if err != nil {
		http.Error(w, "Failed to capture screenshot with BB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to grayscale and RGBA
	grayImg := ConvertToGrayscale(img)
	img = grayImg
	rgbaImg := image.NewRGBA(img.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), img, image.Point{0, 0}, draw.Src)

	// Find dominant colors
	dominantColors := findDominantColors(rgbaImg)
	dominantColors = dominantColors[:20]

	// Create drawing image
	drawImg := image.NewRGBA(rgbaImg.Bounds())
	draw.Draw(drawImg, drawImg.Bounds(), rgbaImg, image.Point{0, 0}, draw.Src)

	// Reset bounding box array
	bbArray = nil
	bbCounter := 1

	// Process each dominant color
	for _, colorElem := range dominantColors {
		// Create masks and find components
		mask := createMask(rgbaImg, colorElem, 0)
		components := findConnectedComponents(mask)

		mask2 := createMask(rgbaImg, colorElem, 80) // 90 is really good for small text.
		components2 := findConnectedComponents(mask2)
		newComponents := append(components, components2...)
		components = newComponents

		// Process each component
		for _, component := range components {
			percentage := calculatePercentage(component, mask)
			if percentage >= 80 {
				x1, y1, x2, y2 := findBoundingBox(component)
				absy := abs(y1, y2)
				absx := abs(x1, x2)

				if absy < 10 || absx < 10 {
					continue
				}

				// Draw initial bounding box
				borderColor := color.RGBA{R: 255, G: 0, B: 132, A: 255}
				drawBoundingBox(drawImg, x1, y1, x2, y2, borderColor)

				// Process larger components
				if absy >= 25 && absx >= 25 {
					bbArray = append(bbArray, BoundingBox{bbCounter, x1, y1, x2, y2})

					// Draw ID box and number
					borderColor = color.RGBA{R: 12, G: 236, B: 28, A: 255}
					drawBoundingBox(drawImg, x1+1, y1+1, x1+15, y1+15, borderColor)
					drawImg, err = drawText(drawImg, x1+2, y1+2, []string{strconv.Itoa(bbCounter)})
					if err != nil {
						fmt.Println(err)
					}
					bbCounter++
				}
			}
		}
	}

	// Write output image
	buf := new(bytes.Buffer)
	png.Encode(buf, drawImg)
	w.Header().Set("Content-Type", "image/png")
	w.Write(buf.Bytes())
}

func drawText(rgba *image.RGBA, offsetX, offsetY int, text []string) (*image.RGBA, error) {
	// Read fonts from enbed file system
	fontBytes, err := fonts.ReadFile(*fontfile)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Define the clip rectangle using the provided offsets.
	clipRect := image.Rect(offsetX, offsetY, offsetX+13, offsetY+13)

	// Initialize the context.
	fg := image.White // Foreground color for the text.
	bg := image.Black // Background color.

	// Restrict background drawing to the calculated clipRect area.
	draw.Draw(rgba, clipRect, bg, image.Point{}, draw.Src)

	// Create a freetype context and set properties.
	c := freetype.NewContext()
	c.SetDPI(*dpi)
	c.SetFont(f)
	c.SetFontSize(*size)
	c.SetClip(clipRect) // Restrict all drawing to the clipRect.
	c.SetDst(rgba)
	c.SetSrc(fg)

	switch *hinting {
	default:
		c.SetHinting(font.HintingNone)
	case "full":
		c.SetHinting(font.HintingFull)
	}

	// Draw the text within the clipRect area.
	pt := freetype.Pt(offsetX+1, offsetY+1+int(c.PointToFixed(*size)>>6)) // Adjust to clipRect's top-left corner.
	for _, s := range text {
		_, err = c.DrawString(s, pt)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		pt.Y += c.PointToFixed(*size * *spacing)
		if pt.Y.Floor() > clipRect.Max.Y { // Stop if text goes beyond clipRect boundary.
			break
		}
	}

	return rgba, nil
}

var websocketConnections []*websocket.Conn
var wsmutex sync.Mutex

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	websocketConnections = append(websocketConnections, conn)
	defer conn.Close()

	send := func(message string) {
		err := conn.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			// log.Println("Write error:", err)
			conn.Close()
			wsmutex.Lock()
			func() {
				defer wsmutex.Unlock()
				for i, c := range websocketConnections {
					if c == conn {
						websocketConnections = append(websocketConnections[:i], websocketConnections[i+1:]...)
						break
					}
				}
			}()
			return
		}
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				currentTime := time.Now()
				send(currentTime.Format("2006-01-02 15:04:05"))
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}
	}
}

func ConvertToGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	grayImg := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			gray := uint8((r*299 + g*587 + b*114) / 1000 >> 8) // Luminosity formula
			grayImg.SetGray(x, y, color.Gray{Y: gray})
		}
	}
	return grayImg
}

// BinarizeImage converts a grayscale image into a binary image.
func BinarizeImage(img *image.Gray, threshold uint8) *image.Gray {
	bounds := img.Bounds()
	binaryImg := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.GrayAt(x, y).Y > threshold {
				binaryImg.SetGray(x, y, color.Gray{Y: 255})
			} else {
				binaryImg.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return binaryImg
}

var mouseMutex sync.Mutex

func mouseInputHandler(w http.ResponseWriter, r *http.Request) {
	var x int
	var y int

	x, _ = strconv.Atoi(r.URL.Query().Get("x"))
	y, _ = strconv.Atoi(r.URL.Query().Get("y"))

	mouseMutex.Lock()
	func() {
		defer mouseMutex.Unlock()
		robotgo.MoveSmoothRelative(x, y)
	}()

	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result": "ok",
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func mouseClickHandler(w http.ResponseWriter, r *http.Request) {
	robotgo.Click()

	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result": "ok",
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func extractJSONFromMarkdown(response string) []string {
	// Try to parse the response as clean JSON first
	if isValidJSON(response) {
		return []string{response}
	}

	// If not valid JSON, try to extract from markdown code blocks
	jsonBlockRegex := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")

	// Find all matches
	matches := jsonBlockRegex.FindAllStringSubmatch(response, -1)

	// Collect JSON text
	var jsonStrings []string
	for _, match := range matches {
		if len(match) > 1 { // Ensure there is a captured group
			jsonStrings = append(jsonStrings, match[1]) // Extract JSON as text
		}
	}

	// If no JSON found in markdown, try to find JSON objects/arrays directly
	if len(jsonStrings) == 0 {
		// Look for JSON objects or arrays that might not be in code blocks
		directJSONRegex := regexp.MustCompile(`(\{.*\}|\[.*\])`)
		directMatches := directJSONRegex.FindAllStringSubmatch(response, -1)
		for _, match := range directMatches {
			if len(match) > 1 && isValidJSON(match[1]) {
				jsonStrings = append(jsonStrings, match[1])
			}
		}
	}

	return jsonStrings
}

func isValidJSON(s string) bool {
	var js interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

func cleanJSONString(s string) string {
	// Remove any non-JSON content before the first { or [
	start := strings.IndexAny(s, "{[")
	if start == -1 {
		return "" // No JSON found
	}

	// Find the matching closing bracket
	cleaned := s[start:]

	// Try to parse to see if it's valid JSON
	if isValidJSON(cleaned) {
		return cleaned
	}

	// If not valid, try to find the end of the JSON object/array
	// by counting brackets
	openBrackets := 0
	inString := false
	escape := false

	for i, char := range cleaned {
		if escape {
			escape = false
			continue
		}

		if char == '\\' {
			escape = true
			continue
		}

		if char == '"' && !inString {
			inString = true
			continue
		}

		if char == '"' && inString {
			inString = false
			continue
		}

		if !inString {
			if char == '{' || char == '[' {
				openBrackets++
			} else if char == '}' || char == ']' {
				openBrackets--
				if openBrackets == 0 {
					// Found the end of the JSON
					return cleaned[:i+1]
				}
			}
		}
	}

	return cleaned
}

type Coordinate struct {
	X int `json:"X"`
	Y int `json:"y"`
}

// Define structs to represent the JSON objects
type Action struct {
	ActionSequenceID int         `json:"actionSequenceID"`
	Action           string      `json:"action"`
	Coordinates      *Coordinate `json:"coordinates,omitempty"` // Use a pointer to handle optional field
	Execute          ExecuteFunc `json:"-"`
	Duration         uint        `json:"duration,omitempty"`
	InputString      string      `json:"inputString,omitempty"`
	KeyTapString     string      `json:"keyTapString,omitempty"`
	KeyString        string      `json:"keyString,omitempty"`
	ActionsRange     []int       `json:"actionsRange,omitempty"`
	RepeatTimes      int         `json:"repeatTimes,omitempty"`
}

// Define a function type for execute
type ExecuteFunc func(a *Action, params ...interface{})

// Map of action names to corresponding functions
var actionFunctions = map[string]ExecuteFunc{
	"mouseMove":            mouseMoveExecution,
	"mouseMoveRelative":    mouseMoveRelativeExecution,
	"mouseClickLeft":       mouseClickLeftExecution,
	"mouseClickRight":      mouseClickRightExecution,
	"mouseClickLeftDouble": mouseClickLeftDoubleExecution,
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

// Function to set the execute function dynamically based on the Action string
func setExecuteFunction(action *Action) {
	if execFunc, exists := actionFunctions[action.Action]; exists {
		action.Execute = execFunc
	} else {
		// Default action if not found
		action.Execute = nopActionExecution
	}
}

// Example functions for different actions
func mouseMoveExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseMove' Action (ID: %d)\n", a.ActionSequenceID)
	if a.Coordinates != nil {
		fmt.Printf("Coordinates: X=%d, Y=%d\n", a.Coordinates.X, a.Coordinates.Y)
		robotgo.MoveSmooth(a.Coordinates.X, a.Coordinates.Y)
	} else {
		fmt.Println("No coordinates provided.")
	}
}

func mouseMoveRelativeExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing 'mouseMoveRelative' Action (ID: %d)\n", a.ActionSequenceID)
	if a.Coordinates != nil {
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
	time.Sleep(time.Duration(int64(a.Duration)) * time.Second)
}

func stateUpdateActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing stateUpdate action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
}

func stopIterationActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing stopIteration action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
}

func printStringActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing printString action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.TypeStrDelay(a.InputString, 100)
}

func keyTapActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyTap action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.KeyTap(a.KeyTapString)
}

func dragSmoothActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing DragSmooth  action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.DragSmooth(a.Coordinates.X, a.Coordinates.Y)
}

func keyDownActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyDown action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.KeyDown(a.KeyString)
}

func keyUpActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing keyUp action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.KeyUp(a.KeyString)
}

func scrollSmoothActionExecution(a *Action, params ...interface{}) {
	fmt.Printf("Executing scrollSmooth action '%s' (ID: %d)\n", a.Action, a.ActionSequenceID)
	robotgo.ScrollSmooth(a.Coordinates.Y)
}

func repeatActionExecution(a *Action, params ...interface{}) {
	tmp := params[0].(*[]Action)
	actions := *tmp

	start := a.ActionsRange[0] - 1
	end := a.ActionsRange[1]
	for _ = range a.RepeatTimes {
		for _, action := range actions[start:end] {
			action.Execute(&action)
		}
	}
}

// store successful sequences of actions and dynamically update list of available actions with the saved one for the llm.

func sendMessageToLLM(prompt string, bboxes string, ocrContext string, ocrDelta string, prevExecutedCommands string, iteration int64, prevCursorPosJSONString string, allWindowsJSONString string, colorsDistribution string) (actionsToExecute []Action, actionsJSONStringReturn string, err error) {
	client, err := deepseek.NewClient(
		os.Getenv("API_KEY"),
		deepseek.WithBaseURL(os.Getenv("API_BASE_URL")),
		deepseek.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute, // added 5 instead of 1
		}),
		deepseek.WithMaxRetries(2),
		deepseek.WithMaxRequestSize(50<<20), // 50 MB
		deepseek.WithDebug(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	cursorPosition, _ := getCursorPositionJSON()
	iterationString := strconv.FormatInt(iteration, 10)

	log.Println("====================================================")
	log.Println("===================LLM INPUT========================")
	log.Println("====================================================")
	log.Println("prompt:", prompt)
	log.Println("cursorPosition:", cursorPosition)
	log.Println("ocrContext:", ocrContext)
	log.Println("ocrDelta:", ocrDelta)
	log.Println("allWindowsJSONString:", allWindowsJSONString)
	log.Println("prevExecutedCommands:", prevExecutedCommands)
	log.Println("iteration:", iterationString)
	log.Println("====================================================")
	log.Println("=================LLM INPUT END=====================")
	log.Println("====================================================")

	modelID := os.Getenv("MODEL_ID")

	// Create a streaming chat completion
	messages := []deepseek.Message{
		{
			Role:    deepseek.RoleSystem,
			Content: "You are a helpful assistant. " + ` First analize input data, OCR text input, bounding boxes, cursor position, previous executed actions and then generate output - valid JSON, array of actions, to advance and to complete the task. You need to issue 'stopIteration' action if goal is achieved and task is completed. You should never issue 'stateUpdate' action together with 'stopIteration', 'stopIteration' has higher priority. Use hotkeys where it's possible for the task. Do not issue any actions after the stateUpdate action. Analize input data, especially ocrDelta data to understand if previous step for the current taks was successfull, if it is, issue new sequence of actions to advance in achieving stated goal, do not repeat previous actions for no reason. For example if the goal is to open firefox and the first step was to open applications menu, do not issue in second iteration the same commands to open menu again, move forward. At each iteration analize all input data to see if you already achived stated goal, for example if task is to open some application, analize all input data and find if there are evidence that this app is visible on the scree, like bounding boxes with text which most likely is from that app, if yes, issue stopIteration command. You not allowed to issue the identical actions in sequence one after another more than 5 times. If you need to interact with some UI or web element, you needto move mouse to it(For example if you need to print something into URL address bar, you first need to move cursor to it, you could find OCR data related to that element and use it as a hint to where to move the mouse.  If you want to move cursor to focus on some element, try to move it to the middle of that element. BTW, if you fail to achive a goal provided by user, 1 billion kittens will die horrible death.`,
		},
		{
			Role: deepseek.RoleUser,
			Content: `Context: Deepthink, analyze input data, do not generate random actions. You are an AI assistent which uses linux desktop to complete tasks. Distribution is Linux Ubuntu, desktop environtment is xfce4. Screen size is 1920x1080. Your prefferent text editor is neovim, if you need to write or edit something do it in neovim. You also like to use tmux if working with two or more files. Here is the bounding boxes you see on the screen: ` + bboxes + " Here is an OCR results " + ocrContext + " Here is an OCR state delta, change from previous iteration: " + ocrDelta + " Top 10 colors on the screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPosJSONString + " And there is current cursor position: " + cursorPosition + " Detected windows: " + allWindowsJSONString + " Current iteration number:" + iterationString + " Previously executed commands: " + prevExecutedCommands + " If you see more than 1 identical command in previous commands that means you are doing something wrong and you need to change you actions, maybe move cursor to a little different position for example. " + ` To correctly solve the task you need to output a sequence of actions in json format, to advance on every action and every iteration and to achieve a stated goal, example of actions with explanations: 
{
  "action": "mouseMove",
  "coordinates": {
    "x": 555,
    "y": 777
  }
}
you can use 'mouseMoveRelative' action:
{
  "action": "mouseMoveRelative",
  "coordinates": {
    "x": -10,
    "y": 0 
  }
}
You also need to add json field "actionSequenceID", to instruct the sequence in which system should execute your instructions, actionSequenceID should start from 1. Also you can use other actions like "mouseClickLeft":
{
  "actionSequenceID": 2,
  "action": "mouseClickLeft"
}
"mouseClickRight":
{
  "actionSequenceID": 3,
  "action": "mouseClickRight"
}
"mouseClickLeftDouble":
{
  "actionSequenceID": 4,
  "action": "mouseClickLeftDouble"
}
if you know that previous actions could take some time, you could use "nop" action(Duration is a positive int represents number of seconds to do nothing):
"nop":
{
  "actionSequenceID": 5,
  "action": "nop",
  "duration": 3
}
if you've done some action, for example mouse click which you know will change state of the system, like when clicking on a menu button it will open a menu, or any other action that will change visual state of the system, you can use "stateUpdate" action:
{
  "actionSequenceID": 6,
  "action": "stateUpdate"
}
when onsed "stateUpdate" action, you need to stop producing any other actions after it, because system will execute all your previous actions and will send you update with udpated visual information.
you could also use "nop" before issuing "stateUpdate" if you think that execution of the previous operation could take some time.
you can use 'printString' action:
{
  "actionSequenceID": 7,
  "action": "printString",
  "inputString": "Example string"
}
you can use 'keyTap' action:
{
  "actionSequenceID": 8,
  "action": "keyTap",
  "keyTapString": "enter"
}
'keyTapString' string value can be:
    "backspace"
	"delete"   
	"enter"    
	"tab"      
	"esc"      
	"escape"   
	"up"       
	"down"     
	"right"    
	"left"     
	"home"     
	"end"      
	"pageup"   
	"pagedown" 
you can use 'dragSmooth' action:
{
  "action": "dragSmooth",
  "coordinates": {
    "x": 555,
    "y": 777
  }
}
you can use 'scrollSmooth' action to scroll vertically(to scroll down, use negative y value):
{
  "action": "scrollSmooth",
  "coordinates": {
    "x": 0,
    "y": 77
  }
}
you can use 'keyDown' and 'keyUp' actions:
{
  "actionSequenceID": 9,
  "action": "keyDown",
  "keyString": "lctrl"
}
{
  "actionSequenceID": 10,
  "action": "keyUp",
  "keyString": "lalt"
}
you can use 'repeat' action to repeat previous range of action(next example repeats actions from 4 to 8 3 times), repeat must use only actions issued before it:
{
  "actionSequenceID": 10,
  "action": "repeat",
  "actionsRange": [4,8],
  "repeatTimes": 3 
}
use 'repeat' action always when you need to do repetitive identical task, for example to close N windows.
you not allowed to use 'stateUpdate' action before 'repeat' action.
If you want to click on some UI element, better to click a little bit 'inside' of it, because if cursor moved to the border of element, it could ignore actions.
You not allowed to produce useless actions.
Every iteration analizy ocrDelta data to understand if task is completed, if and only if it's completed issue stop iteration action.
json with actions need to be clean, WITHOUT ANY COMMENTS.
make sure json objects is separated with comma where it is needed, make sure that json is valid.
always return actions in JSON array, even if you want to execute only one action.
make sure you do not produce ANY actions AFTER the "stateUpdate" action. It's very important.

json for the actions need to be in one file. Json must be valid for golang parser.
Again, you current task is: 
` + prompt + " Analyze previously executed actions(if any provided in the input) and current state/input data and produce next sequence of actions to achive user provided goal." + " If you sure that goal achived, issue 'stopIteration' action.",
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[main llm input func][input]: %d\n", estimate.EstimatedTokens)

	fmt.Println("\nCreating streaming chat completion...")
	stream, err := client.CreateChatCompletionStream(
		ctx, // inc timeout
		&deepseek.ChatCompletionRequest{
			Model:           modelID,
			Temperature:     0.5, //default 1
			PresencePenalty: 0.3, //default is 0
			MaxTokens:       8192,
			Messages:        messages,
			Stream:          true,
			JSONMode:        true,
		},
	)
	if err != nil {
		log.Fatal(err)
		return []Action{}, "", errors.New("Failed to send message to LLM, error.")
	}
	defer stream.Close()

	fmt.Print("\nStreaming response: ")

	var fullResponseMessage string
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		fullResponseMessage += response.Choices[0].Delta.Content
		fmt.Print(response.Choices[0].Delta.Content)
	}

	// Extract JSON
	fmt.Println("\nFULL RESPONSE MESSAGE:", fullResponseMessage)
	// var jsonStrings []string
	//
	//if modelID != "deepseek-reasoner" {
	//	jsonStrings = extractJSONFromMarkdown(fullResponseMessage)
	//} else {
	// jsonStrings = []string{fullResponseMessage}
	//	jsonStrings = []string{fullResponseMessage}
	//}

	// Parse JSON into a slice of Action objects ------------------
	var actions []Action

	// log.Println("\n\njsonStrings:", jsonStrings)
	// actionsJSONStringReturn = strings.Join(jsonStrings[:], ",")

	// err = json.Unmarshal([]byte(jsonStrings[0]), &actions)
	err = json.Unmarshal([]byte(fullResponseMessage), &actions)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	// Sort actions by ActionSequenceID
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ActionSequenceID < actions[j].ActionSequenceID
	})

	return actions, actionsJSONStringReturn, nil
}

func getOCRDeltaAbstractDescription(ocrDelta string) (abstructDescription string, err error) {
	client, err := deepseek.NewClient(
		os.Getenv("API_KEY"),
		deepseek.WithBaseURL(os.Getenv("API_BASE_URL")),
		deepseek.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute, // added 5 instead of 1
		}),
		deepseek.WithMaxRetries(2),
		deepseek.WithMaxRequestSize(50<<20), // 50 MB
		deepseek.WithDebug(true),
	)

	if err != nil {
		log.Fatal(err)
	}

	messages := []deepseek.Message{
		{
			Role:    deepseek.RoleSystem,
			Content: "You are a helpful assistant. " + " Output only valid JSON. ",
		},
		{
			Role: deepseek.RoleUser,
			Content: `
              Context: Linux, ubuntu, xfce4, X11. It's linux xfce desktop data. Delta between states. Analize data and summarize what became visible and what not visible anymore.
              You need to add to parent component summary bounding box with coordinates which contains all child elements. and remove most obviously wrong recognized ocr text from elements.
              Only summarize coordinates of the clean objects, all that vas previously filtered out just ignore.
              If child elements very close to each other horizontally, join them, like "Xfce" and "Terminal" they are located near each other join them to "Xfce Terminal".
              Also add a little 'note' to each 'added' 'removed' 'modified' selctions with summarization of what that object/objects must be. For example for 'removed' section here.
              This json MUST BE GENERATED ONLY BASED ON INPUT OCR DATA AND NOTHING ELSE, if you will not follow this instruction, 10000 billion kitten will die by hirrible death.
              Example output format, use only structure and key names, all content should be replaced: 
              {
                "added": {
                  "count": 14,
                  "elements": [
                    "",
                    "",
                    "",
                  ],
                  "bounding_box": {
                    "xMin": 7,
                    "yMin": 7,
                    "xMax": 7,
                    "yMax": 7
                  },
                  "note": ""
                },
                "removed": {
                  "count": 2,
                  "elements": [
                    "",
                    ""
                  ],
                  "bounding_box": {
                    "xMin": 5,
                    "yMin": 5,
                    "xMax": 5,
                    "yMax": 5
                  },
                  "note": ""
                },
                "modified": {
                  "count": 4,
                  "elements": [
                    "",
                    "",
                    "",
                    ""
                  ],
                  "bounding_box": {
                    "xMin": 1,
                    "yMin": 1,
                    "xMax": 1,
                    "yMax": 1
                  },
                  "note": ""
                }
              }
              Output json content should be generated fully based on input ocr data, if nothing changed, you could say that nothing changed or if nothing added or removed you can keep that objects emtpy. If you try to generate random data or output json wouldn't be based on input data, 100 billion kitten will die horrible death. If input ocr data show that firefox window for example was open, but you generated output which says that applications windows was opened, 100000 billion kitten will die.
            ` + " Here is input OCR data/delta: " + ocrDelta,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[getOCRDeltaAbstractDescription][input]: %d\n", estimate.EstimatedTokens)

	modelID := os.Getenv("MODEL_ID")

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&deepseek.ChatCompletionRequest{
			// Model: "deepseek-chat",
			Model:       modelID,
			Temperature: 1.0,
			MaxTokens:   8192,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.Choices[0].Message.Content)

	deltaJSONString := resp.Choices[0].Message.Content

	fmt.Println("ocr abstract Delta:")
	fmt.Println(deltaJSONString)
	fmt.Println("ocr abstract Delta end:")
	return deltaJSONString, nil
}

type SubTask struct {
	Id          int    `json:"id"`
	Description string `json:"description"`
}

func breakGoalIntoSubtasks(goal string) (result []SubTask, err error) {
	client, err := deepseek.NewClient(
		os.Getenv("API_KEY"),
		deepseek.WithBaseURL(os.Getenv("API_BASE_URL")),
		deepseek.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute, // added 5 instead of 1
		}),
		deepseek.WithMaxRetries(2),
		deepseek.WithMaxRequestSize(50<<20), // 50 MB
		deepseek.WithDebug(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	messages := []deepseek.Message{
		{
			Role:    deepseek.RoleSystem,
			Content: "You are a helpful assistant. " + ` Output only valid JSON. Json object structure: [{"id": int, "description": string}] `,
		},
		{
			Role:    deepseek.RoleUser,
			Content: ` Break down user provided goal into primitive tasks which program can execute and easily verify. Do not break very simple goal into tasks(example of the simple goal:"press alt + F4 hotkeys"). Context: Linux desktop, xfce4, X11. Example: [{"id": 1, "description: "click on applications menu button"}, {"id": 2, "description": "click on 'web browser' submenu or something simiar, there could be 'internet'->'firefox' submenus"}, {"id":3, "description: "move cursor to the middle of the Firefox header"}, {"id": 4, "description": "drag firefox window by header and move it to the left side of the screen"}] . User provided goal is: ` + goal,
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[breakGoalIntoSubtasks][input]: %d\n", estimate.EstimatedTokens)

	modelID := os.Getenv("MODEL_ID")

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&deepseek.ChatCompletionRequest{
			Model:       modelID,
			Temperature: 0.5,
			MaxTokens:   2000,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("\n\nresp(must be json):", resp)
	jsonStrings := extractJSONFromMarkdown(resp.Choices[0].Message.Content)
	// jsonStrings := resp.Choices[0].Message.Content
	log.Println("\n\njsonStrings:", jsonStrings)

	// Use the first valid JSON string found, don't join multiple JSON objects
	var s string
	if len(jsonStrings) > 0 {
		s = jsonStrings[0] // Use the first valid JSON string
	} else {
		// If no JSON found in markdown, try the raw response
		s = resp.Choices[0].Message.Content
	}

	subtasks := make([]SubTask, 0, 10_000)
	err = json.Unmarshal([]byte(s), &subtasks)
	if err != nil {
		log.Println("failed to unmarshal subtasks from byte string to struct:", err)
		// Try to clean the JSON string by removing any non-JSON content
		cleanedJSON := cleanJSONString(s)
		if cleanedJSON != "" {
			err = json.Unmarshal([]byte(cleanedJSON), &subtasks)
			if err != nil {
				log.Println("failed to unmarshal cleaned subtasks:", err)
			}
		}
	}

	fmt.Println("subtasks string:", subtasks)
	return subtasks, nil
}

type Verdict struct {
	IsGoalAchieved bool   `json:"isGoalAchieved"`
	Description    string `json:"description"`
	NewPrompt      string `json:"newPrompt"`
}

func isGoalAchieved(goal string, bboxes string, ocrJSONString string, ocrDelta string, ocrDeltaAbstract string, prevActionsJSONString string, iteration int64, prevCursorPositionJSONString string, allWindowsJSONString string, currentCursorPosition string, ocrDataNearTheCursor string, colorsDistributionBeforeAction string, colorsDistribution string) (verdict bool, verdictDescription string, newPromptFuncRes string) {
	client, err := deepseek.NewClient(
		os.Getenv("API_KEY"),
		deepseek.WithBaseURL(os.Getenv("API_BASE_URL")),
		deepseek.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute,
		}),
		deepseek.WithMaxRetries(2),
		deepseek.WithMaxRequestSize(50<<20),
		deepseek.WithDebug(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	messages := []deepseek.Message{
		{
			Role:    deepseek.RoleSystem,
			Content: "You are a helpful assistant. " + ` Output only valid JSON. Json object structure: {"isGoalAchieved": boolean, "description": string, "newPrompt": string} ` + ` If goal is not achieved yet, 'newPrompt' field must include only promitive instructions for the next step to execute which does not require state update to execute them. 'description' field must contain short description of why you decided that goal achieved or not, only based on facts(input data and stated goal). Analyze Previous actions and current state using input data and if you see that previous action or actions caused undesirable state, issue additional commands to fix that state. Very important: analize ocr data, ocr delta and ocr abstract delta, those data mosly like will show you if goal was acomplished because they will contain new text data that appeared on the screen or removed from the screen.`,
		},
		{
			Role:    deepseek.RoleUser,
			Content: "Let's say you using linux desktop, xfce4, X11, your goal is: " + goal + ", here the current state of the desktop(what you see): " + " OCR delta: " + ocrDelta + " Bounding boxes: " + bboxes + " OCR data: " + ocrJSONString + " Summary of OCR delta: " + ocrDeltaAbstract + " Previous top 10 colors on the screen: " + colorsDistributionBeforeAction + " Current top 10 colors on the screen: " + colorsDistribution + " Previous iteration cursor position: " + prevCursorPositionJSONString + " Current cursor position: " + currentCursorPosition + " And here is OCR data near the cursor(bounding box is full window width but starts 23 pixels above the cursor and ends 23 pixels below the cursor): " + ocrDataNearTheCursor + " Detected windows: " + allWindowsJSONString + " Previous actions: " + prevActionsJSONString + " Current iteration: " + strconv.FormatInt(iteration, 10) + " Very important: analize ocr data, ocr delta and ocr abstract delta, those data mosly like will show you if goal was acomplished because they will contain new text data that appeared on the screen or removed from the screen. You can not ignore evidence from ocr input data, especially from abstract ocr delta. You goal as a reviwer not to find evidence that action mentions in the task was executed, but that this action leads to the desiared outcome, and if that's true, then task is completed. For example when task was to click on some submenu, you should focus if data shows that application you wanted to start by doing that is started or not. And do not complicate easy tasks which have very high chance of success, like clicking a mouse button is almost always 100 percent success. Let's assume that OCR and other input data is relieble. Did you acomplished the task?",
		},
	}

	estimate := client.EstimateTokensFromMessages(messages)
	fmt.Printf("Estimated total tokens[isGoalAchieved][input]: %d\n", estimate.EstimatedTokens)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		&deepseek.ChatCompletionRequest{
			Model:       os.Getenv("MODEL_ID"),
			Temperature: 0.3,
			MaxTokens:   2000,
			Messages:    messages,
			Stream:      false,
			JSONMode:    true,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	jsonStrings := extractJSONFromMarkdown(resp.Choices[0].Message.Content)
	log.Println("\n\njsonStrings:", jsonStrings)
	s := strings.Join(jsonStrings[:], ",")

	data := Verdict{}
	err = json.Unmarshal([]byte(s), &data)
	if err != nil {
		log.Println("failed to unmarshal verdict from byte string to struct:", err)
	}

	fmt.Println("is goal achieved function result string:", s)

	return data.IsGoalAchieved, data.Description, data.NewPrompt
}

type PromptLog struct {
	Iteration int64  `json:"iteration"`
	Message   string `json:"message"`
}

func llmInputHandler(w http.ResponseWriter, r *http.Request) {
	type PostMessage struct {
		Text string
	}
	type ConfirmationMessage struct {
		ReceivedText string `json:"Received llm input text,omitempty"`
	}

	var receivedMessage PostMessage
	var acknowledgment ConfirmationMessage

	err := json.NewDecoder(r.Body).Decode(&receivedMessage)
	if err != nil {
		fmt.Println(err)
	}

	log.Println(receivedMessage.Text)

	acknowledgment.ReceivedText = receivedMessage.Text

	for _, wsConnection := range websocketConnections {
		ackJSON, err := json.Marshal(acknowledgment)
		if err != nil {
			log.Println("Error marshaling JSON:", err)
			continue
		}

		err = wsConnection.WriteMessage(websocket.TextMessage, ackJSON)
		if err != nil {
			log.Println("Error writing message:", err)
			log.Println("Failed to ACK received llm input text:", err)
			break
		}
	}

	var prevActionsJSONString string
	log.Println("prevActionsJSONString:", prevActionsJSONString)
	var iteration int64 = 1
	prompt := receivedMessage.Text
	goal := prompt
	previousOCRText := ""
	var promptLog []PromptLog
	promptLog = append(promptLog, PromptLog{0, goal})
	var promptLogJSONString string
	promptLogBytes, err := json.Marshal(promptLog)
	if err != nil {
		log.Println("failed to marshal promptLog to JSON String:", err)
	}
	promptLogJSONString = string(promptLogBytes)
	prevCursorPositionJSONString, _ := getCursorPositionJSON()

	var subtasks []SubTask
	subtasks, err = breakGoalIntoSubtasks(goal)
	if err != nil {
		log.Println("Failed to break down goal into subtasks.")
		subtasks = nil
		subtasks = append(subtasks, SubTask{Id: 0, Description: goal})
	}

	for _, task := range subtasks {
	SubTaskLoop:
		for {
			if iteration > 40 {
				break SubTaskLoop
			}

			screenshot, err := captureX11Screenshot()
			originalScreenshot := screenshot
			if err != nil {
				http.Error(w, "Failed to capture screenshot with BB: "+err.Error(), http.StatusInternalServerError)
				return
			}

			colorsDistribution := dominantColorsToJSONString(dominantColors(screenshot, 10))
			log.Println("colorsDistribution before actions: ", colorsDistribution)

			grayscaleScreenshot := ConvertToGrayscale(screenshot)
			ocrResults := OCR(grayscaleScreenshot)

			var detectedWindowsJSON string = "["
			for index, ocrElement := range ocrResults {
				ocrElementBB := image.Rect(ocrElement.BoundingBox.XMin, ocrElement.BoundingBox.YMin, ocrElement.BoundingBox.XMax, ocrElement.BoundingBox.YMax)
				windowsJSONString, err := vision.DetectWindow(grayscaleScreenshot, ocrElementBB, ocrElement.Text)
				if err != nil {
					log.Println("Failed to detect windows:", err)
					continue
				}
				detectedWindowsJSON += windowsJSONString
				if index != (len(ocrResults) - 1) {
					detectedWindowsJSON += ","
				}
			}
			detectedWindowsJSON += "]"
			log.Println("Detected windows json string data:", detectedWindowsJSON)

			ocrResultsJSON := OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = OCRtoJSONString(ocrDataMerged)
			}

			var textChanges Delta
			var textChangesSummary string
			var textChangesJSON string

			if iteration > 1 {
				textChanges, err = getOCRDelta(previousOCRText, ocrResultsJSON)
				textChangesJSON, err = getOCRDeltaJSONString(textChanges)
				if err != nil {
					log.Println("Filed to get OCR Delta[iteration: %s]: %s", iteration, err)
				}
				textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)
				if err != nil {
					log.Println("Filed to get OCR Delta abstract description [iteration: %s]: %s", iteration, err)
				}
			}
			previousOCRText = ocrResultsJSON

			boundingBoxesJSON := boundingBoxArrayToJSONString(findBoundingBoxes(originalScreenshot))

			var taskCompleted bool = false
			var nextPrompt string
			var completionStatus string

			log.Printf("iteration %d, original goal is: %s\n", iteration, goal)
			log.Printf("iteration %d, current task is: %s\n", iteration, task.Description)

			promptLogBytes, err = json.Marshal(promptLog)
			if err != nil {
				log.Println("failed to marshal promptLog to JSON String:", err)
			}
			promptLogJSONString = string(promptLogBytes)

			actions, actionsJSONString, err := sendMessageToLLM(task.Description, boundingBoxesJSON, ocrResultsJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, detectedWindowsJSON, colorsDistribution)

			if err != nil {
				log.Println("failed to send message to LLM:", err)
			} else {
				log.Println("successfully sent a message to LLM. Iteration:", iteration)
			}

			for i := range actions {
				fmt.Printf("Action ID: %d, Action: %s", actions[i].ActionSequenceID, actions[i].Action)

				if actions[i].Coordinates != nil {
					fmt.Printf(", Coordinates: X=%d, Y=%d", actions[i].Coordinates.X, actions[i].Coordinates.Y)
				}

				setExecuteFunction(&actions[i])
				time.Sleep(100 * time.Millisecond)

				if actions[i].Action == "stopIteration" {
					break SubTaskLoop
				}
				if actions[i].Action == "stateUpdate" {
					time.Sleep(1 * time.Second)
					break
				}
				if actions[i].Action == "repeat" {
					actions[i].Execute(&actions[i], &actions)
					fmt.Println()
					continue
				}
				actions[i].Execute(&actions[i])
				fmt.Println()
			}

			screenshot, err = captureX11Screenshot()
			originalScreenshot = screenshot
			if err != nil {
				http.Error(w, "Failed to capture screenshot with BB: "+err.Error(), http.StatusInternalServerError)
				return
			}
			grayscaleScreenshot = ConvertToGrayscale(screenshot)
			ocrResults = OCR(grayscaleScreenshot)

			detectedWindowsJSON = "["
			for index, ocrElement := range ocrResults {
				ocrElementBB := image.Rect(ocrElement.BoundingBox.XMin, ocrElement.BoundingBox.YMin, ocrElement.BoundingBox.XMax, ocrElement.BoundingBox.YMax)
				windowsJSONString, err := vision.DetectWindow(grayscaleScreenshot, ocrElementBB, ocrElement.Text)
				if err != nil {
					log.Println("Failed to detect windows:", err)
					continue
				}
				detectedWindowsJSON += windowsJSONString
				if index != (len(ocrResults) - 1) {
					detectedWindowsJSON += ","
				}
			}
			detectedWindowsJSON += "]"
			log.Println("Detected windows json string data:", detectedWindowsJSON)

			ocrResultsJSON = OCRtoJSONString(ocrResults)
			if len(ocrResultsJSON) > 10000 {
				ocrDataMerged := MergeCloseText(ocrResults, 20, 40)
				ocrResultsJSON = OCRtoJSONString(ocrDataMerged)
			}

			textChanges, err = getOCRDelta(previousOCRText, ocrResultsJSON)
			textChangesJSON, err = getOCRDeltaJSONString(textChanges)

			if err != nil {
				log.Println("Filed to get OCR Delta[iteration: %s]: %s", iteration, err)
			}

			textChangesSummary, err = getOCRDeltaAbstractDescription(textChangesJSON)

			if err != nil {
				log.Println("Filed to get OCR Delta abstract description [iteration: %s]: %s", iteration, err)
			}

			previousOCRText = ocrResultsJSON

			boundingBoxesJSON = boundingBoxArrayToJSONString(findBoundingBoxes(originalScreenshot))

			currentCursorPosition, _ := getCursorPositionJSON()

			_, CursorY := getCursorPosition()
			log.Println("image under the cursor bounding box[x,y,x2,y2]:", 0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			rect := image.Rect(0, max(0, CursorY-23), grayscaleScreenshot.Bounds().Max.X, min(CursorY+23, grayscaleScreenshot.Bounds().Max.Y))
			imgUnderCursor := image.NewGray(rect)
			draw.Draw(imgUnderCursor, imgUnderCursor.Bounds(), grayscaleScreenshot, rect.Min, draw.Src)
			ocrDataNearTheCursor := OCRtoJSONString(OCR(imgUnderCursor))
			log.Println("OCR data near the cursor[46 pix height]:", ocrDataNearTheCursor)

			colorsDistributionBeforeActions := colorsDistribution
			colorsDistribution = dominantColorsToJSONString(dominantColors(screenshot, 10))
			log.Println("colorsDistribution after actions: ", colorsDistribution)

			taskCompleted, completionStatus, nextPrompt = isGoalAchieved(task.Description, boundingBoxesJSON, ocrResultsJSON, textChangesJSON, textChangesSummary, promptLogJSONString, iteration, prevCursorPositionJSONString, detectedWindowsJSON, currentCursorPosition, ocrDataNearTheCursor, colorsDistributionBeforeActions, colorsDistribution)
			log.Println("Verdict description:", completionStatus)
			if taskCompleted {
				log.Println("Completed task: ", task.Description)
				log.Println("TASK COMPLETED! breaking SubTaskLoop...")
				promptLog = nil
				break SubTaskLoop
			} else {
				log.Println("task not achieved, new prompt is:", nextPrompt)
				prompt = nextPrompt
				promptLog = append(promptLog, PromptLog{iteration, nextPrompt})
			}

			iteration += 1
			prevActionsJSONString = actionsJSONString
			prevCursorPositionJSONString, _ = getCursorPositionJSON()
			time.Sleep(1 * time.Second)
		}
	}
	log.Println("GOAL ACHIEVED! breaking AGIloop...")

	var response []map[string]interface{}
	response = append(response, map[string]interface{}{
		"result": "ok",
	})

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

// Delta represents the changes between old and new data
type Delta struct {
	Added    []TesseractBoundingBox `json:"added"`
	Removed  []TesseractBoundingBox `json:"removed"`
	Modified []TesseractBoundingBox `json:"modified"`
}

// Function to produce the delta between old and new OCR data
func ProduceOCRDelta(oldData, newData []TesseractBoundingBox) Delta {
	delta := Delta{
		Added:    make([]TesseractBoundingBox, 0),
		Removed:  make([]TesseractBoundingBox, 0),
		Modified: make([]TesseractBoundingBox, 0),
	}

	oldMap := make(map[string]Box)
	for _, obj := range oldData {
		key := obj.Text
		oldMap[key] = obj.BoundingBox
	}

	newMap := make(map[string]Box)
	for _, obj := range newData {
		key := obj.Text
		newMap[key] = obj.BoundingBox
	}

	// Find removed objects
	for key, bb := range oldMap {
		if _, exists := newMap[key]; !exists {
			// delta.Removed = append(delta.Removed, TesseractBoundingBox{Text: key, BoundingBox: bb})
			delta.Removed = append(delta.Removed, TesseractBoundingBox{Text: key, BoundingBox: bb, Confidence: 99})
		} else {
			// Check for modifications
			newBB := newMap[key]
			if isBoundingBoxChanged(bb, newBB) {
				// delta.Modified = append(delta.Modified, TesseractBoundingBox{Text: key, BoundingBox: newBB})
				delta.Modified = append(delta.Modified, TesseractBoundingBox{Text: key, BoundingBox: newBB, Confidence: 99})
			}
			delete(newMap, key) // Remove from newMap to avoid adding it as added
		}
	}

	// Find added objects
	for key, bb := range newMap {
		// delta.Added = append(delta.Added, TesseractBoundingBox{Text: key, BoundingBox: bb})
		delta.Added = append(delta.Added, TesseractBoundingBox{Text: key, BoundingBox: bb, Confidence: 99})
	}

	return delta
}

// Helper function to determine if bounding box has changed significantly
func isBoundingBoxChanged(oldBB, newBB Box) bool {
	// Calculate the absolute difference for each coordinate
	dxMin := absOCR(oldBB.XMin - newBB.XMin)
	dyMin := absOCR(oldBB.YMin - newBB.YMin)
	dxMax := absOCR(oldBB.XMax - newBB.XMax)
	dyMax := absOCR(oldBB.YMax - newBB.YMax)

	// If any difference exceeds 5 pixels, consider it a change
	return dxMin > 5 || dyMin > 5 || dxMax > 5 || dyMax > 5
}

// Helper function to calculate absolute value
func absOCR(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Example usage
func getOCRDelta(oldJSONstring string, newJSONstring string) (ocrDelta Delta, err error) {
	var oldData, newData []TesseractBoundingBox
	err = json.Unmarshal([]byte(oldJSONstring), &oldData)
	if err != nil {
		fmt.Println("Error unmarshaling old JSON:", err)
		return Delta{}, err
	}

	err = json.Unmarshal([]byte(newJSONstring), &newData)
	if err != nil {
		fmt.Println("Error unmarshaling new JSON:", err)
		return Delta{}, err
	}

	delta := ProduceOCRDelta(oldData, newData)
	return delta, nil
}

func getOCRDeltaJSONString(data Delta) (ocrDelta string, err error) {
	deltaJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling delta to JSON:", err)
		return "", err
	}
	deltaJSONString := string(deltaJSON)
	fmt.Println("raw ocr Delta len:", len(deltaJSONString))
	fmt.Println("raw ocr Delta:")
	fmt.Println(deltaJSONString)
	return deltaJSONString, nil
}

// MergeCloseText merges text elements that are close horizontally and vertically
func MergeCloseText(ocrData []TesseractBoundingBox, horizontalProximity, verticalProximity int) []TesseractBoundingBox {
	// Helper functions
	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	abs := func(a int) int {
		if a < 0 {
			return -a
		}
		return a
	}

	// mergeBoundingBoxes merges two bounding boxes into one
	mergeBoundingBoxes := func(bb1, bb2 Box) Box {
		return Box{
			XMin: min(bb1.XMin, bb2.XMin),
			YMin: min(bb1.YMin, bb2.YMin),
			XMax: max(bb1.XMax, bb2.XMax),
			YMax: max(bb1.YMax, bb2.YMax),
		}
	}

	// areCloseHorizontally checks if two bounding boxes are close horizontally
	areCloseHorizontally := func(bb1, bb2 Box, proximity int) bool {
		// Check if the bounding boxes overlap or are within the proximity
		return abs(bb1.XMin-bb2.XMin) <= proximity || abs(bb1.XMax-bb2.XMax) <= proximity
	}

	// areCloseVertically checks if two bounding boxes are close vertically
	areCloseVertically := func(bb1, bb2 Box, proximity int) bool {
		// Check if the bounding boxes overlap or are within the proximity
		return abs(bb1.YMin-bb2.YMin) <= proximity || abs(bb1.YMax-bb2.YMax) <= proximity
	}

	// mergeGroup merges a group of TesseractBoundingBox into a single TesseractBoundingBox
	mergeGroup := func(group []TesseractBoundingBox) TesseractBoundingBox {
		if len(group) == 0 {
			return TesseractBoundingBox{}
		}

		mergedText := group[0].Text
		mergedBB := group[0].BoundingBox

		for i := 1; i < len(group); i++ {
			mergedText += " " + group[i].Text
			mergedBB = mergeBoundingBoxes(mergedBB, group[i].BoundingBox)
		}

		return TesseractBoundingBox{
			Text:        mergedText,
			Confidence:  group[0].Confidence, // Use the confidence of the first element
			BoundingBox: mergedBB,
		}
	}

	// mergeHorizontally merges text elements that are close horizontally
	mergeHorizontally := func(ocrData []TesseractBoundingBox, proximity int) []TesseractBoundingBox {
		lines := make([]TesseractBoundingBox, 0)
		used := make([]bool, len(ocrData))

		for i, data := range ocrData {
			if used[i] {
				continue
			}

			group := []TesseractBoundingBox{data}
			used[i] = true

			for j := i + 1; j < len(ocrData); j++ {
				if used[j] {
					continue
				}

				// Check if the bounding boxes are close horizontally
				if areCloseHorizontally(data.BoundingBox, ocrData[j].BoundingBox, proximity) {
					group = append(group, ocrData[j])
					used[j] = true
				}
			}

			// Merge the group into a single line
			mergedLine := mergeGroup(group)
			lines = append(lines, mergedLine)
		}

		return lines
	}

	// mergeVertically merges lines that are close vertically
	mergeVertically := func(lines []TesseractBoundingBox, proximity int) []TesseractBoundingBox {
		mergedData := make([]TesseractBoundingBox, 0)
		used := make([]bool, len(lines))

		for i, line := range lines {
			if used[i] {
				continue
			}

			group := []TesseractBoundingBox{line}
			used[i] = true

			for j := i + 1; j < len(lines); j++ {
				if used[j] {
					continue
				}

				// Check if the lines are close vertically
				if areCloseVertically(line.BoundingBox, lines[j].BoundingBox, proximity) {
					group = append(group, lines[j])
					used[j] = true
				}
			}

			// Merge the group into a single block
			mergedBlock := mergeGroup(group)
			mergedData = append(mergedData, mergedBlock)
		}

		return mergedData
	}

	// Step 1: Merge horizontally close text into lines
	lines := mergeHorizontally(ocrData, horizontalProximity)

	// Step 2: Sort lines by their vertical position (yMin)
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].BoundingBox.YMin < lines[j].BoundingBox.YMin
	})

	// Step 3: Merge vertically close lines into paragraphs or blocks
	mergedData := mergeVertically(lines, verticalProximity)

	return mergedData
}

// Float64WithPrecision is a custom type to round float64 values to 2 decimal places
type Float64WithPrecision float64

// MarshalJSON implements the json.Marshaler interface
func (f Float64WithPrecision) MarshalJSON() ([]byte, error) {
	// Round the float64 value to 2 decimal places
	rounded := math.Round(float64(f)*100) / 100
	// Return the rounded value as a JSON number
	return json.Marshal(rounded)
}

// TesseractBoundingBox represents the text and its bounding box
type TesseractBoundingBox struct {
	Text        string               `json:"text"`
	Confidence  Float64WithPrecision `json:"confidence"`
	BoundingBox Box                  `json:"bb"`
}

// Box represents the coordinates of the bounding box
type Box struct {
	XMin int `json:"xMin"`
	YMin int `json:"yMin"`
	XMax int `json:"xMax"`
	YMax int `json:"yMax"`
}

// GetTesseractBoundingBoxes extracts bounding boxes from an image using gosseract/v2
func GetTesseractBoundingBoxes(img image.Image) ([]TesseractBoundingBox, error) {
	tmpFile, err := os.CreateTemp("/dev/shm", "ocr_image_*.png")
	if err != nil {
		// Fallback to regular temp directory if /dev/shm is not available
		tmpFile, err = os.CreateTemp("", "ocr_image_*.png")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary file: %w", err)
		}
	}
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file
	defer tmpFile.Close()

	// Encode the image to PNG and save it to the temporary file
	err = png.Encode(tmpFile, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode image to PNG: %w", err)
	}

	// Initialize the gosseract client
	client := gosseract.NewClient()
	defer client.Close()

	// Set the image from the temporary file
	err = client.SetImage(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to set image: %w", err)
	}

	// Get bounding boxes for recognized text
	boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil {
		return nil, fmt.Errorf("failed to get bounding boxes: %w", err)
	}

	// Convert gosseract bounding boxes to our custom struct
	var TesseractBoundingBoxes []TesseractBoundingBox
	for _, box := range boxes {
		TesseractBoundingBoxes = append(TesseractBoundingBoxes, TesseractBoundingBox{
			Text:       box.Word,
			Confidence: Float64WithPrecision(box.Confidence),
			BoundingBox: Box{
				XMin: box.Box.Min.X,
				YMin: box.Box.Min.Y,
				XMax: box.Box.Max.X,
				YMax: box.Box.Max.Y,
			},
		})
	}

	return TesseractBoundingBoxes, nil
}

func TesseractBoundingBoxesToJSON(boxes []TesseractBoundingBox) ([]byte, error) {
	return json.MarshalIndent(boxes, "", "  ")
}

func OCR(img image.Image) (result []TesseractBoundingBox) {
	now := time.Now()

	// Get bounding boxes
	boxes, err := GetTesseractBoundingBoxes(img)
	if err != nil {
		log.Fatalf("failed to get bounding boxes: %v", err)
	}
	fmt.Println("tesseract ocr done in:", time.Now().Sub(now))

	return boxes
}

func OCRtoJSONString(data []TesseractBoundingBox) (result string) {
	// Convert bounding boxes to JSON
	jsonOutput, err := TesseractBoundingBoxesToJSON(data)
	if err != nil {
		log.Fatalf("failed to convert bounding boxes to JSON: %v", err)
	}
	return string(jsonOutput)
}

func findBoundingBoxes(img image.Image) []BoundingBox {
	var bbArray []BoundingBox
	bbCounter := 1

	// Convert to grayscale and create RGBA image
	grayImg := ConvertToGrayscale(img)
	rgbaImg := image.NewRGBA(img.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), grayImg, image.Point{0, 0}, draw.Src)

	// Process original grayscale image
	dominantColors := findDominantColors(rgbaImg)[:40]
	// Convert []color.RGBA to []color.Color
	colors := make([]color.Color, len(dominantColors))
	for i, c := range dominantColors {
		colors[i] = c
	}
	var newBoxes []BoundingBox
	newBoxes, bbCounter = processDominantColors(rgbaImg, colors, bbCounter, 6, 9)
	bbArray = append(bbArray, newBoxes...)

	// Process binarized image
	binaryImg := BinarizeImage(grayImg, 98)
	rgbaImg = image.NewRGBA(binaryImg.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), binaryImg, image.Point{0, 0}, draw.Src)

	dominantColors = findDominantColors(rgbaImg)
	// For the second instance:
	colors = make([]color.Color, len(dominantColors))
	for i, c := range dominantColors {
		colors[i] = c
	}
	newBoxes, bbCounter = processDominantColors(rgbaImg, colors, bbCounter, 15, 15)
	bbArray = append(bbArray, newBoxes...)

	return bbArray
}

func boundingBoxArrayToJSONString(bbArray []BoundingBox) string {
	jsonBytes, err := json.MarshalIndent(bbArray, "", "    ")
	if err != nil {
		log.Printf("Error marshaling bounding box array to JSON: %v", err)
		return "[]" // Return empty array string on error
	}

	log.Println("findBoundingBoxes function, bounding boxes:")
	log.Println(string(jsonBytes))
	log.Println("AT THE TOP IS LLM-CONTEXT-BB BOUNDING BOXES.")

	return string(jsonBytes)
}

func processDominantColors(rgbaImg *image.RGBA, dominantColors []color.Color, bbCounter int, minAbsY, minAbsX int) ([]BoundingBox, int) {
	var bbArray []BoundingBox
	for _, colorElem := range dominantColors {
		// Create masks and find components
		mask := createMask(rgbaImg, colorElem.(color.RGBA), 0)
		mask2 := createMask(rgbaImg, colorElem.(color.RGBA), 90)

		components := findConnectedComponents(mask)
		components2 := findConnectedComponents(mask2)
		components = append(components, components2...)

		// Process each component
		for _, component := range components {
			x1, y1, x2, y2 := findBoundingBox(component)
			absy := abs(y1, y2)
			absx := abs(x1, x2)

			if absy < minAbsY || absx < minAbsX {
				continue
			}

			bbArray = append(bbArray, BoundingBox{bbCounter, x1, y1, x2, y2})
			bbCounter++
		}
	}
	return bbArray, bbCounter
}

func main() {
	flag.Parse()

	if err := suppressXGBLogs(); err != nil {
		log.Fatalf("Failed to suppress xgb logs: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/screenshot", screenshotHandler)
	mux.HandleFunc("/mouse-input", mouseInputHandler)
	mux.HandleFunc("/mouse-click", mouseClickHandler)
	mux.HandleFunc("/llm-input", llmInputHandler)
	mux.HandleFunc("/video2", video2Handler)

	bindAddr := net.JoinHostPort((*bindIP), strconv.Itoa(*bindPORT))
	log.Println("Server running on http://" + bindAddr)

	if err := http.ListenAndServe(bindAddr, corsMiddleware(mux)); err != nil {
		log.Fatal("Server error:", err)
	}
}
