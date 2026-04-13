package api

import (
	"fmt"
	"net/http"

	"github.com/scout-kit/fine-print/internal/storage"
)

// DownloadOriginal serves the original uploaded image.
func (h *Handlers) DownloadOriginal(w http.ResponseWriter, r *http.Request) {
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

	filePath := h.store.Path(storage.BucketOriginals, photo.OriginalKey)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", photo.OriginalKey))
	http.ServeFile(w, r, filePath)
}

// DownloadRendered serves the print-ready rendered image.
func (h *Handlers) DownloadRendered(w http.ResponseWriter, r *http.Request) {
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

	if !photo.RenderedKey.Valid {
		writeError(w, http.StatusNotFound, "rendered image not available")
		return
	}

	filePath := h.store.Path(storage.BucketRendered, photo.RenderedKey.String)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"print_%d.jpg\"", photo.ID))
	http.ServeFile(w, r, filePath)
}
