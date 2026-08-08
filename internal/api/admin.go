package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/settings"
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

	// Check password against stored hash. When no hash is present the
	// first-run wizard hasn't run yet — block login and signal the
	// frontend to redirect to /setup.
	storedHash, _ := h.queries.GetSetting(r.Context(), settings.KeyAdminPasswordHash)
	if storedHash == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":         "setup_required",
			"redirect":      "/setup",
		})
		return
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

// UnapprovePhoto resets a photo back to uploaded status so it can be re-reviewed.
func (h *Handlers) UnapprovePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	if err := h.queries.UpdatePhotoStatus(r.Context(), id, db.PhotoStatusUploaded); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unapprove photo")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded"})
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

	// Delete print jobs (no cascade on FK)
	h.queries.DeletePrintJobsByPhoto(r.Context(), id)

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
// (queued/printing) jobs. If not, reverts to uploaded so it can be re-approved.
func (h *Handlers) revertPhotoStatusIfNoActiveJobs(ctx context.Context, photoID uint64) {
	count, err := h.queries.CountActiveJobsForPhoto(ctx, photoID)
	if err != nil || count > 0 {
		return
	}
	h.queries.UpdatePhotoStatus(ctx, photoID, db.PhotoStatusUploaded)
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

// SettingField describes one tunable setting in the API response.
type SettingField struct {
	Key             string `json:"key"`
	Value           string `json:"value"`
	RequiresRestart bool   `json:"requires_restart"`
}

// GetSettings returns all tunable settings as a typed list.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	fields := make([]SettingField, 0, len(settings.TunableKeys))
	for _, key := range settings.TunableKeys {
		val, _ := h.queries.GetSetting(r.Context(), key)
		fields = append(fields, SettingField{
			Key:             key,
			Value:           val,
			RequiresRestart: settings.RequiresRestart(key),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fields": fields,
	})
}

// UpdateSettings patches one or more tunable settings. Unknown keys are
// rejected so typos don't silently write garbage into the settings table.
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed := make(map[string]bool, len(settings.TunableKeys))
	for _, k := range settings.TunableKeys {
		allowed[k] = true
	}

	anyRequiresRestart := false
	changed := make([]string, 0, len(req))
	for key, value := range req {
		if !allowed[key] {
			writeError(w, http.StatusBadRequest, "unknown setting: "+key)
			return
		}
		if err := validateSettingValue(key, value); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.Set(r.Context(), key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update setting: "+key)
			return
		}
		changed = append(changed, key)
		if settings.RequiresRestart(key) {
			anyRequiresRestart = true
		}
	}

	h.broadcastAdmin("settings_changed", map[string]any{"keys": changed})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"changed":          changed,
		"requires_restart": anyRequiresRestart,
	})
}

// ChangeAdminPassword updates the admin password hash. Requires the caller
// to supply the current password to prevent an open session from being
// hijacked into a permanent lockout.
func (h *Handlers) ChangeAdminPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.New) < 4 {
		writeError(w, http.StatusBadRequest, "new password must be at least 4 characters")
		return
	}

	currentHash, _ := h.queries.GetSetting(r.Context(), settings.KeyAdminPasswordHash)
	if currentHash == "" {
		writeError(w, http.StatusInternalServerError, "admin password not initialized")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.Current)); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.queries.SetSetting(r.Context(), settings.KeyAdminPasswordHash, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RestartService exits the process cleanly. When running under systemd
// (Restart=always) or launchd (KeepAlive) the service supervisor respawns
// it, which is how "requires restart" settings take effect.
func (h *Handlers) RestartService(w http.ResponseWriter, r *http.Request) {
	h.broadcastAdmin("restarting", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
	// Give the response a beat to flush before exiting.
	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Println("Restart requested via admin API — exiting for supervisor respawn")
		os.Exit(0)
	}()
}

// validateSettingValue enforces per-key value constraints before writing to the DB.
func validateSettingValue(key, value string) error {
	switch key {
	case settings.KeyHotspotEnabled, settings.KeyDNSEnabled, settings.KeyPrinterAutoQueue:
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be 'true' or 'false'", key)
		}
	case settings.KeyDNSPort,
		settings.KeyImagingMaxUpload,
		settings.KeyImagingPreviewWidth,
		settings.KeyImagingPrintWidth,
		settings.KeyImagingPrintHeight:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
	case settings.KeyImagingJPEGQuality:
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100 {
			return fmt.Errorf("%s must be between 1 and 100", key)
		}
	case settings.KeyPrinterMedia:
		if value != "4x6" && value != "Postcard" {
			return fmt.Errorf("%s must be '4x6' or 'Postcard'", key)
		}
	}
	return nil
}
