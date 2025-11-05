package ocr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"sort"
	"strings"

	"github.com/otiai10/gosseract/v2"
)

type BoundingBox struct {
	XMin int `json:"xMin"`
	YMin int `json:"yMin"`
	XMax int `json:"xMax"`
	YMax int `json:"yMax"`
}

type OCRResult struct {
	Text        string      `json:"text"`
	BoundingBox BoundingBox `json:"boundingBox"`
	Confidence  float64     `json:"confidence"`
}

type Delta struct {
	Added   []OCRResult `json:"added"`
	Removed []OCRResult `json:"removed"`
	Changed []OCRChange `json:"changed"`
}

type OCRChange struct {
	Old OCRResult `json:"old"`
	New OCRResult `json:"new"`
}

func OCR(img image.Image) []OCRResult {
	client := gosseract.NewClient()
	defer client.Close()

	client.SetImageFromBytes(imageToBytes(img))
	text, err := client.Text()
	if err != nil {
		return []OCRResult{}
	}

	boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil {
		return []OCRResult{}
	}

	var results []OCRResult
	words := strings.Fields(text)

	for i, box := range boxes {
		if i < len(words) {
			result := OCRResult{
				Text: words[i],
				BoundingBox: BoundingBox{
					XMin: int(box.Box.Min.X),
					YMin: int(box.Box.Min.Y),
					XMax: int(box.Box.Max.X),
					YMax: int(box.Box.Max.Y),
				},
				Confidence: box.Confidence,
			}
			results = append(results, result)
		}
	}

	return results
}

func OCRtoJSONString(results []OCRResult) string {
	data, err := json.Marshal(results)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func GetOCRDelta(oldText, newText string) (Delta, error) {
	var oldResults, newResults []OCRResult
	var delta Delta

	if err := json.Unmarshal([]byte(oldText), &oldResults); err != nil {
		return delta, fmt.Errorf("failed to unmarshal old OCR results: %w", err)
	}

	if err := json.Unmarshal([]byte(newText), &newResults); err != nil {
		return delta, fmt.Errorf("failed to unmarshal new OCR results: %w", err)
	}

	delta.Added = findAdded(oldResults, newResults)
	delta.Removed = findRemoved(oldResults, newResults)
	delta.Changed = findChanged(oldResults, newResults)

	return delta, nil
}

func GetOCRDeltaJSONString(delta Delta) (string, error) {
	data, err := json.Marshal(delta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OCR delta: %w", err)
	}
	return string(data), nil
}

func MergeCloseText(results []OCRResult, xThreshold, yThreshold int) []OCRResult {
	if len(results) == 0 {
		return results
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].BoundingBox.YMin == results[j].BoundingBox.YMin {
			return results[i].BoundingBox.XMin < results[j].BoundingBox.XMin
		}
		return results[i].BoundingBox.YMin < results[j].BoundingBox.YMin
	})

	var merged []OCRResult
	current := results[0]

	for i := 1; i < len(results); i++ {
		next := results[i]

		if isClose(current.BoundingBox, next.BoundingBox, xThreshold, yThreshold) {
			current.Text += " " + next.Text
			current.BoundingBox.XMax = max(current.BoundingBox.XMax, next.BoundingBox.XMax)
			current.BoundingBox.YMax = max(current.BoundingBox.YMax, next.BoundingBox.YMax)
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)

	return merged
}

func findAdded(old, new []OCRResult) []OCRResult {
	var added []OCRResult
	for _, newResult := range new {
		found := false
		for _, oldResult := range old {
			if newResult.Text == oldResult.Text &&
				isClose(newResult.BoundingBox, oldResult.BoundingBox, 10, 10) {
				found = true
				break
			}
		}
		if !found {
			added = append(added, newResult)
		}
	}
	return added
}

func findRemoved(old, new []OCRResult) []OCRResult {
	var removed []OCRResult
	for _, oldResult := range old {
		found := false
		for _, newResult := range new {
			if oldResult.Text == newResult.Text &&
				isClose(oldResult.BoundingBox, newResult.BoundingBox, 10, 10) {
				found = true
				break
			}
		}
		if !found {
			removed = append(removed, oldResult)
		}
	}
	return removed
}

func findChanged(old, new []OCRResult) []OCRChange {
	var changes []OCRChange
	for _, oldResult := range old {
		for _, newResult := range new {
			if isClose(oldResult.BoundingBox, newResult.BoundingBox, 10, 10) &&
				oldResult.Text != newResult.Text {
				changes = append(changes, OCRChange{
					Old: oldResult,
					New: newResult,
				})
				break
			}
		}
	}
	return changes
}

func isClose(box1, box2 BoundingBox, xThreshold, yThreshold int) bool {
	return abs(box1.XMin-box2.XMin) <= xThreshold &&
		abs(box1.YMin-box2.YMin) <= yThreshold
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func imageToBytes(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
