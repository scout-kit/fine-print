package imaging

import (
	"image"

	"github.com/disintegration/imaging"
)

// TransformParams holds normalized crop/zoom parameters (all values 0-1).
type TransformParams struct {
	CropX      float64 `json:"crop_x"`
	CropY      float64 `json:"crop_y"`
	CropWidth  float64 `json:"crop_width"`
	CropHeight float64 `json:"crop_height"`
	Rotation   float64 `json:"rotation"`
}

// ApplyCrop applies crop and rotation to an image using normalized coordinates.
func ApplyCrop(img image.Image, t TransformParams) image.Image {
	bounds := img.Bounds()
	w := float64(bounds.Dx())
	h := float64(bounds.Dy())

	// Apply clockwise rotation (imaging.Rotate90 is counter-clockwise, so swap 90/270)
	switch int(t.Rotation) % 360 {
	case 90, -270:
		img = imaging.Rotate270(img) // 90° clockwise = 270° counter-clockwise
	case 180, -180:
		img = imaging.Rotate180(img)
	case 270, -90:
		img = imaging.Rotate90(img) // 270° clockwise = 90° counter-clockwise
	}
	if t.Rotation != 0 {
		bounds = img.Bounds()
		w = float64(bounds.Dx())
		h = float64(bounds.Dy())
	}

	// Convert normalized coordinates to pixel coordinates
	x0 := int(t.CropX * w)
	y0 := int(t.CropY * h)
	x1 := x0 + int(t.CropWidth*w)
	y1 := y0 + int(t.CropHeight*h)

	// Clamp to image bounds
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}

	return imaging.Crop(img, image.Rect(x0, y0, x1, y1))
}

// CenterCrop4x6 returns transform parameters for a center crop at 4x6 aspect ratio.
// Auto-detects orientation: landscape images get 3:2, portrait images get 2:3.
func CenterCrop4x6(imgWidth, imgHeight int) TransformParams {
	targetRatio := 3.0 / 2.0 // 4x6 landscape
	if imgHeight > imgWidth {
		targetRatio = 2.0 / 3.0 // 4x6 portrait
	}
	imgRatio := float64(imgWidth) / float64(imgHeight)

	var cropW, cropH float64

	if imgRatio > targetRatio {
		// Image is wider than 3:2, crop width
		cropH = 1.0
		cropW = targetRatio * float64(imgHeight) / float64(imgWidth)
	} else {
		// Image is taller than 3:2, crop height
		cropW = 1.0
		cropH = float64(imgWidth) / (targetRatio * float64(imgHeight))
	}

	return TransformParams{
		CropX:      (1.0 - cropW) / 2.0,
		CropY:      (1.0 - cropH) / 2.0,
		CropWidth:  cropW,
		CropHeight: cropH,
		Rotation:   0,
	}
}
