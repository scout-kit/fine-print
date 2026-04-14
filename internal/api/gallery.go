package api

import (
	"net/http"
	"strconv"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// Gallery returns photos for the gallery view.
// Optional query param: ?project_id=N to filter by project. Without it, returns all.
func (h *Handlers) Gallery(w http.ResponseWriter, r *http.Request) {
	var photos []db.Photo
	var err error

	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr != "" {
		projectID, parseErr := strconv.ParseUint(projectIDStr, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		photos, err = h.queries.ListGalleryPhotos(r.Context(), projectID)
	} else {
		photos, err = h.queries.ListAllGalleryPhotos(r.Context())
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list gallery")
		return
	}

	type galleryPhoto struct {
		ID         uint64 `json:"id"`
		ProjectID  uint64 `json:"project_id"`
		StatusID   uint   `json:"status_id"`
		Status     string `json:"status"`
		HasPreview bool   `json:"has_preview"`
		HasRender  bool   `json:"has_render"`
		SessionID  string `json:"session_id"`
		CreatedAt  string `json:"created_at"`
	}

	result := make([]galleryPhoto, 0, len(photos))
	for _, p := range photos {
		result = append(result, galleryPhoto{
			ID:         p.ID,
			ProjectID:  p.ProjectID,
			StatusID:   p.StatusID,
			Status:     db.PhotoStatusName(p.StatusID),
			HasPreview: p.PreviewKey.Valid,
			HasRender:  p.RenderedKey.Valid,
			SessionID:  p.SessionID,
			CreatedAt:  p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// DeleteOwnPhoto allows a guest to delete their own photo (matched by session).
func (h *Handlers) DeleteOwnPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	cookie, err := r.Cookie("fineprint_guest")
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "no session")
		return
	}

	photo, err := h.queries.GetPhoto(r.Context(), id)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}

	if photo.SessionID != cookie.Value {
		writeError(w, http.StatusForbidden, "not your photo")
		return
	}

	if photo.StatusID == db.PhotoStatusPrinting {
		writeError(w, http.StatusBadRequest, "cannot delete while printing")
		return
	}

	// Delete files
	h.store.Delete(storage.BucketOriginals, photo.OriginalKey)
	if photo.PreviewKey.Valid {
		h.store.Delete(storage.BucketPreviews, photo.PreviewKey.String)
	}
	if photo.RenderedKey.Valid {
		h.store.Delete(storage.BucketRendered, photo.RenderedKey.String)
	}

	// Delete print jobs (no cascade on FK)
	h.queries.DeletePrintJobsByPhoto(r.Context(), id)

	h.queries.DeletePhoto(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
