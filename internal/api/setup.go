package api

import (
	"net/http"

	"github.com/scout-kit/fine-print/internal/settings"

	"golang.org/x/crypto/bcrypt"
)

// needsSetup reports whether the first-run wizard should be shown. The
// sentinel is the admin password hash: when it's missing the wizard runs.
// Once the wizard (or a legacy first-login seed) has set the hash, we never
// prompt again.
func (h *Handlers) needsSetup(r *http.Request) bool {
	hash, _ := h.queries.GetSetting(r.Context(), settings.KeyAdminPasswordHash)
	return hash == ""
}

// SetupStatus reports whether the first-run wizard is required and returns
// contextual data for it (currently: detected CUPS printers).
//
// Public endpoint — called before any login exists.
func (h *Handlers) SetupStatus(w http.ResponseWriter, r *http.Request) {
	needs := h.needsSetup(r)
	resp := map[string]any{
		"needs_setup": needs,
	}
	if needs {
		// Best-effort printer detection so the wizard can offer a picker.
		if printers, err := h.printer.ListPrinters(); err == nil {
			resp["printers"] = printers
		} else {
			resp["printers"] = []struct{}{}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// CompleteSetup accepts the initial admin password plus optional hotspot
// and printer defaults, persists them, and marks the wizard done. Refuses
// to run a second time so an attacker on the LAN can't reset an already-
// configured instance.
//
// Public endpoint — only accepts writes while needsSetup is true.
func (h *Handlers) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	if !h.needsSetup(r) {
		writeError(w, http.StatusForbidden, "setup has already been completed")
		return
	}

	var req struct {
		AdminPassword   string `json:"admin_password"`
		HotspotSSID     string `json:"hotspot_ssid"`
		HotspotPassword string `json:"hotspot_password"`
		PrinterName     string `json:"printer_name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.AdminPassword) < 4 {
		writeError(w, http.StatusBadRequest, "admin password must be at least 4 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.queries.SetSetting(r.Context(), settings.KeyAdminPasswordHash, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store password")
		return
	}

	// Hotspot is optional. Empty SSID → leave the seeded default in place.
	if req.HotspotSSID != "" {
		if err := h.settings.Set(r.Context(), settings.KeyHotspotSSID, req.HotspotSSID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store hotspot ssid")
			return
		}
	}
	// Password is allowed to be empty (open network).
	if err := h.settings.Set(r.Context(), settings.KeyHotspotPassword, req.HotspotPassword); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store hotspot password")
		return
	}
	if req.PrinterName != "" {
		if err := h.settings.Set(r.Context(), settings.KeyPrinterName, req.PrinterName); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store printer")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
