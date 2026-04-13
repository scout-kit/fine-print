package imaging

import (
	"image"

	"github.com/disintegration/imaging"
)

// ColorParams holds color adjustment values.
// All values range from -1 to 1, where 0 means no change.
type ColorParams struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
}

// ApplyColorAdjustments applies brightness, contrast, and saturation adjustments.
func ApplyColorAdjustments(img image.Image, c ColorParams) image.Image {
	if c.Brightness != 0 {
		// imaging.AdjustBrightness expects a percentage from -100 to 100
		img = imaging.AdjustBrightness(img, c.Brightness*100)
	}
	if c.Contrast != 0 {
		// imaging.AdjustContrast expects a percentage from -100 to 100
		img = imaging.AdjustContrast(img, c.Contrast*100)
	}
	if c.Saturation != 0 {
		// imaging.AdjustSaturation expects a percentage from -100 to 100
		img = imaging.AdjustSaturation(img, c.Saturation*100)
	}
	return img
}
