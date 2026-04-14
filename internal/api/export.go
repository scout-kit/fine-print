package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/scout-kit/fine-print/internal/storage"
)

// ExportProject generates a ZIP of all photos in a project.
func (h *Handlers) ExportProject(w http.ResponseWriter, r *http.Request) {
	projectIDStr := r.PathValue("project_id")
	projectID, err := strconv.ParseUint(projectIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	project, err := h.queries.GetProject(r.Context(), projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	photos, err := h.queries.ListPhotosByProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list photos")
		return
	}

	// Set headers for ZIP download
	filename := fmt.Sprintf("fine-print-%s-%s.zip", project.Name, time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	// Stream ZIP directly to response
	zw := zip.NewWriter(w)
	defer zw.Close()

	for i, photo := range photos {
		// Add original
		if err := addFileToZip(zw, h.store, storage.BucketOriginals, photo.OriginalKey,
			fmt.Sprintf("originals/%03d_%s", i+1, photo.OriginalKey)); err != nil {
			log.Printf("Error adding original to zip: %v", err)
			continue
		}

		// Add rendered (if exists)
		if photo.RenderedKey.Valid {
			if err := addFileToZip(zw, h.store, storage.BucketRendered, photo.RenderedKey.String,
				fmt.Sprintf("processed/%03d_%s", i+1, photo.RenderedKey.String)); err != nil {
				log.Printf("Error adding rendered to zip: %v", err)
			}
		}
	}
}

// ExportPhotos generates a ZIP of selected photos by ID.
func (h *Handlers) ExportPhotos(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PhotoIDs []uint64 `json:"photo_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.PhotoIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no photo IDs provided")
		return
	}
	if len(body.PhotoIDs) > 500 {
		writeError(w, http.StatusBadRequest, "too many photos (max 500)")
		return
	}

	photos, err := h.queries.GetPhotosByIDs(r.Context(), body.PhotoIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch photos")
		return
	}

	filename := fmt.Sprintf("fine-print-selection-%s.zip", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	zw := zip.NewWriter(w)
	defer zw.Close()

	for i, photo := range photos {
		if err := addFileToZip(zw, h.store, storage.BucketOriginals, photo.OriginalKey,
			fmt.Sprintf("originals/%03d_%s", i+1, photo.OriginalKey)); err != nil {
			log.Printf("Error adding original to zip: %v", err)
			continue
		}
		if photo.RenderedKey.Valid {
			if err := addFileToZip(zw, h.store, storage.BucketRendered, photo.RenderedKey.String,
				fmt.Sprintf("processed/%03d_%s", i+1, photo.RenderedKey.String)); err != nil {
				log.Printf("Error adding rendered to zip: %v", err)
			}
		}
	}
}

func addFileToZip(zw *zip.Writer, store storage.Store, bucket, key, zipPath string) error {
	r, err := store.Open(bucket, key)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := zw.Create(zipPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	return err
}
