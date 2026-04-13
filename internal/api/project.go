package api

import (
	"database/sql"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"net/http"
	"path/filepath"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// CreateProject creates a new project.
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		VisibilityID   uint   `json:"visibility_id"`
		ProjectTypeID  uint   `json:"project_type_id"`
		BoothCountdown int    `json:"booth_countdown"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.VisibilityID == 0 {
		req.VisibilityID = db.VisibilityPublic
	}
	if req.ProjectTypeID == 0 {
		req.ProjectTypeID = db.ProjectTypeStandard
	}

	project := &db.Project{
		Name:           req.Name,
		VisibilityID:   req.VisibilityID,
		ProjectTypeID:  req.ProjectTypeID,
		BoothCountdown: req.BoothCountdown,
	}

	// Generate slug for hidden projects (and always set one for shareable links)
	slug := generateToken(12)
	project.Slug = sql.NullString{String: slug, Valid: true}

	if err := h.queries.CreateProject(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, project)
}

// GetProject returns a single project.
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
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

	overlays, _ := h.queries.ListOverlaysByProject(r.Context(), id)
	textOverlays, _ := h.queries.ListTextOverlaysByProject(r.Context(), id)

	writeJSON(w, http.StatusOK, map[string]any{
		"project":       project,
		"overlays":      overlays,
		"text_overlays": textOverlays,
	})
}

// ListProjects returns all projects.
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.queries.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// UpdateProject updates a project's settings.
func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req struct {
		Name           string  `json:"name"`
		Brightness     float64 `json:"brightness"`
		Contrast       float64 `json:"contrast"`
		Saturation     float64 `json:"saturation"`
		VisibilityID   uint    `json:"visibility_id"`
		ProjectTypeID  uint    `json:"project_type_id"`
		BoothCountdown int     `json:"booth_countdown"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Fetch existing to preserve slug
	existing, _ := h.queries.GetProject(r.Context(), id)

	if req.ProjectTypeID == 0 {
		req.ProjectTypeID = db.ProjectTypeStandard
	}

	project := &db.Project{
		ID:             id,
		Name:           req.Name,
		Brightness:     req.Brightness,
		Contrast:       req.Contrast,
		Saturation:     req.Saturation,
		VisibilityID:   req.VisibilityID,
		ProjectTypeID:  req.ProjectTypeID,
		BoothCountdown: req.BoothCountdown,
	}

	// Preserve existing slug, or generate one if needed
	if existing != nil {
		project.Slug = existing.Slug
	}
	if !project.Slug.Valid {
		project.Slug = sql.NullString{String: generateToken(12), Valid: true}
	}
	if project.VisibilityID == 0 {
		project.VisibilityID = db.VisibilityPublic
	}

	if err := h.queries.UpdateProject(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	// Invalidate renders for non-printed photos when template changes
	h.invalidateProjectRenders(r.Context(), id)

	writeJSON(w, http.StatusOK, project)
}

// DeleteProject removes a project and all associated data.
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	if err := h.queries.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// UploadOverlay uploads an overlay PNG for a project.
func (h *Handlers) UploadOverlay(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10MB limit for overlays
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("overlay")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing overlay field")
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if ext != ".png" {
		writeError(w, http.StatusBadRequest, "overlay must be a PNG file")
		return
	}

	// Read orientation from form field (default landscape)
	orientationID := db.OrientationLandscape
	if r.FormValue("orientation_id") == "2" {
		orientationID = db.OrientationPortrait
	}

	// Read image dimensions to calculate aspect-correct default size
	imgCfg, _, imgErr := image.DecodeConfig(file)
	file.Seek(0, 0) // reset reader after decoding config

	defaultW := 0.3
	defaultH := 0.3
	if imgErr == nil && imgCfg.Width > 0 && imgCfg.Height > 0 {
		// Set width to 30% of canvas, calculate height to maintain aspect ratio
		// Account for canvas aspect ratio (3:2 landscape or 2:3 portrait)
		canvasAspect := 3.0 / 2.0
		if orientationID == db.OrientationPortrait {
			canvasAspect = 2.0 / 3.0
		}
		imgAspect := float64(imgCfg.Width) / float64(imgCfg.Height)
		defaultH = (defaultW / imgAspect) * canvasAspect
	}

	overlay := &db.Overlay{
		ProjectID:     id,
		Filename:      header.Filename,
		StorageKey:    "",
		X:             0.0,
		Y:             0.0,
		Width:         defaultW,
		Height:        defaultH,
		Opacity:       0.8,
		OrientationID: orientationID,
	}

	if err := h.queries.CreateOverlay(r.Context(), overlay); err != nil {
		log.Printf("Error creating overlay record: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create overlay")
		return
	}

	storageKey := fmt.Sprintf("%d.png", overlay.ID)
	if err := h.store.Save(storage.BucketOverlays, storageKey, file); err != nil {
		log.Printf("Error saving overlay file: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save overlay")
		return
	}

	overlay.StorageKey = storageKey
	h.queries.UpdateOverlay(r.Context(), overlay)

	h.invalidateProjectRenders(r.Context(), id)
	writeJSON(w, http.StatusCreated, overlay)
}

// UpdateOverlayPosition updates an overlay's position, size, and opacity.
// Fetches existing record and merges provided fields.
func (h *Handlers) UpdateOverlayPosition(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid overlay id")
		return
	}

	var req struct {
		X       *float64 `json:"x"`
		Y       *float64 `json:"y"`
		Width   *float64 `json:"width"`
		Height  *float64 `json:"height"`
		Opacity *float64 `json:"opacity"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := h.queries.GetOverlay(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "overlay not found")
		return
	}

	if req.X != nil {
		existing.X = *req.X
	}
	if req.Y != nil {
		existing.Y = *req.Y
	}
	if req.Width != nil {
		existing.Width = *req.Width
	}
	if req.Height != nil {
		existing.Height = *req.Height
	}
	if req.Opacity != nil {
		existing.Opacity = *req.Opacity
	}

	if err := h.queries.UpdateOverlay(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update overlay")
		return
	}

	h.invalidateProjectRenders(r.Context(), existing.ProjectID)
	writeJSON(w, http.StatusOK, existing)
}

// DeleteOverlay removes an overlay.
func (h *Handlers) DeleteOverlay(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid overlay id")
		return
	}

	// Get project ID before deleting for invalidation
	overlay, _ := h.queries.GetOverlay(r.Context(), id)

	storageKey := fmt.Sprintf("%d.png", id)
	h.store.Delete(storage.BucketOverlays, storageKey)

	if err := h.queries.DeleteOverlay(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete overlay")
		return
	}

	if overlay != nil {
		h.invalidateProjectRenders(r.Context(), overlay.ProjectID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ServeOverlay serves an overlay image file.
func (h *Handlers) ServeOverlay(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid overlay id")
		return
	}

	overlay, err := h.queries.GetOverlay(r.Context(), id)
	if err != nil || overlay == nil || overlay.StorageKey == "" {
		writeError(w, http.StatusNotFound, "overlay not found")
		return
	}

	filePath := h.store.Path(storage.BucketOverlays, overlay.StorageKey)
	http.ServeFile(w, r, filePath)
}

// CreateTextOverlay adds a text overlay to a project.
func (h *Handlers) CreateTextOverlay(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req struct {
		Text          string  `json:"text"`
		FontFamily    string  `json:"font_family"`
		FontSize      float64 `json:"font_size"`
		Color         string  `json:"color"`
		X             float64 `json:"x"`
		Y             float64 `json:"y"`
		Opacity       float64 `json:"opacity"`
		OrientationID uint    `json:"orientation_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.FontSize == 0 {
		req.FontSize = 25
	}
	if req.Color == "" {
		req.Color = "#FFFFFF"
	}
	if req.Opacity == 0 {
		req.Opacity = 1.0
	}

	orientID := req.OrientationID
	if orientID == 0 {
		orientID = db.OrientationLandscape
	}

	t := &db.TextOverlay{
		ProjectID:     projectID,
		Text:          req.Text,
		FontFamily:    req.FontFamily,
		FontSize:      req.FontSize,
		Color:         req.Color,
		X:             req.X,
		Y:             req.Y,
		Opacity:       req.Opacity,
		OrientationID: orientID,
	}

	if err := h.queries.CreateTextOverlay(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create text overlay")
		return
	}

	h.invalidateProjectRenders(r.Context(), projectID)
	writeJSON(w, http.StatusCreated, t)
}

// UpdateTextOverlayHandler updates a text overlay with partial data.
// Any provided field overwrites the existing value; omitted fields are preserved.
func (h *Handlers) UpdateTextOverlayHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid text overlay id")
		return
	}

	var req struct {
		Text       *string  `json:"text"`
		FontFamily *string  `json:"font_family"`
		FontSize   *float64 `json:"font_size"`
		Color      *string  `json:"color"`
		X          *float64 `json:"x"`
		Y          *float64 `json:"y"`
		Opacity    *float64 `json:"opacity"`
		ZOrder     *int     `json:"z_order"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Fetch existing
	existing, err := h.queries.GetTextOverlay(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "text overlay not found")
		return
	}

	// Merge
	if req.Text != nil {
		existing.Text = *req.Text
	}
	if req.FontFamily != nil {
		existing.FontFamily = *req.FontFamily
	}
	if req.FontSize != nil {
		existing.FontSize = *req.FontSize
	}
	if req.Color != nil {
		existing.Color = *req.Color
	}
	if req.X != nil {
		existing.X = *req.X
	}
	if req.Y != nil {
		existing.Y = *req.Y
	}
	if req.Opacity != nil {
		existing.Opacity = *req.Opacity
	}
	if req.ZOrder != nil {
		existing.ZOrder = *req.ZOrder
	}

	if err := h.queries.UpdateTextOverlay(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update text overlay")
		return
	}

	h.invalidateProjectRenders(r.Context(), existing.ProjectID)
	writeJSON(w, http.StatusOK, existing)
}

// DeleteTextOverlayHandler removes a text overlay.
func (h *Handlers) DeleteTextOverlayHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid text overlay id")
		return
	}

	// Get project ID before deleting for invalidation
	textOverlay, _ := h.queries.GetTextOverlay(r.Context(), id)

	if err := h.queries.DeleteTextOverlay(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete text overlay")
		return
	}

	if textOverlay != nil {
		h.invalidateProjectRenders(r.Context(), textOverlay.ProjectID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// CopyTemplateOrientation copies all overlays and text overlays from one orientation to another.
func (h *Handlers) CopyTemplateOrientation(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req struct {
		From uint `json:"from"` // source orientation_id
		To   uint `json:"to"`   // target orientation_id
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if (req.From != db.OrientationLandscape && req.From != db.OrientationPortrait) ||
		(req.To != db.OrientationLandscape && req.To != db.OrientationPortrait) ||
		req.From == req.To {
		writeError(w, http.StatusBadRequest, "invalid orientation")
		return
	}

	ctx := r.Context()

	// Swap axes when copying between orientations:
	// Landscape X→Portrait Y, Landscape Y→Portrait X, W↔H

	// Copy image overlays
	overlays, _ := h.queries.ListOverlaysByProjectOrientation(ctx, projectID, req.From)
	for _, o := range overlays {
		h.queries.CreateOverlay(ctx, &db.Overlay{
			ProjectID:     projectID,
			Filename:      o.Filename,
			StorageKey:    o.StorageKey,
			X:             o.Y,
			Y:             o.X,
			Width:         o.Height,
			Height:        o.Width,
			Opacity:       o.Opacity,
			ZOrder:        o.ZOrder,
			OrientationID: req.To,
		})
	}

	// Copy text overlays
	textOverlays, _ := h.queries.ListTextOverlaysByProjectOrientation(ctx, projectID, req.From)
	for _, t := range textOverlays {
		h.queries.CreateTextOverlay(ctx, &db.TextOverlay{
			ProjectID:     projectID,
			Text:          t.Text,
			FontFamily:    t.FontFamily,
			FontSize:      t.FontSize,
			Color:         t.Color,
			X:             t.Y,
			Y:             t.X,
			Opacity:       t.Opacity,
			ZOrder:        t.ZOrder,
			OrientationID: req.To,
		})
	}

	h.invalidateProjectRenders(ctx, projectID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "copied",
		"count":  fmt.Sprintf("%d overlays, %d text", len(overlays), len(textOverlays)),
	})
}

// CopyProject duplicates a project with its settings and overlays.
func (h *Handlers) CopyProject(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	var req struct {
		Name         string `json:"name"`
		VisibilityID uint   `json:"visibility_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.VisibilityID == 0 {
		req.VisibilityID = db.VisibilityPublic
	}

	ctx := r.Context()

	source, err := h.queries.GetProject(ctx, sourceID)
	if err != nil || source == nil {
		writeError(w, http.StatusNotFound, "source project not found")
		return
	}

	// Create new project with source settings
	newProject := &db.Project{
		Name:           req.Name,
		Brightness:     source.Brightness,
		Contrast:       source.Contrast,
		Saturation:     source.Saturation,
		VisibilityID:   req.VisibilityID,
		ProjectTypeID:  source.ProjectTypeID,
		BoothCountdown: source.BoothCountdown,
		Slug:           sql.NullString{String: generateToken(12), Valid: true},
	}

	if err := h.queries.CreateProject(ctx, newProject); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	// Copy all overlays (both orientations)
	allOverlays, _ := h.queries.ListOverlaysByProject(ctx, sourceID)
	for _, o := range allOverlays {
		h.queries.CreateOverlay(ctx, &db.Overlay{
			ProjectID:     newProject.ID,
			Filename:      o.Filename,
			StorageKey:    o.StorageKey, // share the same file
			X:             o.X,
			Y:             o.Y,
			Width:         o.Width,
			Height:        o.Height,
			Opacity:       o.Opacity,
			ZOrder:        o.ZOrder,
			OrientationID: o.OrientationID,
		})
	}

	// Copy all text overlays (both orientations)
	allText, _ := h.queries.ListTextOverlaysByProject(ctx, sourceID)
	for _, t := range allText {
		h.queries.CreateTextOverlay(ctx, &db.TextOverlay{
			ProjectID:     newProject.ID,
			Text:          t.Text,
			FontFamily:    t.FontFamily,
			FontSize:      t.FontSize,
			Color:         t.Color,
			X:             t.X,
			Y:             t.Y,
			Opacity:       t.Opacity,
			ZOrder:        t.ZOrder,
			OrientationID: t.OrientationID,
		})
	}

	writeJSON(w, http.StatusCreated, newProject)
}
