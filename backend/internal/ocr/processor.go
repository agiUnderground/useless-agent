package ocr

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"sort"

	"github.com/otiai10/gosseract/v2"
)

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

// TesseractBoundingBoxesToJSON converts bounding boxes to JSON
func TesseractBoundingBoxesToJSON(boxes []TesseractBoundingBox) ([]byte, error) {
	return json.MarshalIndent(boxes, "", "  ")
}

// OCR performs OCR on an image
func OCR(img image.Image) (result []TesseractBoundingBox) {
	// Get bounding boxes
	boxes, err := GetTesseractBoundingBoxes(img)
	if err != nil {
		log.Printf("failed to get bounding boxes: %v", err)
		return []TesseractBoundingBox{} // Return empty result instead of fatal
	}
	return boxes
}

// OCRtoJSONString converts OCR data to JSON string
func OCRtoJSONString(data []TesseractBoundingBox) (result string) {
	// Convert bounding boxes to JSON
	jsonOutput, err := TesseractBoundingBoxesToJSON(data)
	if err != nil {
		log.Printf("failed to convert bounding boxes to JSON: %v", err)
		return "[]" // Return empty JSON array instead of fatal
	}
	return string(jsonOutput)
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

// ProduceOCRDelta produces the delta between old and new OCR data
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
			delta.Removed = append(delta.Removed, TesseractBoundingBox{Text: key, BoundingBox: bb, Confidence: 99})
		} else {
			// Check for modifications
			newBB := newMap[key]
			if isBoundingBoxChanged(bb, newBB) {
				delta.Modified = append(delta.Modified, TesseractBoundingBox{Text: key, BoundingBox: newBB, Confidence: 99})
			}
			delete(newMap, key) // Remove from newMap to avoid adding it as added
		}
	}

	// Find added objects
	for key, bb := range newMap {
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

// GetOCRDelta gets OCR delta between old and new JSON strings
func GetOCRDelta(oldJSONstring string, newJSONstring string) (ocrDelta Delta, err error) {
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

// GetOCRDeltaJSONString converts OCR delta to JSON string
func GetOCRDeltaJSONString(data Delta) (ocrDelta string, err error) {
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
