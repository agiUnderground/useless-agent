package image

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sort"
)

// ColorCount represents a color and its count
type ColorCount struct {
	Color      color.RGBA `json:"color"`
	Count      int        `json:"count"`
	Percentage float64    `json:"percentage"`
}

// DominantColors extracts the most dominant colors from an image
func DominantColors(img image.Image, maxColors int) []ColorCount {
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

	var colors []ColorCount
	for c, cnt := range colorCounts {
		percentage := (float64(cnt) / float64(totalPixels)) * 100
		colors = append(colors, ColorCount{Color: c, Count: cnt, Percentage: percentage})
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

// DominantColorsToJSONString converts dominant colors to JSON string
func DominantColorsToJSONString(colors []ColorCount) string {
	var response []map[string]interface{}

	for i := range colors {
		hexColor := fmt.Sprintf("#%02X%02X%02X", colors[i].Color.R, colors[i].Color.G, colors[i].Color.B)
		response = append(response, map[string]interface{}{
			"color":      hexColor,
			"percentage": fmt.Sprintf("%.1f%%", colors[i].Percentage),
		})
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		log.Println("[DominantColorsToJSONString] Failed to encode JSON")
		return ""
	}
	return string(jsonBytes)
}

// DrawBoundingBox draws a bounding box on an image
func DrawBoundingBox(img *image.RGBA, x1, y1, x2, y2 int, borderColor color.RGBA) {
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

// FindBoundingBox finds the bounding box of a component
func FindBoundingBox(component [][]int) (x1, y1, x2, y2 int) {
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

// FindBoundingBoxes finds all bounding boxes in an image
func FindBoundingBoxes(img image.Image) []BoundingBox {
	var bbArray []BoundingBox
	bbCounter := 1

	// Convert to grayscale and create RGBA image
	grayImg := ConvertToGrayscale(img)
	rgbaImg := image.NewRGBA(img.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), grayImg, image.Point{0, 0}, draw.Src)

	// Process original grayscale image
	dominantColors := FindDominantColors(rgbaImg)[:40]
	// Convert []color.RGBA to []color.Color
	colors := make([]color.Color, len(dominantColors))
	for i, c := range dominantColors {
		colors[i] = c
	}
	var newBoxes []BoundingBox
	newBoxes, bbCounter = ProcessDominantColors(rgbaImg, colors, bbCounter, 6, 9)
	bbArray = append(bbArray, newBoxes...)

	// Process binarized image
	binaryImg := BinarizeImage(grayImg, 98)
	rgbaImg = image.NewRGBA(binaryImg.Bounds())
	draw.Draw(rgbaImg, rgbaImg.Bounds(), binaryImg, image.Point{0, 0}, draw.Src)

	dominantColors = FindDominantColors(rgbaImg)
	// For the second instance:
	colors = make([]color.Color, len(dominantColors))
	for i, c := range dominantColors {
		colors[i] = c
	}
	newBoxes, bbCounter = ProcessDominantColors(rgbaImg, colors, bbCounter, 15, 15)
	bbArray = append(bbArray, newBoxes...)

	return bbArray
}

// BoundingBoxArrayToJSONString converts bounding box array to JSON string
func BoundingBoxArrayToJSONString(bbArray []BoundingBox) string {
	jsonBytes, err := json.MarshalIndent(bbArray, "", "    ")
	if err != nil {
		log.Printf("Error marshaling bounding box array to JSON: %v", err)
		return "[]" // Return empty array string on error
	}

	log.Println("FindBoundingBoxes function, bounding boxes:")
	log.Println(string(jsonBytes))
	log.Println("AT THE TOP IS LLM-CONTEXT-BB BOUNDING BOXES.")

	return string(jsonBytes)
}

// ProcessDominantColors processes dominant colors to find bounding boxes
func ProcessDominantColors(rgbaImg *image.RGBA, dominantColors []color.Color, bbCounter int, minAbsY, minAbsX int) ([]BoundingBox, int) {
	var bbArray []BoundingBox
	for _, colorElem := range dominantColors {
		// Create masks and find components
		mask := CreateMask(rgbaImg, colorElem.(color.RGBA), 0)
		mask2 := CreateMask(rgbaImg, colorElem.(color.RGBA), 90)

		components := FindConnectedComponents(mask)
		components2 := FindConnectedComponents(mask2)
		components = append(components, components2...)

		// Process each component
		for _, component := range components {
			x1, y1, x2, y2 := FindBoundingBox(component)
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

// Helper functions
func ConvertToGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, img.At(x, y))
		}
	}
	return gray
}

func BinarizeImage(img *image.Gray, threshold uint8) *image.Gray {
	bounds := img.Bounds()
	binary := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := img.GrayAt(x, y)
			if gray.Y < threshold {
				binary.SetGray(x, y, color.Gray{Y: 0})
			} else {
				binary.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return binary
}

func abs(x, y int) int {
	if x < y {
		return y - x
	}
	return x - y
}

// Clamp clamps a value between 0 and 255
func Clamp(value uint8) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

// CreateMask creates a binary mask for pixels within a color drift range
func CreateMask(img image.Image, dominantColor color.RGBA, drift uint8) [][]bool {
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
			if ((qr >= Clamp(dominantColor.R-drift)) && (qr <= Clamp(dominantColor.R+drift))) && ((qg >= Clamp(dominantColor.G-drift)) && (qg <= Clamp(dominantColor.G+drift))) && ((qb >= Clamp(dominantColor.B-drift)) && (qb <= Clamp(dominantColor.B+drift))) {
				mask[y][x] = true
				counter += 1
			} else {
				mask[y][x] = false
			}
		}
	}
	return mask
}

// FindDominantColors finds all dominant colors in an image
func FindDominantColors(img *image.RGBA) []color.RGBA {
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

// BoundingBox represents a bounding box
type BoundingBox struct {
	ID int `json:"id"`
	X  int `json:"x"`
	Y  int `json:"y"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// CalculatePercentage calculates the percentage of dominant pixels in a component
func CalculatePercentage(component [][]int, mask [][]bool) float64 {
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

// FindConnectedComponents finds connected components in a binary mask
func FindConnectedComponents(mask [][]bool) (components [][][]int) {
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
