package api

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// ListFonts returns all uploaded fonts.
func (h *Handlers) ListFonts(w http.ResponseWriter, r *http.Request) {
	fonts, err := h.queries.ListFonts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list fonts")
		return
	}
	writeJSON(w, http.StatusOK, fonts)
}

// UploadFont handles TTF font file uploads.
func (h *Handlers) UploadFont(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10MB
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("font")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing font field")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
		writeError(w, http.StatusBadRequest, "font must be a TTF, OTF, or TTC file")
		return
	}

	// Derive a display name from the filename
	name := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))

	font := &db.Font{
		Name:       name,
		Filename:   header.Filename,
		StorageKey: "",
	}

	if err := h.queries.CreateFont(r.Context(), font); err != nil {
		log.Printf("Error creating font record: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create font")
		return
	}

	storageKey := fmt.Sprintf("%d%s", font.ID, ext)
	if err := h.store.Save(storage.BucketFonts, storageKey, file); err != nil {
		log.Printf("Error saving font file: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save font")
		return
	}

	font.StorageKey = storageKey
	h.queries.UpdateFontStorageKey(r.Context(), font.ID, storageKey)

	writeJSON(w, http.StatusCreated, font)
}

// ServeFont serves a font file for use in the frontend.
func (h *Handlers) ServeFont(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid font id")
		return
	}

	font, err := h.queries.GetFont(r.Context(), id)
	if err != nil || font == nil {
		writeError(w, http.StatusNotFound, "font not found")
		return
	}

	filePath := h.store.Path(storage.BucketFonts, font.StorageKey)
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filePath)
}

// DeleteFont removes an uploaded font.
func (h *Handlers) DeleteFont(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid font id")
		return
	}

	font, err := h.queries.GetFont(r.Context(), id)
	if err != nil || font == nil {
		writeError(w, http.StatusNotFound, "font not found")
		return
	}

	h.store.Delete(storage.BucketFonts, font.StorageKey)
	h.queries.DeleteFont(r.Context(), id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
