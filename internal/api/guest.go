package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/qrcode"
	"github.com/scout-kit/fine-print/internal/storage"
)

const maxUploadSize = 50 * 1024 * 1024 // 50MB

// UploadPhoto handles photo uploads from guests.
func (h *Handlers) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing photo field")
		return
	}
	defer file.Close()

	// Validate file type
	format := imaging.FormatFromFilename(header.Filename)
	if format == "" {
		writeError(w, http.StatusBadRequest, "unsupported image format")
		return
	}

	// Get or create guest session
	sessionID := getOrCreateGuestSession(w, r)

	// Get project ID from form field
	projectIDStr := r.FormValue("project_id")
	if projectIDStr == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	var projectID uint64
	fmt.Sscanf(projectIDStr, "%d", &projectID)

	project, err := h.queries.GetProject(r.Context(), projectID)
	if err != nil || project == nil {
		writeError(w, http.StatusBadRequest, "invalid project")
		return
	}

	// Create photo record
	ext := filepath.Ext(header.Filename)
	photo := &db.Photo{
		ProjectID:   projectID,
		SessionID:   sessionID,
		OriginalKey: "", // Will be set after we know the ID
		StatusID:    db.PhotoStatusUploaded,
	}

	if err := h.queries.CreatePhoto(r.Context(), photo); err != nil {
		log.Printf("Error creating photo record: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create photo")
		return
	}

	// Save original file
	originalKey := fmt.Sprintf("%d%s", photo.ID, ext)
	if err := h.store.Save(storage.BucketOriginals, originalKey, file); err != nil {
		log.Printf("Error saving original: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save photo")
		return
	}

	// Update photo with the original key in DB
	photo.OriginalKey = originalKey
	h.queries.UpdatePhotoOriginalKey(r.Context(), photo.ID, originalKey)

	// Generate preview asynchronously
	go h.generatePreview(photo)

	// Calculate default center crop transform
	// We'll set this after preview generation knows the dimensions
	go h.setDefaultTransform(photo)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     photo.ID,
		"status": db.PhotoStatusName(photo.StatusID),
	})
}

func (h *Handlers) generatePreview(photo *db.Photo) {
	originalPath := h.store.Path(storage.BucketOriginals, photo.OriginalKey)

	// Convert HEIC to JPEG if needed
	if imaging.IsHEIC(photo.OriginalKey) {
		convertedPath, cleanup, err := imaging.ConvertHEICToTemp(originalPath)
		if err != nil {
			log.Printf("Error converting HEIC: %v", err)
			return
		}
		defer cleanup()
		originalPath = convertedPath
	}

	img, err := h.pipeline.DecodeFromFile(originalPath)
	if err != nil {
		log.Printf("Error decoding image for preview: %v", err)
		return
	}

	bounds := img.Bounds()

	// Generate preview
	preview := h.pipeline.GeneratePreview(img)

	previewKey := fmt.Sprintf("%d.jpg", photo.ID)
	previewPath := h.store.Path(storage.BucketPreviews, previewKey)

	f, err := createFile(previewPath)
	if err != nil {
		log.Printf("Error creating preview file: %v", err)
		return
	}
	defer f.Close()

	if err := h.pipeline.EncodeJPEG(f, preview); err != nil {
		log.Printf("Error encoding preview: %v", err)
		return
	}

	// Get file size
	info, _ := f.Stat()
	var fileSize int64
	if info != nil {
		fileSize = info.Size()
	}

	h.queries.UpdatePhotoPreview(
		contextBackground(),
		photo.ID, previewKey,
		bounds.Dx(), bounds.Dy(),
		fileSize, "image/jpeg",
	)
}

func (h *Handlers) setDefaultTransform(photo *db.Photo) {
	originalPath := h.store.Path(storage.BucketOriginals, photo.OriginalKey)

	if imaging.IsHEIC(photo.OriginalKey) {
		convertedPath, cleanup, err := imaging.ConvertHEICToTemp(originalPath)
		if err != nil {
			return
		}
		defer cleanup()
		originalPath = convertedPath
	}

	img, err := h.pipeline.DecodeFromFile(originalPath)
	if err != nil {
		return
	}

	bounds := img.Bounds()
	defaultCrop := imaging.CenterCrop4x6(bounds.Dx(), bounds.Dy())

	h.queries.UpsertPhotoTransform(contextBackground(), &db.PhotoTransform{
		PhotoID:    photo.ID,
		CropX:      defaultCrop.CropX,
		CropY:      defaultCrop.CropY,
		CropWidth:  defaultCrop.CropWidth,
		CropHeight: defaultCrop.CropHeight,
		Rotation:   0,
	})
}

// PhotoStatus returns the current status of a photo.
func (h *Handlers) PhotoStatus(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     photo.ID,
		"status": db.PhotoStatusName(photo.StatusID),
	})
}

// PhotoPreview serves the preview image for a photo.
func (h *Handlers) PhotoPreview(w http.ResponseWriter, r *http.Request) {
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

	if !photo.PreviewKey.Valid {
		writeError(w, http.StatusNotFound, "preview not ready")
		return
	}

	previewPath := h.store.Path(storage.BucketPreviews, photo.PreviewKey.String)
	http.ServeFile(w, r, previewPath)
}

// SaveTransform saves the crop/transform data for a photo.
func (h *Handlers) SaveTransform(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	var req struct {
		CropX      float64 `json:"crop_x"`
		CropY      float64 `json:"crop_y"`
		CropWidth  float64 `json:"crop_width"`
		CropHeight float64 `json:"crop_height"`
		Rotation   float64 `json:"rotation"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	transform := &db.PhotoTransform{
		PhotoID:    id,
		CropX:      req.CropX,
		CropY:      req.CropY,
		CropWidth:  req.CropWidth,
		CropHeight: req.CropHeight,
		Rotation:   req.Rotation,
	}

	if err := h.queries.UpsertPhotoTransform(r.Context(), transform); err != nil {
		log.Printf("Error saving transform: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save transform")
		return
	}

	// Clear render cache so next render uses updated transform
	photo, _ := h.queries.GetPhoto(r.Context(), id)
	if photo != nil {
		h.clearPhotoRender(r.Context(), photo)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListProjectsPublic returns only public projects for the guest project picker.
func (h *Handlers) ListProjectsPublic(w http.ResponseWriter, r *http.Request) {
	projects, err := h.queries.ListPublicProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// GetProjectPublic returns a single project with its overlays for guests.
// Public and hidden projects are accessible; private projects are not.
func (h *Handlers) GetProjectPublic(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	project, err := h.queries.GetProject(r.Context(), id)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	// Private projects are not accessible via the public API
	if project.VisibilityID == db.VisibilityPrivate {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	overlays, _ := h.queries.ListOverlaysByProject(r.Context(), id)
	textOverlays, _ := h.queries.ListTextOverlaysByProject(r.Context(), id)

	writeJSON(w, http.StatusOK, map[string]any{
		"project":       project,
		"overlays":      overlays,
		"text_overlays": textOverlays,
	})
}

// GetProjectBySlug returns a hidden project by its slug for direct-link access.
func (h *Handlers) GetProjectBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}

	project, err := h.queries.GetProjectBySlug(r.Context(), slug)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	// Only hidden and public projects are accessible via slug
	if project.VisibilityID == db.VisibilityPrivate {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	overlays, _ := h.queries.ListOverlaysByProject(r.Context(), project.ID)
	textOverlays, _ := h.queries.ListTextOverlaysByProject(r.Context(), project.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"project":       project,
		"overlays":      overlays,
		"text_overlays": textOverlays,
	})
}

// requestOrigin returns the base URL from the incoming request (e.g. "http://192.168.1.5:8080").
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// QRCode generates a QR code PNG for the portal URL using the request's host.
func (h *Handlers) QRCode(w http.ResponseWriter, r *http.Request) {
	qrcode.GeneratePNG(w, requestOrigin(r)+"/")
}

// ProjectQRCode generates a QR code PNG for a specific project's direct link.
func (h *Handlers) ProjectQRCode(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	project, err := h.queries.GetProject(r.Context(), id)
	if err != nil || project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	if !project.Slug.Valid {
		writeError(w, http.StatusBadRequest, "project has no share link")
		return
	}

	qrcode.GeneratePNG(w, requestOrigin(r)+"/p/"+project.Slug.String)
}

func getOrCreateGuestSession(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("fineprint_guest")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	token := generateToken(16)
	http.SetCookie(w, &http.Cookie{
		Name:     "fineprint_guest",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24 hours
	})
	return token
}

func generateToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
