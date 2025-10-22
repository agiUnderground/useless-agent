package screenshot

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

// SuppressXGBLogs suppresses XGB logs
func SuppressXGBLogs() error {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	xgb.Logger.SetOutput(devNull)
	return nil
}

// CaptureX11Screenshot captures an X11 screenshot
func CaptureX11Screenshot() (image.Image, error) {
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

// ConvertToGrayscale converts an image to grayscale
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

// BinarizeImage converts a grayscale image into a binary image
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

// EncodeToPNG encodes an image to PNG bytes
func EncodeToPNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
