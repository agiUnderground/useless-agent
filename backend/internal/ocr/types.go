package ocr

import (
	"encoding/json"
	"math"
)

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

// Delta represents the changes between old and new data
type Delta struct {
	Added    []TesseractBoundingBox `json:"added"`
	Removed  []TesseractBoundingBox `json:"removed"`
	Modified []TesseractBoundingBox `json:"modified"`
}
