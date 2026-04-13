package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/storage"
)

// RenderPreview generates (or serves cached) the final rendered image for a photo
// without approving it. This lets admins and guests see what the print will look like.
func (h *Handlers) RenderPreview(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	photo, err := h.queries.GetPhoto(r.Context(), id)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}

	// If already rendered, serve it
	if photo.RenderedKey.Valid {
		filePath := h.store.Path(storage.BucketRendered, photo.RenderedKey.String)
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filePath)
		return
	}

	// Generate on the fly
	originalPath := h.store.Path(storage.BucketOriginals, photo.OriginalKey)

	if imaging.IsHEIC(photo.OriginalKey) {
		convertedPath, cleanup, convErr := imaging.ConvertHEICToTemp(originalPath)
		if convErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to convert HEIC")
			return
		}
		defer cleanup()
		originalPath = convertedPath
	}

	img, err := h.pipeline.DecodeFromFile(originalPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decode image")
		return
	}

	// Get transform
	transform, _ := h.queries.GetPhotoTransform(r.Context(), id)
	var transformParams *imaging.TransformParams
	if transform != nil {
		transformParams = &imaging.TransformParams{
			CropX:      transform.CropX,
			CropY:      transform.CropY,
			CropWidth:  transform.CropWidth,
			CropHeight: transform.CropHeight,
			Rotation:   transform.Rotation,
		}
	}

	// Get project color settings
	project, _ := h.queries.GetProject(r.Context(), photo.ProjectID)
	var colorParams *imaging.ColorParams
	if project != nil && (project.Brightness != 0 || project.Contrast != 0 || project.Saturation != 0) {
		colorParams = &imaging.ColorParams{
			Brightness: project.Brightness,
			Contrast:   project.Contrast,
			Saturation: project.Saturation,
		}
	}

	// Per-image overrides
	override, _ := h.queries.GetPhotoOverride(r.Context(), id)
	if override != nil {
		if override.Brightness.Valid || override.Contrast.Valid || override.Saturation.Valid {
			if colorParams == nil {
				colorParams = &imaging.ColorParams{}
			}
			if override.Brightness.Valid {
				colorParams.Brightness = override.Brightness.Float64
			}
			if override.Contrast.Valid {
				colorParams.Contrast = override.Contrast.Float64
			}
			if override.Saturation.Valid {
				colorParams.Saturation = override.Saturation.Float64
			}
		}
	}

	// Determine orientation from crop to pick the right template overlays
	bounds := img.Bounds()
	orientationID := imaging.OrientationFromTransform(transformParams, bounds.Dx(), bounds.Dy())

	// Get overlays for this orientation
	var overlayParams []imaging.OverlayParams
	overlays, _ := h.queries.ListOverlaysByProjectOrientation(r.Context(), photo.ProjectID, orientationID)
	for _, o := range overlays {
		overlayParams = append(overlayParams, imaging.OverlayParams{
			Path:    h.store.Path(storage.BucketOverlays, o.StorageKey),
			X:       o.X,
			Y:       o.Y,
			Width:   o.Width,
			Height:  o.Height,
			Opacity: o.Opacity,
		})
	}

	// Get text overlays
	var textParams []imaging.TextParams
	textOverlays, _ := h.queries.ListTextOverlaysByProjectOrientation(r.Context(), photo.ProjectID, orientationID)
	for _, t := range textOverlays {
		textParams = append(textParams, imaging.TextParams{
			Text:     t.Text,
			FontPath: t.FontFamily,
			FontSize: t.FontSize,
			Color:    t.Color,
			X:        t.X,
			Y:        t.Y,
			Opacity:  t.Opacity,
		})
	}

	// Render
	rendered, err := h.pipeline.Render(img, imaging.RenderOptions{
		Transform:    transformParams,
		Color:        colorParams,
		Overlays:     overlayParams,
		TextOverlays: textParams,
	})
	if err != nil {
		log.Printf("Error rendering preview for photo %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to render")
		return
	}

	// Save it so we don't re-render next time
	renderedKey := fmt.Sprintf("%d.jpg", photo.ID)
	renderedPath := h.store.Path(storage.BucketRendered, renderedKey)

	f, err := createFile(renderedPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save render")
		return
	}

	if err := h.pipeline.EncodeJPEG(f, rendered); err != nil {
		f.Close()
		writeError(w, http.StatusInternalServerError, "failed to encode render")
		return
	}
	f.Close()

	h.queries.UpdatePhotoRendered(r.Context(), photo.ID, renderedKey)

	// Serve the rendered file
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, renderedPath)
}
