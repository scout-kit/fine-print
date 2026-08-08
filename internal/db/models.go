package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Photo status IDs (match photo_statuses lookup table)
const (
	PhotoStatusUploaded uint = 1
	PhotoStatusApproved uint = 2
	PhotoStatusQueued   uint = 3
	PhotoStatusPrinting uint = 4
	PhotoStatusPrinted  uint = 5
	PhotoStatusFailed   uint = 6
	PhotoStatusRejected uint = 7
)

// Print job status IDs (match print_job_statuses lookup table)
const (
	PrintJobStatusQueued   uint = 1
	PrintJobStatusPrinting uint = 2
	PrintJobStatusPrinted  uint = 3
	PrintJobStatusFailed   uint = 4
	PrintJobStatusCanceled uint = 5
)

type Setting struct {
	ID    uint64 `db:"id"`
	Key   string `db:"key"`
	Value string `db:"value"`
}

// Project visibility IDs (match project_visibilities lookup table)
const (
	VisibilityPublic  uint = 1
	VisibilityHidden  uint = 2
	VisibilityPrivate uint = 3
)

// Project type IDs (match project_types lookup table)
const (
	ProjectTypeStandard uint = 1
	ProjectTypeBooth    uint = 2
)

func VisibilityName(id uint) string {
	switch id {
	case VisibilityPublic:
		return "public"
	case VisibilityHidden:
		return "hidden"
	case VisibilityPrivate:
		return "private"
	default:
		return "unknown"
	}
}

type Project struct {
	ID             uint64         `db:"id" json:"id"`
	Name           string         `db:"name" json:"name"`
	CreatedAt      time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updated_at"`
	Brightness     float64        `db:"brightness" json:"brightness"`
	Contrast       float64        `db:"contrast" json:"contrast"`
	Saturation     float64        `db:"saturation" json:"saturation"`
	VisibilityID   uint           `db:"visibility_id" json:"visibility_id"`
	ProjectTypeID  uint           `db:"project_type_id" json:"project_type_id"`
	BoothCountdown int            `db:"booth_countdown" json:"booth_countdown"`
	Slug           sql.NullString `db:"slug" json:"-"`
}

// SlugValue returns the slug as a plain string pointer for JSON serialization.
func (p Project) SlugValue() *string {
	if p.Slug.Valid {
		return &p.Slug.String
	}
	return nil
}

// MarshalJSON customizes JSON output to flatten NullString fields.
func (p Project) MarshalJSON() ([]byte, error) {
	type Alias Project
	return json.Marshal(&struct {
		Alias
		Slug *string `json:"slug"`
	}{
		Alias: Alias(p),
		Slug:  p.SlugValue(),
	})
}

// Overlay orientation IDs
const (
	OrientationLandscape uint = 1
	OrientationPortrait  uint = 2
)

type Overlay struct {
	ID            uint64    `db:"id" json:"id"`
	ProjectID     uint64    `db:"project_id" json:"project_id"`
	Filename      string    `db:"filename" json:"filename"`
	StorageKey    string    `db:"storage_key" json:"storage_key"`
	X             float64   `db:"x" json:"x"`
	Y             float64   `db:"y" json:"y"`
	Width         float64   `db:"width" json:"width"`
	Height        float64   `db:"height" json:"height"`
	Opacity       float64   `db:"opacity" json:"opacity"`
	ZOrder        int       `db:"z_order" json:"z_order"`
	OrientationID uint      `db:"orientation_id" json:"orientation_id"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// Text overlay content sources. Mirrors imaging.TextSource; kept as plain
// strings here so the db package doesn't depend on imaging.
const (
	TextSourceStatic        = "static"
	TextSourcePhotoDate     = "photo_date"
	TextSourcePhotoDateTime = "photo_datetime"
)

// Which edge of the text stays pinned to the overlay's x. Mirrors
// imaging.TextAlign.
const (
	TextAlignLeft   = "left"
	TextAlignCenter = "center"
	TextAlignRight  = "right"
)

type TextOverlay struct {
	ID            uint64  `db:"id" json:"id"`
	ProjectID     uint64  `db:"project_id" json:"project_id"`
	Text          string  `db:"text" json:"text"`
	FontFamily    string  `db:"font_family" json:"font_family"`
	FontSize      float64 `db:"font_size" json:"font_size"`
	Color         string  `db:"color" json:"color"`
	X             float64 `db:"x" json:"x"`
	Y             float64 `db:"y" json:"y"`
	Opacity       float64 `db:"opacity" json:"opacity"`
	ZOrder        int     `db:"z_order" json:"z_order"`
	OrientationID uint    `db:"orientation_id" json:"orientation_id"`
	// Source is "static" (print Text verbatim) or a photo-derived source
	// such as "photo_date" / "photo_datetime".
	Source string `db:"source" json:"source"`
	// DateFormat names a preset from the imaging package. Only meaningful
	// for date sources; NULL/empty falls back to that source's default.
	DateFormat sql.NullString `db:"date_format" json:"-"`
	// TextAlign picks which edge of the rendered text sits at X. Matters most
	// for date overlays, whose width changes with the date being printed.
	TextAlign string    `db:"text_align" json:"text_align"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// AlignOrDefault returns the anchor edge, treating a blank column (rows
// written before the text_align column existed) as left.
func (t TextOverlay) AlignOrDefault() string {
	if t.TextAlign == "" {
		return TextAlignLeft
	}
	return t.TextAlign
}

// SourceOrDefault returns the content source, treating a blank column (rows
// written before the source column existed) as static text.
func (t TextOverlay) SourceOrDefault() string {
	if t.Source == "" {
		return TextSourceStatic
	}
	return t.Source
}

// IsDateSource reports whether this overlay's content is derived from the
// photo's timestamp rather than its stored text.
func (t TextOverlay) IsDateSource() bool {
	s := t.SourceOrDefault()
	return s == TextSourcePhotoDate || s == TextSourcePhotoDateTime
}

// MarshalJSON flattens date_format so the frontend sees a plain string.
func (t TextOverlay) MarshalJSON() ([]byte, error) {
	type Alias TextOverlay
	return json.Marshal(&struct {
		Alias
		Source     string `json:"source"`
		DateFormat string `json:"date_format"`
		TextAlign  string `json:"text_align"`
	}{
		Alias:      Alias(t),
		Source:     t.SourceOrDefault(),
		DateFormat: t.DateFormat.String,
		TextAlign:  t.AlignOrDefault(),
	})
}

type Photo struct {
	ID             uint64         `db:"id" json:"-"`
	ProjectID      uint64         `db:"project_id" json:"-"`
	SessionID      string         `db:"session_id" json:"-"`
	OriginalKey    string         `db:"original_key" json:"-"`
	PreviewKey     sql.NullString `db:"preview_key" json:"-"`
	RenderedKey    sql.NullString `db:"rendered_key" json:"-"`
	OriginalWidth  sql.NullInt64  `db:"original_width" json:"-"`
	OriginalHeight sql.NullInt64  `db:"original_height" json:"-"`
	FileSize       sql.NullInt64  `db:"file_size" json:"-"`
	MimeType       sql.NullString `db:"mime_type" json:"-"`
	StatusID       uint           `db:"status_id" json:"-"`
	Copies         int            `db:"copies" json:"-"`
	// TakenAt is the EXIF capture time. NULL when the file carried no usable
	// timestamp — consumers fall back to CreatedAt (the upload time).
	TakenAt     sql.NullTime   `db:"taken_at" json:"-"`
	CameraMake  sql.NullString `db:"camera_make" json:"-"`
	CameraModel sql.NullString `db:"camera_model" json:"-"`
	CreatedAt   time.Time      `db:"created_at" json:"-"`
	UpdatedAt   time.Time      `db:"updated_at" json:"-"`
}

// EffectiveTakenAt returns the timestamp to print and display, along with
// where it came from. EXIF wins; otherwise the upload time stands in, and the
// "upload" source is reported so the UI can say so rather than implying the
// camera recorded it.
func (p Photo) EffectiveTakenAt() (time.Time, string) {
	if p.TakenAt.Valid && !p.TakenAt.Time.IsZero() {
		return p.TakenAt.Time, "exif"
	}
	return p.CreatedAt, "upload"
}

// CameraLabel joins make and model into one display string, avoiding the
// common "Canon Canon EOS R6" duplication when the model repeats the make.
func (p Photo) CameraLabel() string {
	make_, model := strings.TrimSpace(p.CameraMake.String), strings.TrimSpace(p.CameraModel.String)
	switch {
	case make_ == "" && model == "":
		return ""
	case make_ == "":
		return model
	case model == "":
		return make_
	case strings.HasPrefix(strings.ToLower(model), strings.ToLower(make_)):
		return model
	default:
		return make_ + " " + model
	}
}

func nullStr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func nullInt(ni sql.NullInt64) *int64 {
	if ni.Valid {
		return &ni.Int64
	}
	return nil
}

func (p Photo) MarshalJSON() ([]byte, error) {
	takenAt, takenAtSource := p.EffectiveTakenAt()
	return json.Marshal(struct {
		ID             uint64  `json:"id"`
		ProjectID      uint64  `json:"project_id"`
		SessionID      string  `json:"session_id"`
		OriginalKey    string  `json:"original_key"`
		PreviewKey     *string `json:"preview_key"`
		RenderedKey    *string `json:"rendered_key"`
		OriginalWidth  *int64  `json:"original_width"`
		OriginalHeight *int64  `json:"original_height"`
		FileSize       *int64  `json:"file_size"`
		MimeType       *string `json:"mime_type"`
		StatusID       uint    `json:"status_id"`
		Copies         int     `json:"copies"`
		CreatedAt      string  `json:"created_at"`
		UpdatedAt      string  `json:"updated_at"`
		// Capture metadata. TakenAt is always populated — it falls back to
		// the upload time — and TakenAtSource says which it is.
		TakenAt       string  `json:"taken_at"`
		TakenAtSource string  `json:"taken_at_source"`
		TakenAtEXIF   *string `json:"taken_at_exif"`
		CameraMake    *string `json:"camera_make"`
		CameraModel   *string `json:"camera_model"`
		CameraLabel   string  `json:"camera_label"`
	}{
		ID:             p.ID,
		ProjectID:      p.ProjectID,
		SessionID:      p.SessionID,
		OriginalKey:    p.OriginalKey,
		PreviewKey:     nullStr(p.PreviewKey),
		RenderedKey:    nullStr(p.RenderedKey),
		OriginalWidth:  nullInt(p.OriginalWidth),
		OriginalHeight: nullInt(p.OriginalHeight),
		FileSize:       nullInt(p.FileSize),
		MimeType:       nullStr(p.MimeType),
		StatusID:       p.StatusID,
		Copies:         max(p.Copies, 1),
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      p.UpdatedAt.Format(time.RFC3339),
		TakenAt:        takenAt.Format(time.RFC3339),
		TakenAtSource:  takenAtSource,
		TakenAtEXIF:    nullTimeRFC3339(p.TakenAt),
		CameraMake:     nullStr(p.CameraMake),
		CameraModel:    nullStr(p.CameraModel),
		CameraLabel:    p.CameraLabel(),
	})
}

// nullTimeRFC3339 renders a nullable timestamp for JSON, nil when absent.
func nullTimeRFC3339(nt sql.NullTime) *string {
	if !nt.Valid || nt.Time.IsZero() {
		return nil
	}
	s := nt.Time.Format(time.RFC3339)
	return &s
}

type PhotoTransform struct {
	ID         uint64  `db:"id" json:"id"`
	PhotoID    uint64  `db:"photo_id" json:"photo_id"`
	CropX      float64 `db:"crop_x" json:"crop_x"`
	CropY      float64 `db:"crop_y" json:"crop_y"`
	CropWidth  float64 `db:"crop_width" json:"crop_width"`
	CropHeight float64 `db:"crop_height" json:"crop_height"`
	Rotation   float64 `db:"rotation" json:"rotation"`
}

type PhotoOverride struct {
	ID               uint64          `db:"id" json:"id"`
	PhotoID          uint64          `db:"photo_id" json:"photo_id"`
	Brightness       sql.NullFloat64 `db:"brightness" json:"brightness"`
	Contrast         sql.NullFloat64 `db:"contrast" json:"contrast"`
	Saturation       sql.NullFloat64 `db:"saturation" json:"saturation"`
	OverlayOverrides sql.NullString  `db:"overlay_overrides" json:"overlay_overrides"`
	TextOverrides    sql.NullString  `db:"text_overrides" json:"text_overrides"`
}

type PrintJob struct {
	ID          uint64         `db:"id" json:"-"`
	PhotoID     uint64         `db:"photo_id" json:"-"`
	CUPSJobID   sql.NullString `db:"cups_job_id" json:"-"`
	Position    int            `db:"position" json:"-"`
	StatusID    uint           `db:"status_id" json:"-"`
	ErrorMsg    sql.NullString `db:"error_msg" json:"-"`
	Attempts    int            `db:"attempts" json:"-"`
	PrinterName sql.NullString `db:"printer_name" json:"-"`
	CreatedAt   time.Time      `db:"created_at" json:"-"`
	UpdatedAt   time.Time      `db:"updated_at" json:"-"`
	PrintedAt   sql.NullTime   `db:"printed_at" json:"-"`
}

func (j PrintJob) MarshalJSON() ([]byte, error) {
	var printedAt *string
	if j.PrintedAt.Valid {
		s := j.PrintedAt.Time.Format(time.RFC3339)
		printedAt = &s
	}
	return json.Marshal(struct {
		ID          uint64  `json:"id"`
		PhotoID     uint64  `json:"photo_id"`
		CUPSJobID   *string `json:"cups_job_id"`
		Position    int     `json:"position"`
		StatusID    uint    `json:"status_id"`
		ErrorMsg    *string `json:"error_msg"`
		Attempts    int     `json:"attempts"`
		PrinterName *string `json:"printer_name"`
		CreatedAt   string  `json:"created_at"`
		UpdatedAt   string  `json:"updated_at"`
		PrintedAt   *string `json:"printed_at"`
	}{
		ID:          j.ID,
		PhotoID:     j.PhotoID,
		CUPSJobID:   nullStr(j.CUPSJobID),
		Position:    j.Position,
		StatusID:    j.StatusID,
		ErrorMsg:    nullStr(j.ErrorMsg),
		Attempts:    j.Attempts,
		PrinterName: nullStr(j.PrinterName),
		CreatedAt:   j.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   j.UpdatedAt.Format(time.RFC3339),
		PrintedAt:   printedAt,
	})
}

type Font struct {
	ID         uint64    `db:"id" json:"id"`
	Name       string    `db:"name" json:"name"`
	Filename   string    `db:"filename" json:"filename"`
	StorageKey string    `db:"storage_key" json:"storage_key"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

type PrinterAssignment struct {
	ID        uint64    `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type AdminSession struct {
	ID        uint64    `db:"id"`
	Token     string    `db:"token"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

// PhotoStatusName returns the human-readable name for a photo status ID.
func PhotoStatusName(id uint) string {
	switch id {
	case PhotoStatusUploaded:
		return "uploaded"
	case PhotoStatusApproved:
		return "approved"
	case PhotoStatusQueued:
		return "queued"
	case PhotoStatusPrinting:
		return "printing"
	case PhotoStatusPrinted:
		return "printed"
	case PhotoStatusFailed:
		return "failed"
	case PhotoStatusRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// PrintJobStatusName returns the human-readable name for a print job status ID.
func PrintJobStatusName(id uint) string {
	switch id {
	case PrintJobStatusQueued:
		return "queued"
	case PrintJobStatusPrinting:
		return "printing"
	case PrintJobStatusPrinted:
		return "printed"
	case PrintJobStatusFailed:
		return "failed"
	case PrintJobStatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}
