package vision 

import (
    "os"
    "fmt"
    "image"
    "image/png"
    "image/color"
    "encoding/json"
)

const debug = false

type WindowInfo struct {
    Window Window `json:"window"`
}

type Window struct {
    Title   string          `json:"title"`
    BBox    image.Rectangle `json:"bbox"`
    Header  image.Rectangle `json:"header"`
    Buttons []Button        `json:"buttons"`
}

type Button struct {
    Name string             `json:"name"`
    BBox image.Rectangle `json:"bbox"`
}

func DetectWindow(img image.Image, textBBox image.Rectangle, title string) (string, error) {
    logDebug("DetectWindow running detection for: %v  <---------------------------------------------------------", title)
    logDebug("=== Starting Detection Process ===")
    logDebug("Image bounds: %v", img.Bounds())
    logDebug("Initial text bbox: %v", textBBox)

    bgColor := detectBackgroundColor(img, textBBox)
    logDebug("Detected background color: %+v", rgbaString(bgColor))

    header, borderRight, err := detectHeader(img, textBBox, bgColor)
    if err != nil {
        return "", fmt.Errorf("header detection failed: %w", err)
    }
    logDebug("Final header: %v", header)
    logDebug("Border right position: %d", borderRight)

    buttons := detectButtons(img, header, bgColor, borderRight)
    if len(buttons) != 4 {
        return "", fmt.Errorf("button detection failed: found %d buttons", len(buttons))
    }
    logDebug("Detected buttons:")
    for _, btn := range buttons {
        logDebug(" - %s: %v", btn.Name, btn.BBox)
    }

    // Detect window height using border color
    windowBottom := detectWindowHeight(img, header, borderRight-1, bgColor)

    result := WindowInfo{
        Window: Window{
            Title: title,
            BBox:    image.Rect(header.Min.X, header.Min.Y, borderRight, windowBottom),
            Header:  header,
            Buttons: buttons,
        },
    }

    jsonData, err := json.MarshalIndent(result, "", "  ")
    if err != nil {
        return "", fmt.Errorf("JSON encoding failed: %w", err)
    }

    return string(jsonData), nil
}

func detectHeader(img image.Image, textBBox image.Rectangle, bgColor color.Color) (image.Rectangle, int, error) {
    logDebug("\n=== Header Detection ===")
    
    // Phase 1: Check right side pattern
    logDebug("Checking right side pattern...")
    rightHeader, borderRight, found := checkRightPattern(img, textBBox, bgColor)
    if !found {
        return image.Rectangle{}, 0, fmt.Errorf("right pattern not found")
    }
    logDebug("Right pattern verified - header right: %d, border: %d", rightHeader.Max.X, borderRight)

    // Phase 2: Expand to the left
    logDebug("Expanding to the left...")
    leftEdge := findLeftEdge(img, textBBox, bgColor)

    titleMidX := (textBBox.Max.X - textBBox.Min.X) / 2
    headerTop := findHeaderTop(img, titleMidX, textBBox.Min.Y, bgColor)
    finalHeader := image.Rect(leftEdge, headerTop, rightHeader.Max.X, textBBox.Max.Y + (textBBox.Min.Y - headerTop))
    
    logDebug("Final header boundaries - left: %d, right: %d", leftEdge, rightHeader.Max.X)
    return finalHeader, borderRight, nil
}

func checkRightPattern(img image.Image, textBBox image.Rectangle, bgColor color.Color) (image.Rectangle, int, bool) {
    logDebug("Tracing right edge...")
    headerRight, borderRight := traceRightEdge(img, textBBox, bgColor)
    tempHeader := image.Rect(textBBox.Min.X, textBBox.Min.Y, headerRight, textBBox.Max.Y)
    
    logDebug("Temporary header: %v", tempHeader)
    logDebug("Checking button pattern...")
    if valid := verifyButtonPattern(img, tempHeader, bgColor, borderRight); !valid {
        logDebug("Button pattern verification failed")
        return image.Rectangle{}, 0, false
    }
    
    return tempHeader, borderRight, true
}

func traceRightEdge(img image.Image, textBBox image.Rectangle, bgColor color.Color) (int, int) {
    logDebug("Starting right edge tracing from %d", textBBox.Max.X)
    headerRight := textBBox.Max.X
    borderRight := headerRight

    for x := textBBox.Max.X; x < img.Bounds().Max.X; x++ {
        // if !isBackgroundColumn(img, x, textBBox, bgColor) {

        //if !isBackgroundPixel(img, x, textBBox, bgColor) { --------------------------
        if isNotBackgroundColumn(img, x, textBBox, bgColor) {
            borderRight = x
            logDebug("Found border at x=%d", x)
            break
        }
        headerRight = x
        logDebug("Background column at x=%d", x)
    }
    logDebug("Final header right: %d, border: %d", headerRight, borderRight)
    return headerRight, borderRight
}

func verifyButtonPattern(img image.Image, header image.Rectangle, bgColor color.Color, borderRight int) bool {
    logDebug("Scanning for button pattern from %d", borderRight)
    currentX := borderRight - 1
    foundButtons := 0
    prevButtonLeftEdge := 0

    for i := 0; i < 4; i++ {
        logDebug("Checking for button %d at x=%d", i+1, currentX)
        rightEdge, btnWidth := findVerticalEdge(img, currentX, header, bgColor, true)
        if rightEdge == -1 {
            logDebug("No right edge found")
            break
        }
        // horizontal distance check
        if i > 0 {
            xDelta := prevButtonLeftEdge - rightEdge
            if (xDelta < 12) || (xDelta > 40) {
              logDebug("Potential buttons distance is not correct.")
              break
            }
        }

        // btnWidth := rightEdge - leftEdge
        logDebug("Potential button %d: left=%d, right=%d (width=%d)", 
            i+1, rightEdge - btnWidth, rightEdge, btnWidth)

        if btnWidth < 4 || btnWidth > 30 {
            logDebug("Invalid button width")
            break
        }

        foundButtons++
        // currentX = leftEdge - 5
        prevButtonLeftEdge = rightEdge - btnWidth
        currentX = rightEdge - btnWidth
    }
    // 15 40 x distance between buttons >=15 <=40
    logDebug("Found %d valid buttons", foundButtons)
    return foundButtons == 4
}

func findLeftEdge(img image.Image, textBBox image.Rectangle, bgColor color.Color) int {
    logDebug("Searching left edge from %d", textBBox.Min.X)
    leftEdge := textBBox.Min.X

    for x := textBBox.Min.X - 1; x >= img.Bounds().Min.X; x-- {
        if !isBackgroundColumn(img, x, textBBox, bgColor) {
            logDebug("Found left boundary at x=%d", x+1)
            for x2 := x; x2 >= img.Bounds().Min.X; x2-- {
              if isBackgroundColumn(img, x2, textBBox, bgColor) {
                leftEdge = x2
                logDebug("Extending left to x=%d", x2)
              }
            }
            break
        }
    }
    return leftEdge
}

func detectButtons(img image.Image, header image.Rectangle, bgColor color.Color, borderRight int) []Button {
    logDebug("\n=== Button Detection ===")
    var buttons []Button
    currentX := borderRight - 1
    buttonNames := map[int]string{
        1: "roll-up",
        2: "minimize",
        3: "maximize",
        4: "close",
    }

    for i := 0; i < 4; i++ {
        logDebug("Searching for button %d starting at x=%d", 4-i, currentX)
        rightEdge, btnWidth := findVerticalEdge(img, currentX, header, bgColor, true)
        if rightEdge == -1 {
            logDebug("Right edge not found")
            break
        }

        leftEdge := rightEdge - btnWidth

        btn := Button{
            //Name: fmt.Sprintf("button%d", 4-i),
            Name: buttonNames[4-i],
            // BBox: image.Rect(leftEdge, header.Min.Y, rightEdge, header.Max.Y),
            BBox: image.Rect(leftEdge, header.Min.Y + 5, rightEdge, header.Max.Y - 5),
        }
        logDebug("Detected %s: %v", btn.Name, btn.BBox)
        
        buttons = append([]Button{btn}, buttons...)
        currentX = leftEdge - 5
    }

    logDebug("Total detected buttons: %d", len(buttons))
    return buttons
}

func findVerticalEdge(img image.Image, startX int, header image.Rectangle, bgColor color.Color, searchLeft bool) (int, int) {
    step := 1
    direction := "right"
    if searchLeft {
        step = -1
        direction = "left"
    }

    logDebug("Scanning %s from x=%d (header Y: %d-%d)", direction, startX, header.Min.Y, header.Max.Y)
    rightEdge := -1
    leftEdge := -1
    
    foundRightEdge:
    for x := startX; x >= header.Min.X && x <= header.Max.X; x += step {
        for y := header.Min.Y; y < header.Max.Y; y++ {
            if !colorsSimilar(img.At(x, y), bgColor) {
                logDebug("Found right edge at x=%d, y=%d", x, y)
                rightEdge = x
                break foundRightEdge
            }
        }
    }

    if rightEdge == -1 {
      logDebug("Right edge not found")
      return -1, -1
    }

    foundLeftEdge:
    // !!!!!!!!!!!!!!!!!!!!!
    for x := rightEdge; x >= header.Min.X && x <= header.Max.X; x += step {
        for y := header.Min.Y; y < header.Max.Y; y++ {
            if !colorsSimilar(img.At(x, y), bgColor) {
                continue foundLeftEdge
            }
        }
        // logDebug("Found edge at x=%d, y=%d", x, y)
        logDebug("Found left edge at x=%d", x)
        leftEdge = x
        break foundLeftEdge
    }

    if leftEdge == -1 {
      logDebug("Left edge not found")
      return -1, -1
    }

    logDebug("Edges found")
    return rightEdge, rightEdge - leftEdge
}

func findHeaderTop(img image.Image, titleMidX int, titleMinY int, bgColor color.Color) int {
    for y := titleMinY; y > 0; y-- {
      if !colorsSimilar(img.At(titleMidX, y), bgColor) {
          logDebug("[findHeaderTop] Non-background pixel at (%d,%d)", titleMidX, y)
          return y 
      }
    }
    logDebug("[findHeaderTop] header top not found, returning value of the title.")
    return titleMinY
}

func isBackgroundPixel(img image.Image, x int, textBBox image.Rectangle, bgColor color.Color) bool {
    y := textBBox.Min.Y;
    if !colorsSimilar(img.At(x, y), bgColor) {
        logDebug("Non-background pixel at (%d,%d)", x, y)
        return false
    }
    return true
}

func isBackgroundColumn(img image.Image, x int, textBBox image.Rectangle, bgColor color.Color) bool {
    for y := textBBox.Min.Y; y < textBBox.Max.Y; y++ {
      // y := textBBox.Min.Y;
      if !colorsSimilar(img.At(x, y), bgColor) {
          logDebug("Non-background pixel at (%d,%d)", x, y)
          return false
      }
    }
    return true
}

func isNotBackgroundColumn(img image.Image, x int, textBBox image.Rectangle, bgColor color.Color) bool {
    for y := textBBox.Min.Y; y < textBBox.Max.Y; y++ {
      // y := textBBox.Min.Y;
      if colorsSimilar(img.At(x, y), bgColor) {
          logDebug("background-like pixel at (%d,%d)", x, y)
          return false
      }
    }
    return true
}

func detectBackgroundColor(img image.Image, textBBox image.Rectangle) color.Color {
    logDebug("\n=== Background Color Detection ===")
    samplePoints := []image.Point{
        {textBBox.Min.X - 1, textBBox.Min.Y},
        {textBBox.Max.X + 1, textBBox.Min.Y},
        {textBBox.Min.X, textBBox.Min.Y - 1},
        {textBBox.Min.X, textBBox.Max.Y + 1},
    }

    colorCount := make(map[string]int)
    for _, p := range samplePoints {
        if p.In(img.Bounds()) {
            c := rgbaString(img.At(p.X, p.Y))
            colorCount[c]++
            logDebug("Sampled color at (%d,%d): %s", p.X, p.Y, c)
        }
    }

    maxCount := 0
    dominantColor := ""
    for color, count := range colorCount {
        if count > maxCount {
            maxCount = count
            dominantColor = color
        }
    }

    logDebug("Dominant background color: %s (count: %d)", dominantColor, maxCount)
    return parseColor(dominantColor)
}

func loadImage(path string) (image.Image, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("error opening file: %w", err)
    }
    defer file.Close()

    img, err := png.Decode(file)
    if err != nil {
        return nil, fmt.Errorf("error decoding PNG: %w", err)
    }
    return img, nil
}

func colorsSimilar(c1, c2 color.Color) bool {
    r1, g1, b1, _ := c1.RGBA()
    r2, g2, b2, _ := c2.RGBA()
    return absDiff(r1, r2)+absDiff(g1, g2)+absDiff(b1, b2) < 0x2000
}

func absDiff(a, b uint32) uint32 {
    if a > b {
        return a - b
    }
    return b - a
}

func rgbaString(c color.Color) string {
    r, g, b, a := c.RGBA()
    return fmt.Sprintf("RGBA(%d,%d,%d,%d)", r>>8, g>>8, b>>8, a>>8)
}

func parseColor(s string) color.Color {
    var r, g, b, a uint8
    fmt.Sscanf(s, "RGBA(%d,%d,%d,%d)", &r, &g, &b, &a)
    return color.RGBA{r, g, b, a}
}

func logDebug(format string, args ...interface{}) {
    if debug {
        fmt.Printf("[DEBUG] "+format+"\n", args...)
    }
}

func logError(format string, args ...interface{}) {
    fmt.Printf("[ERROR] "+format+"\n", args...)
}

const (
    maxHeightSearch    = 2000 // Prevent infinite search
    borderSamplingSize = 1    // Number of pixels to sample for border color
)

func detectWindowHeight(img image.Image, header image.Rectangle, borderRight int, bgColor color.Color) int {
    logDebug("\n=== Window Height Detection ===")
    
    // 1. Detect border color from header area
    borderColor := detectBorderColor(img, header, borderRight)
    logDebug("Detected border color: %s", rgbaString(borderColor))
    
    // 2. Trace border downward
    startY := header.Max.Y
    maxY := img.Bounds().Max.Y
    windowBottom := startY
    
    // Check multiple points across the border width
    // xPositions := []int{borderRight - 1, borderRight - 2, borderRight - 3}
    xPositions := []int{borderRight}
    
    for y := startY; y < maxY && y < startY+maxHeightSearch; y++ {
        validBorder := true
        // Check multiple x positions to verify continuous border
        for _, x := range xPositions {
            if x < 0 || x >= img.Bounds().Max.X {
                continue
            }
            if !colorsSimilar(img.At(x, y), borderColor) {
                validBorder = false
                break
            }
        }
        
        if !validBorder {
            logDebug("Border ends at y=%d", y)
            return y
        }
        windowBottom = y
    }
    
    logDebug("Reached max search limit at y=%d", windowBottom)
    return windowBottom
}

func detectBorderColor(img image.Image, header image.Rectangle, borderRight int) color.Color {
    // Sample vertical line in header area
    colorCount := make(map[string]int)
    startY := header.Min.Y
    endY := header.Max.Y
    
    for y := startY; y < endY; y++ {
        if borderRight-1 >= 0 {
            c := img.At(borderRight-1, y)
            colorCount[rgbaString(c)]++
        }
        if borderRight-2 >= 0 {
            c := img.At(borderRight-2, y)
            colorCount[rgbaString(c)]++
        }
    }
    
    // Find most frequent color
    maxCount := 0
    dominantColor := ""
    for color, count := range colorCount {
        if count > maxCount {
            maxCount = count
            dominantColor = color
        }
    }
    
    return parseColor(dominantColor)
}


