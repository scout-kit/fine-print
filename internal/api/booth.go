package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/scout-kit/fine-print/internal/db"
)

// BoothPrint combines save edits + approve + render + queue in one call.
// Used by photo booth projects to skip admin review and print immediately.
func (h *Handlers) BoothPrint(w http.ResponseWriter, r *http.Request) {
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

	// Verify project is booth type
	project, err := h.queries.GetProject(r.Context(), photo.ProjectID)
	if err != nil || project == nil {
		writeError(w, http.StatusBadRequest, "project not found")
		return
	}
	if project.ProjectTypeID != db.ProjectTypeBooth {
		writeError(w, http.StatusBadRequest, "project is not a photo booth")
		return
	}

	// Save edits (same as SaveEdits)
	var req struct {
		CropX            float64         `json:"crop_x"`
		CropY            float64         `json:"crop_y"`
		CropWidth        float64         `json:"crop_width"`
		CropHeight       float64         `json:"crop_height"`
		Rotation         float64         `json:"rotation"`
		Brightness       *float64        `json:"brightness"`
		Contrast         *float64        `json:"contrast"`
		Saturation       *float64        `json:"saturation"`
		OverlayOverrides json.RawMessage `json:"overlay_overrides"`
		TextOverrides    json.RawMessage `json:"text_overrides"`
		Copies           *int            `json:"copies"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Save transform
	h.queries.UpsertPhotoTransform(ctx, &db.PhotoTransform{
		PhotoID:    id,
		CropX:      req.CropX,
		CropY:      req.CropY,
		CropWidth:  req.CropWidth,
		CropHeight: req.CropHeight,
		Rotation:   req.Rotation,
	})

	// Save overrides
	override := &db.PhotoOverride{PhotoID: id}
	if req.Brightness != nil {
		override.Brightness = sql.NullFloat64{Float64: *req.Brightness, Valid: true}
	}
	if req.Contrast != nil {
		override.Contrast = sql.NullFloat64{Float64: *req.Contrast, Valid: true}
	}
	if req.Saturation != nil {
		override.Saturation = sql.NullFloat64{Float64: *req.Saturation, Valid: true}
	}
	h.queries.UpsertPhotoOverride(ctx, override)

	// Update copies
	if req.Copies != nil && *req.Copies >= 1 {
		h.queries.UpdatePhotoCopies(ctx, id, *req.Copies)
	}

	// Clear render cache
	h.clearPhotoRender(ctx, photo)

	// Auto-approve: set status to approved and render + queue
	h.queries.UpdatePhotoStatus(ctx, id, db.PhotoStatusApproved)
	go h.renderAndQueue(photo)

	writeJSON(w, http.StatusOK, map[string]string{"status": "printing"})
}
