package imagepkg

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"sort"
)

type BoundingBox struct {
	ID     int `json:"id"`
	X      int `json:"x"`
	Y      int `json:"y"`
	X2     int `json:"x2"`
	Y2     int `json:"y2"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ColorCount struct {
	Color color.Color `json:"color"`
	Count int         `json:"count"`
	R     uint8       `json:"r"`
	G     uint8       `json:"g"`
	B     uint8       `json:"b"`
}

type Component struct {
	Pixels []image.Point
}

func DominantColors(img image.Image, maxColors int) []ColorCount {
	colorMap := make(map[color.Color]int)
	bounds := img.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			colorMap[c]++
		}
	}

	var colors []ColorCount
	for c, count := range colorMap {
		r, g, b, _ := c.RGBA()
		colors = append(colors, ColorCount{
			Color: c,
			Count: count,
			R:     uint8(r >> 8),
			G:     uint8(g >> 8),
			B:     uint8(b >> 8),
		})
	}

	sort.Slice(colors, func(i, j int) bool {
		return colors[i].Count > colors[j].Count
	})

	if len(colors) > maxColors {
		colors = colors[:maxColors]
	}

	return colors
}

func DominantColorsToJSONString(colors []ColorCount) string {
	data, err := json.Marshal(colors)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func FindBoundingBoxes(img image.Image) []BoundingBox {
	bounds := img.Bounds()
	visited := make(map[image.Point]bool)
	var components []Component

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p := image.Point{X: x, Y: y}
			if !visited[p] {
				component := findComponent(img, p, visited)
				if len(component.Pixels) > 10 {
					components = append(components, component)
				}
			}
		}
	}

	var boundingBoxes []BoundingBox
	for i, component := range components {
		if len(component.Pixels) > 50 {
			box := componentToBoundingBox(component, i+1)
			boundingBoxes = append(boundingBoxes, box)
		}
	}

	return boundingBoxes
}

func BoundingBoxArrayToJSONString(boxes []BoundingBox) string {
	data, err := json.Marshal(boxes)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func FindDominantColors(img image.Image) []ColorCount {
	return DominantColors(img, 50)
}

func CreateMask(img image.Image, targetColor color.Color, tolerance int) image.Image {
	bounds := img.Bounds()
	mask := image.NewGray(bounds)

	tr, tg, tb, _ := targetColor.RGBA()
	tr >>= 8
	tg >>= 8
	tb >>= 8

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r >>= 8
			g >>= 8
			b >>= 8

			distance := math.Sqrt(float64((r-tr)*(r-tr) + (g-tg)*(g-tg) + (b-tb)*(b-tb)))
			if distance <= float64(tolerance) {
				mask.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	return mask
}

func FindConnectedComponents(mask image.Image) []Component {
	bounds := mask.Bounds()
	visited := make(map[image.Point]bool)
	var components []Component

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p := image.Point{X: x, Y: y}
			if !visited[p] && mask.At(x, y) == color.White {
				component := findComponent(mask, p, visited)
				if len(component.Pixels) > 5 {
					components = append(components, component)
				}
			}
		}
	}

	return components
}

func CalculatePercentage(component Component, mask image.Image) float64 {
	totalPixels := 0
	maskPixels := 0
	bounds := mask.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			totalPixels++
			if mask.At(x, y) == color.White {
				maskPixels++
			}
		}
	}

	if totalPixels == 0 {
		return 0
	}

	return float64(len(component.Pixels)) / float64(maskPixels) * 100
}

func FindBoundingBox(component Component) (int, int, int, int) {
	if len(component.Pixels) == 0 {
		return 0, 0, 0, 0
	}

	minX, minY := component.Pixels[0].X, component.Pixels[0].Y
	maxX, maxY := component.Pixels[0].X, component.Pixels[0].Y

	for _, p := range component.Pixels {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	return minX, minY, maxX, maxY
}

func DrawBoundingBox(img image.Image, x1, y1, x2, y2 int, borderColor color.Color) {
	if rgba, ok := img.(*image.RGBA); ok {
		drawBorder(rgba, x1, y1, x2, y2, borderColor)
	}
}

func findComponent(img image.Image, start image.Point, visited map[image.Point]bool) Component {
	var component Component
	var stack []image.Point
	stack = append(stack, start)

	bounds := img.Bounds()

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[p] || !p.In(bounds) {
			continue
		}

		visited[p] = true
		component.Pixels = append(component.Pixels, p)

		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				if dx == 0 && dy == 0 {
					continue
				}
				np := image.Point{X: p.X + dx, Y: p.Y + dy}
				if !visited[np] && np.In(bounds) {
					if img.At(np.X, np.Y) == img.At(p.X, p.Y) {
						stack = append(stack, np)
					}
				}
			}
		}
	}

	return component
}

func componentToBoundingBox(component Component, id int) BoundingBox {
	if len(component.Pixels) == 0 {
		return BoundingBox{ID: id}
	}

	minX, minY := component.Pixels[0].X, component.Pixels[0].Y
	maxX, maxY := component.Pixels[0].X, component.Pixels[0].Y

	for _, p := range component.Pixels {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	return BoundingBox{
		ID:     id,
		X:      minX,
		Y:      minY,
		X2:     maxX,
		Y2:     maxY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}
}

func drawBorder(img *image.RGBA, x1, y1, x2, y2 int, borderColor color.Color) {
	for x := x1; x <= x2; x++ {
		img.Set(x, y1, borderColor)
		img.Set(x, y2, borderColor)
	}
	for y := y1; y <= y2; y++ {
		img.Set(x1, y, borderColor)
		img.Set(x2, y, borderColor)
	}
}
