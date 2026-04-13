package api

import (
	"net/http"
)

// SyncPrinters discovers CUPS printers and syncs them to the DB.
func (h *Handlers) SyncPrinters(w http.ResponseWriter, r *http.Request) {
	cupsPrinters, err := h.printer.ListPrinters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list CUPS printers")
		return
	}

	// Add newly detected printers as DISABLED. Don't change existing ones.
	for _, p := range cupsPrinters {
		h.queries.InsertPrinterIfNotExists(r.Context(), p.Name)
	}

	assignments, err := h.queries.ListPrinterAssignments(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	writeJSON(w, http.StatusOK, assignments)
}

// UpdatePrinterEnabled enables or disables a printer.
func (h *Handlers) UpdatePrinterEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.queries.UpsertPrinterAssignment(r.Context(), req.Name, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update printer")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetPrinterSettings returns the current printer mode settings.
func (h *Handlers) GetPrinterSettings(w http.ResponseWriter, r *http.Request) {
	mode, _ := h.queries.GetSetting(r.Context(), "printer_mode")
	if mode == "" {
		mode = "round_robin"
	}

	assignments, _ := h.queries.ListPrinterAssignments(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"mode":     mode,
		"printers": assignments,
	})
}

// UpdatePrinterMode sets the printer assignment mode (round_robin or manual).
func (h *Handlers) UpdatePrinterMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"` // "round_robin" or "manual"
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Mode != "round_robin" && req.Mode != "manual" {
		writeError(w, http.StatusBadRequest, "mode must be 'round_robin' or 'manual'")
		return
	}

	if err := h.queries.SetSetting(r.Context(), "printer_mode", req.Mode); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update mode")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
