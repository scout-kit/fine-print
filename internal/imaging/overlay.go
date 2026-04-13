package imaging

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/disintegration/imaging"
)

// OverlayParams defines how an image overlay is positioned and blended.
type OverlayParams struct {
	Path    string  `json:"path"`    // Filesystem path to the overlay PNG
	X       float64 `json:"x"`       // Normalized position 0-1
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`   // Normalized size 0-1
	Height  float64 `json:"height"`
	Opacity float64 `json:"opacity"` // 0-1
}

// CompositeOverlay composites an overlay PNG onto the base image.
func CompositeOverlay(base image.Image, params OverlayParams) (image.Image, error) {
	overlay, err := imaging.Open(params.Path)
	if err != nil {
		return nil, fmt.Errorf("opening overlay: %w", err)
	}

	bounds := base.Bounds()
	baseW := float64(bounds.Dx())
	baseH := float64(bounds.Dy())

	// Calculate overlay dimensions in pixels
	overlayW := int(params.Width * baseW)
	overlayH := int(params.Height * baseH)

	if overlayW <= 0 || overlayH <= 0 {
		return base, nil
	}

	// Resize overlay to target size
	overlay = imaging.Resize(overlay, overlayW, overlayH, imaging.Lanczos)

	// Calculate position
	posX := int(params.X * baseW)
	posY := int(params.Y * baseH)

	// Apply opacity
	if params.Opacity < 1.0 {
		overlay = applyOpacity(overlay, params.Opacity)
	}

	// Composite overlay onto base
	result := imaging.Clone(base)
	draw.Draw(result, image.Rect(posX, posY, posX+overlayW, posY+overlayH),
		overlay, image.Point{}, draw.Over)

	return result, nil
}

// applyOpacity adjusts the alpha channel of an image.
func applyOpacity(img image.Image, opacity float64) *image.NRGBA {
	bounds := img.Bounds()
	result := image.NewNRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			newA := uint8(float64(a>>8) * opacity)
			result.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: newA,
			})
		}
	}

	return result
}
