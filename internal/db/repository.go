package db

import "context"

// Repository defines the data access interface for the application.
// Both SQLite and MySQL implementations share this interface through sqlx.
type Repository struct {
	db *Queries
}

func NewRepository(q *Queries) *Repository {
	return &Repository{db: q}
}

// Settings
func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	return r.db.GetSetting(ctx, key)
}

func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	return r.db.SetSetting(ctx, key, value)
}

// Projects
func (r *Repository) CreateProject(ctx context.Context, p *Project) error {
	return r.db.CreateProject(ctx, p)
}

func (r *Repository) GetProject(ctx context.Context, id uint64) (*Project, error) {
	return r.db.GetProject(ctx, id)
}

func (r *Repository) ListProjects(ctx context.Context) ([]Project, error) {
	return r.db.ListProjects(ctx)
}

func (r *Repository) UpdateProject(ctx context.Context, p *Project) error {
	return r.db.UpdateProject(ctx, p)
}

func (r *Repository) DeleteProject(ctx context.Context, id uint64) error {
	return r.db.DeleteProject(ctx, id)
}

// Photos
func (r *Repository) CreatePhoto(ctx context.Context, p *Photo) error {
	return r.db.CreatePhoto(ctx, p)
}

func (r *Repository) GetPhoto(ctx context.Context, id uint64) (*Photo, error) {
	return r.db.GetPhoto(ctx, id)
}

func (r *Repository) ListPhotosByStatus(ctx context.Context, statusID uint) ([]Photo, error) {
	return r.db.ListPhotosByStatus(ctx, statusID)
}

func (r *Repository) ListPhotosBySession(ctx context.Context, sessionID string) ([]Photo, error) {
	return r.db.ListPhotosBySession(ctx, sessionID)
}

func (r *Repository) ListPhotosByProject(ctx context.Context, projectID uint64) ([]Photo, error) {
	return r.db.ListPhotosByProject(ctx, projectID)
}

func (r *Repository) UpdatePhotoStatus(ctx context.Context, id uint64, statusID uint) error {
	return r.db.UpdatePhotoStatus(ctx, id, statusID)
}

func (r *Repository) UpdatePhotoRendered(ctx context.Context, id uint64, renderedKey string) error {
	return r.db.UpdatePhotoRendered(ctx, id, renderedKey)
}

func (r *Repository) UpdatePhotoPreview(ctx context.Context, id uint64, previewKey string, width, height int, fileSize int64, mimeType string) error {
	return r.db.UpdatePhotoPreview(ctx, id, previewKey, width, height, fileSize, mimeType)
}

func (r *Repository) DeletePhoto(ctx context.Context, id uint64) error {
	return r.db.DeletePhoto(ctx, id)
}

// Photo Transforms
func (r *Repository) UpsertPhotoTransform(ctx context.Context, t *PhotoTransform) error {
	return r.db.UpsertPhotoTransform(ctx, t)
}

func (r *Repository) GetPhotoTransform(ctx context.Context, photoID uint64) (*PhotoTransform, error) {
	return r.db.GetPhotoTransform(ctx, photoID)
}

// Photo Overrides
func (r *Repository) UpsertPhotoOverride(ctx context.Context, o *PhotoOverride) error {
	return r.db.UpsertPhotoOverride(ctx, o)
}

func (r *Repository) GetPhotoOverride(ctx context.Context, photoID uint64) (*PhotoOverride, error) {
	return r.db.GetPhotoOverride(ctx, photoID)
}

// Print Jobs
func (r *Repository) CreatePrintJob(ctx context.Context, j *PrintJob) error {
	return r.db.CreatePrintJob(ctx, j)
}

func (r *Repository) GetPrintJob(ctx context.Context, id uint64) (*PrintJob, error) {
	return r.db.GetPrintJob(ctx, id)
}

func (r *Repository) GetNextQueuedJob(ctx context.Context) (*PrintJob, error) {
	return r.db.GetNextQueuedJob(ctx)
}

func (r *Repository) ListPrintJobs(ctx context.Context) ([]PrintJob, error) {
	return r.db.ListPrintJobs(ctx)
}

func (r *Repository) UpdatePrintJobStatus(ctx context.Context, id uint64, statusID uint, cupsJobID, errorMsg string) error {
	return r.db.UpdatePrintJobStatus(ctx, id, statusID, cupsJobID, errorMsg)
}

func (r *Repository) GetNextQueuePosition(ctx context.Context) (int, error) {
	return r.db.GetNextQueuePosition(ctx)
}

// Overlays
func (r *Repository) CreateOverlay(ctx context.Context, o *Overlay) error {
	return r.db.CreateOverlay(ctx, o)
}

func (r *Repository) ListOverlaysByProject(ctx context.Context, projectID uint64) ([]Overlay, error) {
	return r.db.ListOverlaysByProject(ctx, projectID)
}

func (r *Repository) UpdateOverlay(ctx context.Context, o *Overlay) error {
	return r.db.UpdateOverlay(ctx, o)
}

func (r *Repository) DeleteOverlay(ctx context.Context, id uint64) error {
	return r.db.DeleteOverlay(ctx, id)
}

// Text Overlays
func (r *Repository) CreateTextOverlay(ctx context.Context, t *TextOverlay) error {
	return r.db.CreateTextOverlay(ctx, t)
}

func (r *Repository) ListTextOverlaysByProject(ctx context.Context, projectID uint64) ([]TextOverlay, error) {
	return r.db.ListTextOverlaysByProject(ctx, projectID)
}

func (r *Repository) UpdateTextOverlay(ctx context.Context, t *TextOverlay) error {
	return r.db.UpdateTextOverlay(ctx, t)
}

func (r *Repository) DeleteTextOverlay(ctx context.Context, id uint64) error {
	return r.db.DeleteTextOverlay(ctx, id)
}

// Admin Sessions
func (r *Repository) CreateAdminSession(ctx context.Context, s *AdminSession) error {
	return r.db.CreateAdminSession(ctx, s)
}

func (r *Repository) GetAdminSessionByToken(ctx context.Context, token string) (*AdminSession, error) {
	return r.db.GetAdminSessionByToken(ctx, token)
}

func (r *Repository) DeleteAdminSession(ctx context.Context, token string) error {
	return r.db.DeleteAdminSession(ctx, token)
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context) error {
	return r.db.DeleteExpiredSessions(ctx)
}
