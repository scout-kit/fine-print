package api

import (
	"net/http"
	"time"
)

// Healthz is a liveness probe. If the HTTP server is serving at all, the
// process is alive — so this is always 200 with a cheap payload.
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// Readyz is a readiness probe. It fails (503) when the app is running but
// can't serve its purpose — DB unreachable, or (when the disk guard is
// configured) free space below the hard threshold. Monitoring can pause
// traffic / restart based on this signal.
func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	overall := http.StatusOK

	// DB ping — issuing any query proves the connection pool is live.
	if h.queries != nil {
		if _, err := h.queries.GetSetting(r.Context(), "__readyz_probe__"); err != nil {
			checks["db"] = "fail: " + err.Error()
			overall = http.StatusServiceUnavailable
		} else {
			checks["db"] = "ok"
		}
	}

	// Disk guard — empty status when the guard isn't wired yet.
	if h.diskGuard != nil {
		status, err := h.diskGuard.ReadyzStatus()
		if err != nil {
			checks["disk"] = "fail: " + err.Error()
			overall = http.StatusServiceUnavailable
		} else if status != "ok" {
			checks["disk"] = status
			overall = http.StatusServiceUnavailable
		} else {
			checks["disk"] = "ok"
		}
	}

	body := map[string]any{
		"status":     "ready",
		"checks":     checks,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	}
	if overall != http.StatusOK {
		body["status"] = "not_ready"
	}
	writeJSON(w, overall, body)
}
