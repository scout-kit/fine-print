package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/storage"

	"golang.org/x/crypto/bcrypt"
)

// AdminLogin authenticates an admin user and sets a session cookie.
func (h *Handlers) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check password against stored hash
	storedHash, _ := h.queries.GetSetting(r.Context(), "admin_password_hash")

	if storedHash == "" {
		// First login — set the password from config
		hash, err := bcrypt.GenerateFromPassword([]byte(h.cfg.Admin.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		h.queries.SetSetting(r.Context(), "admin_password_hash", string(hash))
		storedHash = string(hash)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	// Create session
	token := generateToken(32)
	session := &db.AdminSession{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	if err := h.queries.CreateAdminSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "fineprint_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24 hours
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AdminSession checks if the current session is valid.
func (h *Handlers) AdminSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("fineprint_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	session, err := h.queries.GetAdminSessionByToken(r.Context(), cookie.Value)
	if err != nil || session == nil {
		writeError(w, http.StatusUnauthorized, "session expired")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

// AdminLogout invalidates the current session.
func (h *Handlers) AdminLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("fineprint_session")
	if err == nil {
		h.queries.DeleteAdminSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "fineprint_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListPhotos returns all photos with optional status filter.
func (h *Handlers) ListPhotos(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var photos []db.Photo
	var err error

	if statusFilter != "" {
		var statusID uint
		switch statusFilter {
		case "uploaded":
			statusID = db.PhotoStatusUploaded
		case "approved":
			statusID = db.PhotoStatusApproved
		case "queued":
			statusID = db.PhotoStatusQueued
		case "printing":
			statusID = db.PhotoStatusPrinting
		case "printed":
			statusID = db.PhotoStatusPrinted
		case "failed":
			statusID = db.PhotoStatusFailed
		case "rejected":
			statusID = db.PhotoStatusRejected
		default:
			writeError(w, http.StatusBadRequest, "invalid status filter")
			return
		}
		photos, err = h.queries.ListPhotosByStatus(r.Context(), statusID)
	} else {
		// Optional project filter
		projectIDStr := r.URL.Query().Get("project_id")
		if projectIDStr != "" {
			var projectID uint64
			fmt.Sscanf(projectIDStr, "%d", &projectID)
			photos, err = h.queries.ListPhotosByProject(r.Context(), projectID)
		} else {
			photos, err = h.queries.ListAllPhotos(r.Context())
		}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list photos")
		return
	}

	writeJSON(w, http.StatusOK, photos)
}

// ApprovePhoto marks a photo as approved and queues it for printing.
func (h *Handlers) ApprovePhoto(w http.ResponseWriter, r *http.Request) {
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

	if photo.StatusID != db.PhotoStatusUploaded {
		writeError(w, http.StatusBadRequest, "photo is not in uploaded status")
		return
	}

	// Update status to approved
	if err := h.queries.UpdatePhotoStatus(r.Context(), id, db.PhotoStatusApproved); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to approve photo")
		return
	}

	// Render the print-ready image async, then queue for printing
	go h.renderAndQueue(photo)

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (h *Handlers) renderAndQueue(photo *db.Photo) {
	ctx := contextBackground()

	// Load original
	originalPath := h.store.Path(storage.BucketOriginals, photo.OriginalKey)

	// Convert HEIC if needed
	if imaging.IsHEIC(photo.OriginalKey) {
		convertedPath, cleanup, convErr := imaging.ConvertHEICToTemp(originalPath)
		if convErr != nil {
			log.Printf("Error converting HEIC for render: %v", convErr)
			h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusFailed)
			return
		}
		defer cleanup()
		originalPath = convertedPath
	}

	img, err := h.pipeline.DecodeFromFile(originalPath)
	if err != nil {
		log.Printf("Error decoding original for render: %v", err)
		h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusFailed)
		return
	}

	// Get transform
	transform, _ := h.queries.GetPhotoTransform(ctx, photo.ID)
	var transformParams *imaging.TransformParams
	if transform != nil {
		transformParams = &imaging.TransformParams{
			CropX:      transform.CropX,
			CropY:      transform.CropY,
			CropWidth:  transform.CropWidth,
			CropHeight: transform.CropHeight,
			Rotation:   transform.Rotation,
		}
	}

	// Get project color settings
	project, _ := h.queries.GetProject(ctx, photo.ProjectID)
	var colorParams *imaging.ColorParams
	if project != nil && (project.Brightness != 0 || project.Contrast != 0 || project.Saturation != 0) {
		colorParams = &imaging.ColorParams{
			Brightness: project.Brightness,
			Contrast:   project.Contrast,
			Saturation: project.Saturation,
		}
	}

	// Check for per-image overrides
	override, _ := h.queries.GetPhotoOverride(ctx, photo.ID)
	if override != nil {
		if override.Brightness.Valid || override.Contrast.Valid || override.Saturation.Valid {
			if colorParams == nil {
				colorParams = &imaging.ColorParams{}
			}
			if override.Brightness.Valid {
				colorParams.Brightness = override.Brightness.Float64
			}
			if override.Contrast.Valid {
				colorParams.Contrast = override.Contrast.Float64
			}
			if override.Saturation.Valid {
				colorParams.Saturation = override.Saturation.Float64
			}
		}
	}

	// Determine orientation for template overlays
	imgBounds := img.Bounds()
	orientationID := imaging.OrientationFromTransform(transformParams, imgBounds.Dx(), imgBounds.Dy())

	// Get overlays for this orientation
	var overlayParams []imaging.OverlayParams
	overlays, _ := h.queries.ListOverlaysByProjectOrientation(ctx, photo.ProjectID, orientationID)
	for _, o := range overlays {
		overlayParams = append(overlayParams, imaging.OverlayParams{
			Path:    h.store.Path(storage.BucketOverlays, o.StorageKey),
			X:       o.X,
			Y:       o.Y,
			Width:   o.Width,
			Height:  o.Height,
			Opacity: o.Opacity,
		})
	}

	// Get text overlays for this orientation
	var textParams []imaging.TextParams
	textOverlays, _ := h.queries.ListTextOverlaysByProjectOrientation(ctx, photo.ProjectID, orientationID)
	for _, t := range textOverlays {
		textParams = append(textParams, imaging.TextParams{
			Text:     t.Text,
			FontPath: t.FontFamily,
			FontSize: t.FontSize,
			Color:    t.Color,
			X:        t.X,
			Y:        t.Y,
			Opacity:  t.Opacity,
		})
	}

	// Render final image
	rendered, err := h.pipeline.Render(img, imaging.RenderOptions{
		Transform:    transformParams,
		Color:        colorParams,
		Overlays:     overlayParams,
		TextOverlays: textParams,
	})
	if err != nil {
		log.Printf("Error rendering photo %d: %v", photo.ID, err)
		h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusFailed)
		return
	}

	// Save rendered file
	renderedKey := fmt.Sprintf("%d.jpg", photo.ID)
	renderedPath := h.store.Path(storage.BucketRendered, renderedKey)

	f, err := createFile(renderedPath)
	if err != nil {
		log.Printf("Error creating rendered file: %v", err)
		h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusFailed)
		return
	}
	defer f.Close()

	if err := h.pipeline.EncodeJPEG(f, rendered); err != nil {
		log.Printf("Error encoding rendered image: %v", err)
		h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusFailed)
		return
	}

	// Update photo with rendered key
	h.queries.UpdatePhotoRendered(ctx, photo.ID, renderedKey)

	// Queue for printing — create N jobs based on copies
	h.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusQueued)

	// Re-read photo to get current copies value
	freshPhoto, _ := h.queries.GetPhoto(ctx, photo.ID)
	copies := 1
	if freshPhoto != nil && freshPhoto.Copies > 1 {
		copies = freshPhoto.Copies
	}

	for i := 0; i < copies; i++ {
		pos, _ := h.queries.GetNextQueuePosition(ctx)
		h.queries.CreatePrintJob(ctx, &db.PrintJob{
			PhotoID:  photo.ID,
			Position: pos,
			StatusID: db.PrintJobStatusQueued,
		})
	}

	log.Printf("Photo %d rendered and queued for printing (%d copies)", photo.ID, copies)
}

// RejectPhoto marks a photo as rejected.
func (h *Handlers) RejectPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	if err := h.queries.UpdatePhotoStatus(r.Context(), id, db.PhotoStatusRejected); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reject photo")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// OverridePhoto saves per-image template overrides.
func (h *Handlers) OverridePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	var req db.PhotoOverride
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.PhotoID = id

	if err := h.queries.UpsertPhotoOverride(r.Context(), &req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save override")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeletePhoto removes a photo and its files.
func (h *Handlers) DeletePhoto(w http.ResponseWriter, r *http.Request) {
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

	// Delete files
	h.store.Delete(storage.BucketOriginals, photo.OriginalKey)
	if photo.PreviewKey.Valid {
		h.store.Delete(storage.BucketPreviews, photo.PreviewKey.String)
	}
	if photo.RenderedKey.Valid {
		h.store.Delete(storage.BucketRendered, photo.RenderedKey.String)
	}

	// Delete DB record (cascades to transforms/overrides)
	if err := h.queries.DeletePhoto(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete photo")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListQueue returns the print queue.
func (h *Handlers) ListQueue(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.queries.ListPrintJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":   jobs,
		"paused": h.queue.IsPaused(),
	})
}

// PauseQueue pauses the print queue.
func (h *Handlers) PauseQueue(w http.ResponseWriter, r *http.Request) {
	h.queue.Pause()
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// ResumeQueue resumes the print queue.
func (h *Handlers) ResumeQueue(w http.ResponseWriter, r *http.Request) {
	h.queue.Resume()
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// RetryJob retries a failed print job.
func (h *Handlers) RetryJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.queries.GetPrintJob(r.Context(), id)
	if err != nil || job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	if job.StatusID != db.PrintJobStatusFailed {
		writeError(w, http.StatusBadRequest, "job is not in failed status")
		return
	}

	// Reset to queued
	if err := h.queries.UpdatePrintJobStatus(r.Context(), id, db.PrintJobStatusQueued, "", ""); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retry job")
		return
	}

	// Also reset photo status
	h.queries.UpdatePhotoStatus(r.Context(), job.PhotoID, db.PhotoStatusQueued)

	// Resume queue if paused
	if h.queue.IsPaused() {
		h.queue.Resume()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

// CancelJob cancels a queued or failed print job.
func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	// Get the job to find the photo
	job, _ := h.queries.GetPrintJob(r.Context(), id)

	if err := h.queries.UpdatePrintJobStatus(r.Context(), id, db.PrintJobStatusCanceled, "", ""); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel job")
		return
	}

	// If no other active jobs exist for this photo, revert photo status to uploaded
	if job != nil {
		h.revertPhotoStatusIfNoActiveJobs(r.Context(), job.PhotoID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
}

// revertPhotoStatusIfNoActiveJobs checks if a photo has any remaining active
// (queued/printing) jobs. If not, sets the photo to approved (not uploaded,
// so it doesn't re-appear in the review queue).
func (h *Handlers) revertPhotoStatusIfNoActiveJobs(ctx context.Context, photoID uint64) {
	count, err := h.queries.CountActiveJobsForPhoto(ctx, photoID)
	if err != nil || count > 0 {
		return
	}
	h.queries.UpdatePhotoStatus(ctx, photoID, db.PhotoStatusApproved)
}

// ListPrinters returns available CUPS printers.
func (h *Handlers) ListPrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := h.printer.ListPrinters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list printers")
		return
	}
	writeJSON(w, http.StatusOK, printers)
}

// TestPrint sends a test page to the printer.
func (h *Handlers) TestPrint(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "test print not yet implemented")
}

// GetSettings returns current system settings.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings := map[string]string{}
	for _, key := range []string{
		"printer_name", "printer_media",
		"hotspot_ssid", "hotspot_password", "gateway_ip", "server_port",
	} {
		val, _ := h.queries.GetSetting(r.Context(), key)
		settings[key] = val
	}
	writeJSON(w, http.StatusOK, settings)
}

// UpdateSettings updates system settings.
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for key, value := range req {
		if err := h.queries.SetSetting(r.Context(), key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update setting: "+key)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
