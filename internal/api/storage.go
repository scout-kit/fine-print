package api

import (
	"net/http"
)

// GetStorage returns the current disk usage snapshot for the data volume.
// Public-ish — exposed on the admin namespace so the UI can show a banner
// without needing a separate poll loop, but no secrets leak.
func (h *Handlers) GetStorage(w http.ResponseWriter, r *http.Request) {
	if h.diskGuard == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	usage, err := h.diskGuard.Usage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read disk usage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"usage":   usage,
	})
}
