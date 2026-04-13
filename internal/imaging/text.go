package imaging

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/fogleman/gg"
)

// TextParams defines how text is rendered on an image.
type TextParams struct {
	Text       string  `json:"text"`
	FontPath   string  `json:"font_path"`
	FontSize   float64 `json:"font_size"`
	Color      string  `json:"color"`   // Hex color, e.g., "#FFFFFF"
	X          float64 `json:"x"`       // Normalized position 0-1
	Y          float64 `json:"y"`
	Opacity    float64 `json:"opacity"` // 0-1
}

// RenderText draws text onto an image at the specified position.
func RenderText(base image.Image, params TextParams) (image.Image, error) {
	bounds := base.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	dc := gg.NewContextForImage(base)

	// Scale font size relative to image width so the stored value is intuitive.
	// A value of 100 should produce clearly readable text on a 4x6 print.
	// The image is typically 1800px wide. Scale so 100 → ~300px (1 inch at 300dpi).
	imgWidth := float64(bounds.Dx())
	scaledSize := params.FontSize * (imgWidth / 600.0)

	// Load font
	if params.FontPath != "" {
		if err := dc.LoadFontFace(params.FontPath, scaledSize); err != nil {
			return nil, fmt.Errorf("loading font %s: %w", params.FontPath, err)
		}
	} else {
		dc.LoadFontFace("/System/Library/Fonts/Helvetica.ttc", scaledSize)
	}

	// Parse color
	clr, err := parseHexColor(params.Color)
	if err != nil {
		clr = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}

	// Apply opacity to color
	clr.A = uint8(float64(clr.A) * params.Opacity)
	dc.SetColor(clr)

	// Calculate pixel position — X,Y is top-left of text box
	posX := params.X * float64(w)
	posY := params.Y * float64(h)

	// DrawString places text at the baseline. Offset Y by the font ascent
	// so that posY represents the top of the text, matching the canvas preview.
	_, textH := dc.MeasureString(params.Text)
	dc.DrawString(params.Text, posX, posY+textH)

	return dc.Image(), nil
}

// parseHexColor parses a hex color string like "#FF0000" or "#ff0000".
func parseHexColor(hex string) (color.NRGBA, error) {
	hex = strings.TrimPrefix(hex, "#")

	var r, g, b uint8
	switch len(hex) {
	case 6:
		_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		if err != nil {
			return color.NRGBA{}, err
		}
	case 3:
		_, err := fmt.Sscanf(hex, "%1x%1x%1x", &r, &g, &b)
		if err != nil {
			return color.NRGBA{}, err
		}
		r *= 17
		g *= 17
		b *= 17
	default:
		return color.NRGBA{}, fmt.Errorf("invalid hex color: %s", hex)
	}

	return color.NRGBA{R: r, G: g, B: b, A: 255}, nil
}
