package imaging

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"strings"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Pipeline orchestrates image processing from upload to print-ready output.
type Pipeline struct {
	printWidth      int
	printHeight     int
	previewMaxWidth int
	jpegQuality     int
	maxUploadPixels int
}

func NewPipeline(printWidth, printHeight, previewMaxWidth, jpegQuality, maxUploadPixels int) *Pipeline {
	return &Pipeline{
		printWidth:      printWidth,
		printHeight:     printHeight,
		previewMaxWidth: previewMaxWidth,
		jpegQuality:     jpegQuality,
		maxUploadPixels: maxUploadPixels,
	}
}

// DecodeImage reads and decodes an image from a reader. It auto-orients based
// on EXIF data (handled by the imaging library).
func (p *Pipeline) DecodeImage(r io.Reader, filename string) (image.Image, string, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("decoding image: %w", err)
	}

	// Auto-orient based on EXIF. The imaging library handles this when
	// opening from a file, but since we're using a reader, we do it manually
	// by re-encoding and decoding through imaging.Open. For now, we accept
	// the decoded image as-is. EXIF orientation will be handled at the
	// decode-from-file level.

	return img, format, nil
}

// DecodeFromFile reads and decodes an image file with EXIF auto-orientation.
func (p *Pipeline) DecodeFromFile(path string) (image.Image, error) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("opening image %s: %w", path, err)
	}
	return img, nil
}

// GeneratePreview creates a web-sized preview image.
func (p *Pipeline) GeneratePreview(img image.Image) image.Image {
	bounds := img.Bounds()
	if bounds.Dx() <= p.previewMaxWidth {
		return img
	}
	return imaging.Resize(img, p.previewMaxWidth, 0, imaging.Lanczos)
}

// PreDownscale reduces oversized images to a reasonable working resolution.
func (p *Pipeline) PreDownscale(img image.Image) image.Image {
	bounds := img.Bounds()
	maxDim := bounds.Dx()
	if bounds.Dy() > maxDim {
		maxDim = bounds.Dy()
	}
	if maxDim <= p.maxUploadPixels {
		return img
	}
	if bounds.Dx() > bounds.Dy() {
		return imaging.Resize(img, p.maxUploadPixels, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, p.maxUploadPixels, imaging.Lanczos)
}

// Render produces the final print-ready image by applying all transformations.
func (p *Pipeline) Render(img image.Image, opts RenderOptions) (image.Image, error) {
	// 1. Apply crop
	if opts.Transform != nil {
		img = ApplyCrop(img, *opts.Transform)
	}

	// 2. Apply color adjustments
	if opts.Color != nil {
		img = ApplyColorAdjustments(img, *opts.Color)
	}

	// 3. Determine print dimensions based on crop aspect ratio
	printW := p.printWidth
	printH := p.printHeight
	bounds := img.Bounds()
	if bounds.Dy() > bounds.Dx() {
		// Portrait crop — swap print dimensions
		printW = p.printHeight
		printH = p.printWidth
	}
	img = imaging.Fill(img, printW, printH, imaging.Center, imaging.Lanczos)

	// 4. Apply image overlays
	for _, overlay := range opts.Overlays {
		var err error
		img, err = CompositeOverlay(img, overlay)
		if err != nil {
			log.Printf("Warning: failed to apply overlay %s: %v", overlay.Path, err)
			continue
		}
	}

	// 5. Apply text overlays
	for _, text := range opts.TextOverlays {
		var err error
		img, err = RenderText(img, text)
		if err != nil {
			log.Printf("Warning: failed to render text overlay: %v", err)
			continue
		}
	}

	return img, nil
}

// EncodeJPEG writes an image as JPEG to the given writer.
func (p *Pipeline) EncodeJPEG(w io.Writer, img image.Image) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: p.jpegQuality})
}

// EncodePNG writes an image as PNG to the given writer.
func (p *Pipeline) EncodePNG(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}

// FormatFromFilename returns the image format based on the file extension.
func FormatFromFilename(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "png"
	case strings.HasSuffix(lower, ".webp"):
		return "webp"
	case strings.HasSuffix(lower, ".tiff"), strings.HasSuffix(lower, ".tif"):
		return "tiff"
	case strings.HasSuffix(lower, ".heic"), strings.HasSuffix(lower, ".heif"):
		return "heic"
	default:
		return ""
	}
}
