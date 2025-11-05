package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/kbinani/screenshot"
)

func SuppressXGBLogs() error {
	return nil
}

func CaptureX11Screenshot() (image.Image, error) {
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("failed to capture screenshot: %w", err)
	}
	return img, nil
}

func EncodeToPNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

func ConvertToGrayscale(img image.Image) image.Image {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			originalColor := img.At(x, y)
			r, g, b, _ := originalColor.RGBA()
			grayValue := uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000)
			gray.SetGray(x, y, color.Gray{Y: grayValue})
		}
	}
	return gray
}

func GetDisplayBounds() (int, int, int, int) {
	bounds := screenshot.GetDisplayBounds(0)
	return bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y
}

func InitX11Connection() (*xgb.Conn, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11: %w", err)
	}
	return conn, nil
}

func GetX11WindowGeometry(conn *xgb.Conn, window xproto.Window) (int, int, int, int, error) {
	geom, err := xproto.GetGeometry(conn, xproto.Drawable(window)).Reply()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get window geometry: %w", err)
	}
	return int(geom.X), int(geom.Y), int(geom.Width), int(geom.Height), nil
}

func SaveToFile(img image.Image, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	err = png.Encode(file, img)
	if err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}
	return nil
}

func Crop(img image.Image, x, y, width, height int) image.Image {
	bounds := img.Bounds()
	cropRect := image.Rect(x, y, x+width, y+height)
	cropRect = cropRect.Intersect(bounds)

	cropped := image.NewRGBA(cropRect)
	draw.Draw(cropped, cropRect, img, cropRect.Min, draw.Src)
	return cropped
}

func Scale(img image.Image, scaleX, scaleY float64) image.Image {
	original := img.Bounds()
	newWidth := int(float64(original.Dx()) * scaleX)
	newHeight := int(float64(original.Dy()) * scaleY)

	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / scaleX)
			srcY := int(float64(y) / scaleY)
			if srcX < original.Dx() && srcY < original.Dy() {
				scaled.Set(x, y, img.At(original.Min.X+srcX, original.Min.Y+srcY))
			}
		}
	}
	return scaled
}

func GetScreenshotWithTimestamp() (image.Image, error) {
	img, err := CaptureX11Screenshot()
	if err != nil {
		return nil, err
	}
	log.Printf("Screenshot captured at %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	return img, nil
}
