package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Queries provides all database query methods. Uses sqlx named queries
// that work across both SQLite and MySQL.
type Queries struct {
	db *sqlx.DB
}

func NewQueries(db *sqlx.DB) *Queries {
	return &Queries{db: db}
}

// ExecDirect runs a raw SQL statement — only meant for maintenance
// operations that don't fit the query-per-method pattern (e.g. SQLite's
// `VACUUM INTO` during backup). Prefer a typed method for everything else.
func (q *Queries) ExecDirect(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return q.db.ExecContext(ctx, query, args...)
}

// Settings

func (q *Queries) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := q.db.GetContext(ctx, &value, "SELECT value FROM settings WHERE `key` = ?", key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (q *Queries) SetSetting(ctx context.Context, key, value string) error {
	// Use INSERT OR REPLACE for SQLite, INSERT ... ON DUPLICATE KEY UPDATE for MySQL.
	// Since both drivers use ?, we handle this with a driver-agnostic approach:
	// try UPDATE first, then INSERT if no rows affected.
	res, err := q.db.ExecContext(ctx, "UPDATE settings SET value = ? WHERE `key` = ?", value, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = q.db.ExecContext(ctx, "INSERT INTO settings (`key`, value) VALUES (?, ?)", key, value)
	}
	return err
}

// Projects

func (q *Queries) CreateProject(ctx context.Context, p *Project) error {
	if p.VisibilityID == 0 {
		p.VisibilityID = VisibilityPublic
	}
	if p.ProjectTypeID == 0 {
		p.ProjectTypeID = ProjectTypeStandard
	}
	res, err := q.db.ExecContext(ctx,
		"INSERT INTO projects (name, brightness, contrast, saturation, visibility_id, project_type_id, booth_countdown, slug) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		p.Name, p.Brightness, p.Contrast, p.Saturation, p.VisibilityID, p.ProjectTypeID, p.BoothCountdown, p.Slug,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = uint64(id)
	return nil
}

func (q *Queries) GetProject(ctx context.Context, id uint64) (*Project, error) {
	var p Project
	err := q.db.GetContext(ctx, &p, "SELECT * FROM projects WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (q *Queries) ListProjects(ctx context.Context) ([]Project, error) {
	projects := make([]Project, 0)
	err := q.db.SelectContext(ctx, &projects, "SELECT * FROM projects ORDER BY created_at DESC")
	return projects, err
}

func (q *Queries) UpdateProject(ctx context.Context, p *Project) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE projects SET name = ?, brightness = ?, contrast = ?, saturation = ?, visibility_id = ?, project_type_id = ?, booth_countdown = ?, slug = ?, updated_at = ? WHERE id = ?",
		p.Name, p.Brightness, p.Contrast, p.Saturation, p.VisibilityID, p.ProjectTypeID, p.BoothCountdown, p.Slug, dbTime(time.Now()), p.ID,
	)
	return err
}

// GetProjectBySlug finds a project by its unique slug (for hidden project access).
func (q *Queries) GetProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	var p Project
	err := q.db.GetContext(ctx, &p, "SELECT * FROM projects WHERE slug = ?", slug)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

// ListPublicProjects returns only public projects.
func (q *Queries) ListPublicProjects(ctx context.Context) ([]Project, error) {
	projects := make([]Project, 0)
	err := q.db.SelectContext(ctx, &projects,
		"SELECT * FROM projects WHERE visibility_id = ? ORDER BY created_at DESC",
		VisibilityPublic)
	return projects, err
}

func (q *Queries) DeleteProject(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	return err
}

// Photos

func (q *Queries) CreatePhoto(ctx context.Context, p *Photo) error {
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO photos (project_id, session_id, original_key, status_id, content_hash)
		 VALUES (?, ?, ?, ?, ?)`,
		p.ProjectID, p.SessionID, p.OriginalKey, p.StatusID, p.ContentHash,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = uint64(id)
	return nil
}

func (q *Queries) GetPhoto(ctx context.Context, id uint64) (*Photo, error) {
	var p Photo
	err := q.db.GetContext(ctx, &p, "SELECT * FROM photos WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (q *Queries) ListPhotosByStatus(ctx context.Context, statusID uint) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		"SELECT * FROM photos WHERE status_id = ? ORDER BY created_at ASC", statusID)
	return photos, err
}

func (q *Queries) ListPhotosBySession(ctx context.Context, sessionID string) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		"SELECT * FROM photos WHERE session_id = ? ORDER BY created_at DESC", sessionID)
	return photos, err
}

func (q *Queries) ListPhotosByProject(ctx context.Context, projectID uint64) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		"SELECT * FROM photos WHERE project_id = ? ORDER BY created_at ASC", projectID)
	return photos, err
}

// FindPhotoByContentHash returns the earliest photo in a project whose bytes
// hash to the given value, or nil when the project has no copy of it yet.
// Scoped to the project because the same photo in two different projects is
// two different prints, not a duplicate.
func (q *Queries) FindPhotoByContentHash(ctx context.Context, projectID uint64, hash string) (*Photo, error) {
	if hash == "" {
		return nil, nil
	}
	var p Photo
	err := q.db.GetContext(ctx, &p,
		`SELECT * FROM photos
		 WHERE project_id = ? AND content_hash = ?
		 ORDER BY id ASC LIMIT 1`, projectID, hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (q *Queries) GetPhotosByIDs(ctx context.Context, ids []uint64) ([]Photo, error) {
	if len(ids) == 0 {
		return make([]Photo, 0), nil
	}
	query, args, err := sqlx.In("SELECT * FROM photos WHERE id IN (?)", ids)
	if err != nil {
		return nil, err
	}
	query = q.db.Rebind(query)
	photos := make([]Photo, 0, len(ids))
	err = q.db.SelectContext(ctx, &photos, query, args...)
	return photos, err
}

// UpdatePhotoCaptureMetadata records the capture info read from a file, along
// with which of its timestamps the date came from. Passing a zero takenAt
// stores NULL, which makes consumers fall back to created_at.
func (q *Queries) UpdatePhotoCaptureMetadata(ctx context.Context, id uint64, takenAt time.Time, source, cameraMake, cameraModel string) error {
	if takenAt.IsZero() {
		source = ""
	}
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET taken_at = ?, taken_at_source = ?, camera_make = ?, camera_model = ? WHERE id = ?",
		dbWallTime(takenAt), nullableStr(source), nullableStr(cameraMake), nullableStr(cameraModel), id)
	return err
}

// nullableStr maps "" to a SQL NULL so absent EXIF fields aren't stored as
// empty strings that later read as "present but blank".
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// dbTimeLayout is the one format timestamps are written in. It is what SQLite's
// CURRENT_TIMESTAMP produces for created_at, so the columns sort against each
// other, and MySQL accepts it for DATETIME.
//
// Binding a time.Time directly is what must be avoided: the SQLite driver
// stores it in Go's String() layout, which includes the zone *name*, and can
// only read a handful of layouts back. A numerically named zone — exactly what
// an EXIF offset yields, FixedZone("-04:00", …) — writes a value the driver
// then refuses to scan, and one such row makes every SELECT * over that table
// fail.
const dbTimeLayout = "2006-01-02 15:04:05"

// dbTime binds an instant, normalized to UTC. A zero time becomes SQL NULL.
func dbTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(dbTimeLayout)
}

// dbWallTime binds a clock reading whose zone carries no meaning of its own.
// An EXIF capture time is what the camera's clock showed, so it is stored as
// written rather than shifted into UTC — otherwise a date overlay would print
// an hour the photographer never saw. A zero time becomes SQL NULL.
func dbWallTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(dbTimeLayout)
}

func (q *Queries) UpdatePhotoStatus(ctx context.Context, id uint64, statusID uint) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET status_id = ?, updated_at = ? WHERE id = ?",
		statusID, dbTime(time.Now()), id)
	return err
}

func (q *Queries) UpdatePhotoOriginalKey(ctx context.Context, id uint64, originalKey string) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET original_key = ?, updated_at = ? WHERE id = ?",
		originalKey, dbTime(time.Now()), id)
	return err
}

func (q *Queries) UpdatePhotoRendered(ctx context.Context, id uint64, renderedKey string) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET rendered_key = ?, updated_at = ? WHERE id = ?",
		renderedKey, dbTime(time.Now()), id)
	return err
}

func (q *Queries) ClearPhotoRendered(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET rendered_key = NULL, updated_at = ? WHERE id = ?",
		dbTime(time.Now()), id)
	return err
}

func (q *Queries) ClearProjectRenderedPhotos(ctx context.Context, projectID uint64) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE photos SET rendered_key = NULL, updated_at = ?
		 WHERE project_id = ? AND status_id != ? AND rendered_key IS NOT NULL`,
		dbTime(time.Now()), projectID, PhotoStatusPrinted)
	return err
}

func (q *Queries) UpdatePhotoCopies(ctx context.Context, id uint64, copies int) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE photos SET copies = ?, updated_at = ? WHERE id = ?",
		copies, dbTime(time.Now()), id)
	return err
}

// ListRenderedPhotosByProject returns photos that have a rendered_key set and are not printed.
// Used for file cleanup when invalidating project renders.
func (q *Queries) ListRenderedPhotosByProject(ctx context.Context, projectID uint64) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		`SELECT * FROM photos WHERE project_id = ? AND status_id != ? AND rendered_key IS NOT NULL`,
		projectID, PhotoStatusPrinted)
	return photos, err
}

func (q *Queries) UpdatePhotoPreview(ctx context.Context, id uint64, previewKey string, width, height int, fileSize int64, mimeType string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE photos SET preview_key = ?, original_width = ?, original_height = ?,
		 file_size = ?, mime_type = ?, updated_at = ? WHERE id = ?`,
		previewKey, width, height, fileSize, mimeType, dbTime(time.Now()), id)
	return err
}

func (q *Queries) DeletePhoto(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM photos WHERE id = ?", id)
	return err
}

// Photo Transforms

func (q *Queries) UpsertPhotoTransform(ctx context.Context, t *PhotoTransform) error {
	// Try update first
	res, err := q.db.ExecContext(ctx,
		`UPDATE photo_transforms SET crop_x = ?, crop_y = ?, crop_width = ?, crop_height = ?, rotation = ?
		 WHERE photo_id = ?`,
		t.CropX, t.CropY, t.CropWidth, t.CropHeight, t.Rotation, t.PhotoID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		insertRes, err := q.db.ExecContext(ctx,
			`INSERT INTO photo_transforms (photo_id, crop_x, crop_y, crop_width, crop_height, rotation)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			t.PhotoID, t.CropX, t.CropY, t.CropWidth, t.CropHeight, t.Rotation,
		)
		if err != nil {
			return err
		}
		id, err := insertRes.LastInsertId()
		if err != nil {
			return err
		}
		t.ID = uint64(id)
	}
	return nil
}

func (q *Queries) GetPhotoTransform(ctx context.Context, photoID uint64) (*PhotoTransform, error) {
	var t PhotoTransform
	err := q.db.GetContext(ctx, &t, "SELECT * FROM photo_transforms WHERE photo_id = ?", photoID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

// Photo Overrides

func (q *Queries) UpsertPhotoOverride(ctx context.Context, o *PhotoOverride) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE photo_overrides SET brightness = ?, contrast = ?, saturation = ?,
		 overlay_overrides = ?, text_overrides = ? WHERE photo_id = ?`,
		o.Brightness, o.Contrast, o.Saturation,
		o.OverlayOverrides, o.TextOverrides, o.PhotoID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		insertRes, err := q.db.ExecContext(ctx,
			`INSERT INTO photo_overrides (photo_id, brightness, contrast, saturation, overlay_overrides, text_overrides)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			o.PhotoID, o.Brightness, o.Contrast, o.Saturation,
			o.OverlayOverrides, o.TextOverrides,
		)
		if err != nil {
			return err
		}
		id, err := insertRes.LastInsertId()
		if err != nil {
			return err
		}
		o.ID = uint64(id)
	}
	return nil
}

func (q *Queries) GetPhotoOverride(ctx context.Context, photoID uint64) (*PhotoOverride, error) {
	var o PhotoOverride
	err := q.db.GetContext(ctx, &o, "SELECT * FROM photo_overrides WHERE photo_id = ?", photoID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

// Print Jobs

func (q *Queries) CreatePrintJob(ctx context.Context, j *PrintJob) error {
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO print_jobs (photo_id, position, status_id)
		 VALUES (?, ?, ?)`,
		j.PhotoID, j.Position, j.StatusID,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	j.ID = uint64(id)
	return nil
}

func (q *Queries) GetPrintJob(ctx context.Context, id uint64) (*PrintJob, error) {
	var j PrintJob
	err := q.db.GetContext(ctx, &j, "SELECT * FROM print_jobs WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

func (q *Queries) GetNextQueuedJob(ctx context.Context) (*PrintJob, error) {
	var j PrintJob
	err := q.db.GetContext(ctx, &j,
		"SELECT * FROM print_jobs WHERE status_id = ? ORDER BY position ASC LIMIT 1",
		PrintJobStatusQueued)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

func (q *Queries) ListPrintJobs(ctx context.Context) ([]PrintJob, error) {
	jobs := make([]PrintJob, 0)
	err := q.db.SelectContext(ctx, &jobs,
		"SELECT * FROM print_jobs WHERE status_id NOT IN (?, ?) ORDER BY position ASC",
		PrintJobStatusCanceled, PrintJobStatusPrinted)
	return jobs, err
}

func (q *Queries) UpdatePrintJobStatus(ctx context.Context, id uint64, statusID uint, cupsJobID, errorMsg string) error {
	query := "UPDATE print_jobs SET status_id = ?, updated_at = ?"
	args := []any{statusID, dbTime(time.Now())}

	if cupsJobID != "" {
		query += ", cups_job_id = ?"
		args = append(args, cupsJobID)
	}
	if errorMsg != "" {
		query += ", error_msg = ?"
		args = append(args, errorMsg)
	}
	if statusID == PrintJobStatusPrinted {
		query += ", printed_at = ?"
		args = append(args, dbTime(time.Now()))
	}
	if statusID == PrintJobStatusPrinting {
		query += ", attempts = attempts + 1"
	}

	query += " WHERE id = ?"
	args = append(args, id)

	_, err := q.db.ExecContext(ctx, query, args...)
	return err
}

func (q *Queries) DeletePrintJobsByPhoto(ctx context.Context, photoID uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM print_jobs WHERE photo_id = ?", photoID)
	return err
}

func (q *Queries) CountActiveJobsForPhoto(ctx context.Context, photoID uint64) (int, error) {
	var count int
	err := q.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM print_jobs WHERE photo_id = ? AND status_id IN (?, ?)",
		photoID, PrintJobStatusQueued, PrintJobStatusPrinting)
	return count, err
}

func (q *Queries) GetNextQueuePosition(ctx context.Context) (int, error) {
	var pos sql.NullInt64
	err := q.db.GetContext(ctx, &pos, "SELECT MAX(position) FROM print_jobs")
	if err != nil {
		return 1, err
	}
	if !pos.Valid {
		return 1, nil
	}
	return int(pos.Int64) + 1, nil
}

// Overlays

func (q *Queries) CreateOverlay(ctx context.Context, o *Overlay) error {
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO overlays (project_id, filename, storage_key, x, y, width, height, opacity, z_order, orientation_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ProjectID, o.Filename, o.StorageKey, o.X, o.Y, o.Width, o.Height, o.Opacity, o.ZOrder, o.OrientationID,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	o.ID = uint64(id)
	return nil
}

func (q *Queries) GetOverlay(ctx context.Context, id uint64) (*Overlay, error) {
	var o Overlay
	err := q.db.GetContext(ctx, &o, "SELECT * FROM overlays WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (q *Queries) ListOverlaysByProject(ctx context.Context, projectID uint64) ([]Overlay, error) {
	overlays := make([]Overlay, 0)
	err := q.db.SelectContext(ctx, &overlays,
		"SELECT * FROM overlays WHERE project_id = ? ORDER BY z_order ASC", projectID)
	return overlays, err
}

func (q *Queries) ListOverlaysByProjectOrientation(ctx context.Context, projectID uint64, orientationID uint) ([]Overlay, error) {
	overlays := make([]Overlay, 0)
	err := q.db.SelectContext(ctx, &overlays,
		"SELECT * FROM overlays WHERE project_id = ? AND orientation_id = ? ORDER BY z_order ASC",
		projectID, orientationID)
	return overlays, err
}

func (q *Queries) UpdateOverlay(ctx context.Context, o *Overlay) error {
	if o.StorageKey != "" {
		_, err := q.db.ExecContext(ctx,
			`UPDATE overlays SET storage_key = ?, x = ?, y = ?, width = ?, height = ?, opacity = ?, z_order = ?
			 WHERE id = ?`,
			o.StorageKey, o.X, o.Y, o.Width, o.Height, o.Opacity, o.ZOrder, o.ID)
		return err
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE overlays SET x = ?, y = ?, width = ?, height = ?, opacity = ?, z_order = ?
		 WHERE id = ?`,
		o.X, o.Y, o.Width, o.Height, o.Opacity, o.ZOrder, o.ID)
	return err
}

func (q *Queries) DeleteOverlay(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM overlays WHERE id = ?", id)
	return err
}

// Text Overlays

func (q *Queries) CreateTextOverlay(ctx context.Context, t *TextOverlay) error {
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO text_overlays (project_id, text, font_family, font_size, color, x, y, opacity, z_order, orientation_id, source, date_format, text_align)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ProjectID, t.Text, t.FontFamily, t.FontSize, t.Color, t.X, t.Y, t.Opacity, t.ZOrder, t.OrientationID,
		t.SourceOrDefault(), t.DateFormat, t.AlignOrDefault(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = uint64(id)
	return nil
}

func (q *Queries) GetTextOverlay(ctx context.Context, id uint64) (*TextOverlay, error) {
	var t TextOverlay
	err := q.db.GetContext(ctx, &t, "SELECT * FROM text_overlays WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

func (q *Queries) ListTextOverlaysByProject(ctx context.Context, projectID uint64) ([]TextOverlay, error) {
	overlays := make([]TextOverlay, 0)
	err := q.db.SelectContext(ctx, &overlays,
		"SELECT * FROM text_overlays WHERE project_id = ? ORDER BY z_order ASC", projectID)
	return overlays, err
}

func (q *Queries) ListTextOverlaysByProjectOrientation(ctx context.Context, projectID uint64, orientationID uint) ([]TextOverlay, error) {
	overlays := make([]TextOverlay, 0)
	err := q.db.SelectContext(ctx, &overlays,
		"SELECT * FROM text_overlays WHERE project_id = ? AND orientation_id = ? ORDER BY z_order ASC",
		projectID, orientationID)
	return overlays, err
}

func (q *Queries) UpdateTextOverlay(ctx context.Context, t *TextOverlay) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE text_overlays SET text = ?, font_family = ?, font_size = ?, color = ?,
		 x = ?, y = ?, opacity = ?, z_order = ?, source = ?, date_format = ?, text_align = ? WHERE id = ?`,
		t.Text, t.FontFamily, t.FontSize, t.Color, t.X, t.Y, t.Opacity, t.ZOrder,
		t.SourceOrDefault(), t.DateFormat, t.AlignOrDefault(), t.ID)
	return err
}

func (q *Queries) DeleteTextOverlay(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM text_overlays WHERE id = ?", id)
	return err
}

// Admin Sessions

func (q *Queries) CreateAdminSession(ctx context.Context, s *AdminSession) error {
	res, err := q.db.ExecContext(ctx,
		"INSERT INTO admin_sessions (token, expires_at) VALUES (?, ?)",
		s.Token, dbTime(s.ExpiresAt),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = uint64(id)
	return nil
}

func (q *Queries) GetAdminSessionByToken(ctx context.Context, token string) (*AdminSession, error) {
	var s AdminSession
	err := q.db.GetContext(ctx, &s,
		"SELECT * FROM admin_sessions WHERE token = ? AND expires_at > ?",
		token, dbTime(time.Now()))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (q *Queries) DeleteAdminSession(ctx context.Context, token string) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token = ?", token)
	return err
}

func (q *Queries) DeleteExpiredSessions(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE expires_at <= ?", dbTime(time.Now()))
	return err
}

// Fonts

func (q *Queries) CreateFont(ctx context.Context, f *Font) error {
	res, err := q.db.ExecContext(ctx,
		"INSERT INTO fonts (name, filename, storage_key) VALUES (?, ?, ?)",
		f.Name, f.Filename, f.StorageKey,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	f.ID = uint64(id)
	return nil
}

func (q *Queries) ListFonts(ctx context.Context) ([]Font, error) {
	fonts := make([]Font, 0)
	err := q.db.SelectContext(ctx, &fonts, "SELECT * FROM fonts ORDER BY name ASC")
	return fonts, err
}

func (q *Queries) GetFont(ctx context.Context, id uint64) (*Font, error) {
	var f Font
	err := q.db.GetContext(ctx, &f, "SELECT * FROM fonts WHERE id = ?", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &f, err
}

func (q *Queries) UpdateFontStorageKey(ctx context.Context, id uint64, storageKey string) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE fonts SET storage_key = ? WHERE id = ?", storageKey, id)
	return err
}

func (q *Queries) DeleteFont(ctx context.Context, id uint64) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM fonts WHERE id = ?", id)
	return err
}

// Printer Assignments

// InsertPrinterIfNotExists adds a printer as disabled if it doesn't already exist.
// Does NOT change the enabled state of existing printers.
func (q *Queries) InsertPrinterIfNotExists(ctx context.Context, name string) error {
	// Check if it already exists
	var count int
	err := q.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM printer_assignments WHERE name = ?", name)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // already exists, don't change
	}
	_, err = q.db.ExecContext(ctx,
		"INSERT INTO printer_assignments (name, enabled) VALUES (?, 0)", name)
	return err
}

func (q *Queries) UpsertPrinterAssignment(ctx context.Context, name string, enabled bool) error {
	res, err := q.db.ExecContext(ctx,
		"UPDATE printer_assignments SET enabled = ? WHERE name = ?", enabled, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, err = q.db.ExecContext(ctx,
			"INSERT INTO printer_assignments (name, enabled) VALUES (?, ?)", name, enabled)
	}
	return err
}

func (q *Queries) ListPrinterAssignments(ctx context.Context) ([]PrinterAssignment, error) {
	printers := make([]PrinterAssignment, 0)
	err := q.db.SelectContext(ctx, &printers,
		"SELECT * FROM printer_assignments ORDER BY name ASC")
	return printers, err
}

func (q *Queries) GetEnabledPrinters(ctx context.Context) ([]PrinterAssignment, error) {
	printers := make([]PrinterAssignment, 0)
	err := q.db.SelectContext(ctx, &printers,
		"SELECT * FROM printer_assignments WHERE enabled = 1 ORDER BY name ASC")
	return printers, err
}

func (q *Queries) DeletePrinterAssignment(ctx context.Context, name string) error {
	_, err := q.db.ExecContext(ctx, "DELETE FROM printer_assignments WHERE name = ?", name)
	return err
}

// UpdatePrintJobPrinter sets which printer a job is assigned to.
func (q *Queries) UpdatePrintJobPrinter(ctx context.Context, id uint64, printerName string) error {
	_, err := q.db.ExecContext(ctx,
		"UPDATE print_jobs SET printer_name = ? WHERE id = ?", printerName, id)
	return err
}

// GetNextQueuedJobForPrinter gets the next queued job not yet assigned, or assigned to this printer.
func (q *Queries) GetNextQueuedJobForPrinter(ctx context.Context, printerName string) (*PrintJob, error) {
	var j PrintJob
	err := q.db.GetContext(ctx, &j,
		`SELECT * FROM print_jobs
		 WHERE status_id = ? AND (printer_name IS NULL OR printer_name = ?)
		 ORDER BY position ASC LIMIT 1`,
		PrintJobStatusQueued, printerName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

// Gallery: list all photos for a project with status names
// ListGalleryPhotos returns a single project's gallery photos. The caller is
// responsible for checking that the project may be shown to the requester —
// see ListPublicGalleryPhotos for the unscoped, visibility-filtered variant.
func (q *Queries) ListGalleryPhotos(ctx context.Context, projectID uint64) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		"SELECT * FROM photos WHERE project_id = ? AND status_id != ? ORDER BY created_at DESC",
		projectID, PhotoStatusRejected)
	return photos, err
}

// ListPublicGalleryPhotos returns gallery photos from public projects only.
//
// This backs the unscoped guest gallery, so the visibility join is the only
// thing keeping hidden (link/QR-only) and private projects out of it. It must
// stay a join rather than a filter applied after the fact — an earlier version
// selected every photo and leaked both.
func (q *Queries) ListPublicGalleryPhotos(ctx context.Context) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		`SELECT photos.* FROM photos
		 JOIN projects ON projects.id = photos.project_id
		 WHERE projects.visibility_id = ? AND photos.status_id != ?
		 ORDER BY photos.created_at DESC`,
		VisibilityPublic, PhotoStatusRejected)
	return photos, err
}

func (q *Queries) ListAllPhotos(ctx context.Context) ([]Photo, error) {
	photos := make([]Photo, 0)
	err := q.db.SelectContext(ctx, &photos,
		"SELECT * FROM photos ORDER BY created_at ASC")
	return photos, err
}

// CountPhotosByStatus returns a map of status_id -> count.
func (q *Queries) CountPhotosByStatus(ctx context.Context) (map[uint]int, error) {
	type row struct {
		StatusID uint `db:"status_id"`
		Count    int  `db:"cnt"`
	}
	var rows []row
	err := q.db.SelectContext(ctx, &rows,
		"SELECT status_id, COUNT(*) as cnt FROM photos GROUP BY status_id")
	if err != nil {
		return nil, fmt.Errorf("counting photos by status: %w", err)
	}
	counts := make(map[uint]int)
	for _, r := range rows {
		counts[r.StatusID] = r.Count
	}
	return counts, nil
}
