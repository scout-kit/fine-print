package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// GetEdits returns the full edit state for a photo (transform + overrides + project defaults).
func (h *Handlers) GetEdits(w http.ResponseWriter, r *http.Request) {
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

	// Auth: guest must own the photo, or be admin
	if !h.canEditPhoto(r, photo) {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}

	transform, _ := h.queries.GetPhotoTransform(r.Context(), id)
	override, _ := h.queries.GetPhotoOverride(r.Context(), id)
	project, _ := h.queries.GetProject(r.Context(), photo.ProjectID)

	// Build response
	type transformResp struct {
		CropX      float64 `json:"crop_x"`
		CropY      float64 `json:"crop_y"`
		CropWidth  float64 `json:"crop_width"`
		CropHeight float64 `json:"crop_height"`
		Rotation   float64 `json:"rotation"`
	}

	type overrideResp struct {
		Brightness      *float64 `json:"brightness"`
		Contrast        *float64 `json:"contrast"`
		Saturation      *float64 `json:"saturation"`
		OverlayOverrides json.RawMessage `json:"overlay_overrides"`
		TextOverrides    json.RawMessage `json:"text_overrides"`
	}

	type projectResp struct {
		Brightness float64 `json:"brightness"`
		Contrast   float64 `json:"contrast"`
		Saturation float64 `json:"saturation"`
	}

	resp := map[string]any{
		"copies": max(photo.Copies, 1),
	}

	if transform != nil {
		resp["transform"] = transformResp{
			CropX:      transform.CropX,
			CropY:      transform.CropY,
			CropWidth:  transform.CropWidth,
			CropHeight: transform.CropHeight,
			Rotation:   transform.Rotation,
		}
	}

	if override != nil {
		or := overrideResp{}
		if override.Brightness.Valid {
			or.Brightness = &override.Brightness.Float64
		}
		if override.Contrast.Valid {
			or.Contrast = &override.Contrast.Float64
		}
		if override.Saturation.Valid {
			or.Saturation = &override.Saturation.Float64
		}
		if override.OverlayOverrides.Valid {
			or.OverlayOverrides = json.RawMessage(override.OverlayOverrides.String)
		}
		if override.TextOverrides.Valid {
			or.TextOverrides = json.RawMessage(override.TextOverrides.String)
		}
		resp["overrides"] = or
	}

	if project != nil {
		resp["project"] = projectResp{
			Brightness: project.Brightness,
			Contrast:   project.Contrast,
			Saturation: project.Saturation,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// SaveEdits saves the full edit state for a photo and clears the render cache.
func (h *Handlers) SaveEdits(w http.ResponseWriter, r *http.Request) {
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

	if !h.canEditPhoto(r, photo) {
		writeError(w, http.StatusForbidden, "not authorized")
		return
	}

	var req struct {
		CropX      float64          `json:"crop_x"`
		CropY      float64          `json:"crop_y"`
		CropWidth  float64          `json:"crop_width"`
		CropHeight float64          `json:"crop_height"`
		Rotation   float64          `json:"rotation"`
		Brightness *float64         `json:"brightness"`
		Contrast   *float64         `json:"contrast"`
		Saturation *float64         `json:"saturation"`
		OverlayOverrides json.RawMessage `json:"overlay_overrides"`
		TextOverrides    json.RawMessage `json:"text_overrides"`
		Copies     *int             `json:"copies"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Upsert transform
	h.queries.UpsertPhotoTransform(ctx, &db.PhotoTransform{
		PhotoID:    id,
		CropX:      req.CropX,
		CropY:      req.CropY,
		CropWidth:  req.CropWidth,
		CropHeight: req.CropHeight,
		Rotation:   req.Rotation,
	})

	// Upsert overrides
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
	if len(req.OverlayOverrides) > 0 && string(req.OverlayOverrides) != "null" {
		override.OverlayOverrides = sql.NullString{String: string(req.OverlayOverrides), Valid: true}
	}
	if len(req.TextOverrides) > 0 && string(req.TextOverrides) != "null" {
		override.TextOverrides = sql.NullString{String: string(req.TextOverrides), Valid: true}
	}
	h.queries.UpsertPhotoOverride(ctx, override)

	// Update copies
	if req.Copies != nil && *req.Copies >= 1 {
		h.queries.UpdatePhotoCopies(ctx, id, *req.Copies)
	}

	// Clear render cache
	h.clearPhotoRender(ctx, photo)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReprintPhoto re-queues a printed/failed photo for printing.
func (h *Handlers) ReprintPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	var req struct {
		ClearCache bool `json:"clear_cache"`
	}
	readJSON(r, &req)

	photo, err := h.queries.GetPhoto(r.Context(), id)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}

	ctx := r.Context()

	if req.ClearCache {
		h.clearPhotoRender(ctx, photo)
		// Re-render and queue (same as approve flow)
		go h.renderAndQueue(photo)
	} else {
		// Use existing render, just create new print jobs
		copies := max(photo.Copies, 1)
		for i := 0; i < copies; i++ {
			pos, _ := h.queries.GetNextQueuePosition(ctx)
			h.queries.CreatePrintJob(ctx, &db.PrintJob{
				PhotoID:  photo.ID,
				Position: pos,
				StatusID: db.PrintJobStatusQueued,
			})
		}
		h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusQueued)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

// clearPhotoRender deletes the cached render for a photo.
func (h *Handlers) clearPhotoRender(ctx context.Context, photo *db.Photo) {
	if photo.RenderedKey.Valid {
		h.store.Delete(storage.BucketRendered, photo.RenderedKey.String)
	}
	h.queries.ClearPhotoRendered(ctx, photo.ID)
}

// invalidateProjectRenders clears all cached renders for non-printed photos in a project.
func (h *Handlers) invalidateProjectRenders(ctx context.Context, projectID uint64) {
	// Delete render files from storage
	photos, err := h.queries.ListRenderedPhotosByProject(ctx, projectID)
	if err != nil {
		log.Printf("Error listing rendered photos for invalidation: %v", err)
		return
	}
	for _, photo := range photos {
		if photo.RenderedKey.Valid {
			h.store.Delete(storage.BucketRendered, photo.RenderedKey.String)
		}
	}
	// Bulk clear rendered_key in DB
	h.queries.ClearProjectRenderedPhotos(ctx, projectID)
}

// canEditPhoto checks if the request is authorized to edit this photo.
// Any guest with a session can edit any photo they have access to.
// Admins can always edit.
func (h *Handlers) canEditPhoto(r *http.Request, photo *db.Photo) bool {
	// Check admin session
	cookie, err := r.Cookie("fineprint_session")
	if err == nil {
		session, _ := h.queries.GetAdminSessionByToken(r.Context(), cookie.Value)
		if session != nil {
			return true
		}
	}

	// Any guest with a session can edit any photo
	_, err = r.Cookie("fineprint_guest")
	if err == nil {
		return true
	}

	return false
}
