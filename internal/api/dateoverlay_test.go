package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
)

// doJSONWithID is doJSON plus a ServeMux path value, which handlers read via
// parseID. httptest.NewRequest alone doesn't populate path values.
func doJSONWithID(t *testing.T, h http.HandlerFunc, method, path string, id uint64, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encoding body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatUint(id, 10))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func itoa(v uint64) string { return strconv.FormatUint(v, 10) }

// newProject inserts a project to hang overlays off.
func newProject(t *testing.T, q *db.Queries) *db.Project {
	t.Helper()
	p := &db.Project{Name: "Test Event", VisibilityID: db.VisibilityPublic, ProjectTypeID: db.ProjectTypeStandard}
	if err := q.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("creating project: %v", err)
	}
	return p
}

// A date overlay must survive the round trip through SQLite: source and
// date_format are new columns, so a scanning mistake would silently degrade
// every date overlay back to static text.
func TestTextOverlay_DateSourceRoundTrip(t *testing.T) {
	_, q := newTestHandlers(t)
	ctx := context.Background()
	project := newProject(t, q)

	want := &db.TextOverlay{
		ProjectID:     project.ID,
		Text:          "",
		FontFamily:    "/fonts/Inter.ttf",
		FontSize:      32,
		Color:         "#FF0000",
		X:             0.25,
		Y:             0.75,
		Opacity:       0.9,
		OrientationID: db.OrientationLandscape,
		Source:        db.TextSourcePhotoDateTime,
		DateFormat:    sql.NullString{String: string(imaging.DateTimeFormatISO), Valid: true},
	}
	if err := q.CreateTextOverlay(ctx, want); err != nil {
		t.Fatalf("creating text overlay: %v", err)
	}

	got, err := q.GetTextOverlay(ctx, want.ID)
	if err != nil || got == nil {
		t.Fatalf("reading back overlay: %v", err)
	}
	if got.Source != db.TextSourcePhotoDateTime {
		t.Errorf("source = %q, want %q", got.Source, db.TextSourcePhotoDateTime)
	}
	if !got.DateFormat.Valid || got.DateFormat.String != string(imaging.DateTimeFormatISO) {
		t.Errorf("date_format = %+v, want %q", got.DateFormat, imaging.DateTimeFormatISO)
	}
	if !got.IsDateSource() {
		t.Error("IsDateSource() = false, want true")
	}

	// And an update must not lose them.
	got.FontSize = 40
	if err := q.UpdateTextOverlay(ctx, got); err != nil {
		t.Fatalf("updating overlay: %v", err)
	}
	after, _ := q.GetTextOverlay(ctx, want.ID)
	if after.Source != db.TextSourcePhotoDateTime || after.DateFormat.String != string(imaging.DateTimeFormatISO) {
		t.Errorf("after update: source=%q format=%q, want preserved", after.Source, after.DateFormat.String)
	}
}

// Rows written before the migration have source = 'static' from the column
// default, so existing static overlays must keep printing their text.
func TestTextOverlay_StaticRoundTripUnchanged(t *testing.T) {
	_, q := newTestHandlers(t)
	ctx := context.Background()
	project := newProject(t, q)

	in := &db.TextOverlay{
		ProjectID: project.ID, Text: "Smile!", FontSize: 25, Color: "#FFFFFF",
		Opacity: 1, OrientationID: db.OrientationLandscape,
	}
	if err := q.CreateTextOverlay(ctx, in); err != nil {
		t.Fatalf("creating overlay: %v", err)
	}

	got, err := q.GetTextOverlay(ctx, in.ID)
	if err != nil || got == nil {
		t.Fatalf("reading back overlay: %v", err)
	}
	if got.SourceOrDefault() != db.TextSourceStatic {
		t.Errorf("source = %q, want static", got.SourceOrDefault())
	}
	if got.IsDateSource() {
		t.Error("a static overlay must not report as a date source")
	}
	if got.DateFormat.Valid {
		t.Errorf("static overlay stored a date_format: %q", got.DateFormat.String)
	}
	// The renderer must still print its literal text.
	content := imaging.ResolveTextContent(
		got.Text, imaging.TextSource(got.SourceOrDefault()),
		imaging.DateFormat(got.DateFormat.String), time.Now(),
	)
	if content != "Smile!" {
		t.Errorf("resolved content = %q, want %q", content, "Smile!")
	}
}

// The create endpoint must accept a date overlay with no text, and reject a
// static one without any.
func TestCreateTextOverlay_HTTPValidation(t *testing.T) {
	h, q := newTestHandlers(t)
	project := newProject(t, q)
	path := "/api/admin/projects/" + itoa(project.ID) + "/text-overlay"

	// Date source, no text — allowed.
	rec := doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"source":      db.TextSourcePhotoDate,
		"date_format": string(imaging.DateFormatISO),
		"font_size":   24,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("date overlay: status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Source     string `json:"source"`
		DateFormat string `json:"date_format"`
		Text       string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if created.Source != db.TextSourcePhotoDate {
		t.Errorf("source = %q, want %q", created.Source, db.TextSourcePhotoDate)
	}
	if created.DateFormat != string(imaging.DateFormatISO) {
		t.Errorf("date_format = %q, want %q", created.DateFormat, imaging.DateFormatISO)
	}

	// Static source with no text — rejected, as before this feature.
	rec = doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"source": db.TextSourceStatic,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty static overlay: status = %d, want 400", rec.Code)
	}

	// Mismatched format — rejected rather than silently coerced.
	rec = doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"source":      db.TextSourcePhotoDate,
		"date_format": string(imaging.DateTimeFormatISO),
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("date source with datetime format: status = %d, want 400", rec.Code)
	}
}

// EffectiveTakenAt is what decides the printed date, so both branches matter.
func TestPhoto_EffectiveTakenAt(t *testing.T) {
	uploaded := time.Date(2026, time.March, 14, 18, 0, 0, 0, time.UTC)
	captured := time.Date(2026, time.March, 14, 14, 31, 0, 0, time.UTC)

	withEXIF := db.Photo{CreatedAt: uploaded, TakenAt: sql.NullTime{Time: captured, Valid: true}}
	at, source := withEXIF.EffectiveTakenAt()
	if !at.Equal(captured) || source != "exif" {
		t.Errorf("with EXIF: got %s/%s, want %s/exif", at, source, captured)
	}

	withoutEXIF := db.Photo{CreatedAt: uploaded}
	at, source = withoutEXIF.EffectiveTakenAt()
	if !at.Equal(uploaded) || source != "upload" {
		t.Errorf("without EXIF: got %s/%s, want %s/upload", at, source, uploaded)
	}

	// A NULL-but-zero timestamp must not be mistaken for a real capture time.
	zeroEXIF := db.Photo{CreatedAt: uploaded, TakenAt: sql.NullTime{Valid: true}}
	at, source = zeroEXIF.EffectiveTakenAt()
	if !at.Equal(uploaded) || source != "upload" {
		t.Errorf("zero EXIF time: got %s/%s, want %s/upload", at, source, uploaded)
	}
}

// Camera make and model are usually concatenated for display, but many
// cameras repeat the make inside the model.
func TestPhoto_CameraLabel(t *testing.T) {
	tests := []struct {
		make, model, want string
	}{
		{"Canon", "Canon EOS R6", "Canon EOS R6"},
		{"NIKON CORPORATION", "NIKON Z 6", "NIKON CORPORATION NIKON Z 6"},
		{"Apple", "iPhone 15 Pro", "Apple iPhone 15 Pro"},
		{"", "iPhone 15 Pro", "iPhone 15 Pro"},
		{"Apple", "", "Apple"},
		{"", "", ""},
	}
	for _, tc := range tests {
		p := db.Photo{
			CameraMake:  sql.NullString{String: tc.make, Valid: tc.make != ""},
			CameraModel: sql.NullString{String: tc.model, Valid: tc.model != ""},
		}
		if got := p.CameraLabel(); got != tc.want {
			t.Errorf("CameraLabel(%q, %q) = %q, want %q", tc.make, tc.model, got, tc.want)
		}
	}
}

// The metadata a photo carries must round-trip through the DB, since the
// columns are new and scanned via SELECT *.
func TestUpdatePhotoCaptureMetadata_RoundTrip(t *testing.T) {
	_, q := newTestHandlers(t)
	ctx := context.Background()
	project := newProject(t, q)

	photo := &db.Photo{ProjectID: project.ID, SessionID: "sess", StatusID: db.PhotoStatusUploaded}
	if err := q.CreatePhoto(ctx, photo); err != nil {
		t.Fatalf("creating photo: %v", err)
	}

	captured := time.Date(2026, time.March, 14, 14, 31, 7, 0, time.UTC)
	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, captured, "Canon", "Canon EOS R6"); err != nil {
		t.Fatalf("storing metadata: %v", err)
	}

	got, err := q.GetPhoto(ctx, photo.ID)
	if err != nil || got == nil {
		t.Fatalf("reading photo: %v", err)
	}
	if !got.TakenAt.Valid {
		t.Fatal("taken_at was not persisted")
	}
	if !got.TakenAt.Time.UTC().Equal(captured) {
		t.Errorf("taken_at = %s, want %s", got.TakenAt.Time.UTC(), captured)
	}
	if got.CameraLabel() != "Canon EOS R6" {
		t.Errorf("camera label = %q, want %q", got.CameraLabel(), "Canon EOS R6")
	}
	at, source := got.EffectiveTakenAt()
	if source != "exif" || !at.UTC().Equal(captured) {
		t.Errorf("effective = %s/%s, want %s/exif", at.UTC(), source, captured)
	}
}

// Absent EXIF must store NULL, not an empty string that later reads as
// "present but blank".
func TestUpdatePhotoCaptureMetadata_ZeroTimeStoresNull(t *testing.T) {
	_, q := newTestHandlers(t)
	ctx := context.Background()
	project := newProject(t, q)

	photo := &db.Photo{ProjectID: project.ID, SessionID: "sess", StatusID: db.PhotoStatusUploaded}
	if err := q.CreatePhoto(ctx, photo); err != nil {
		t.Fatalf("creating photo: %v", err)
	}
	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, time.Time{}, "", ""); err != nil {
		t.Fatalf("storing metadata: %v", err)
	}

	got, _ := q.GetPhoto(ctx, photo.ID)
	if got.TakenAt.Valid {
		t.Errorf("taken_at should be NULL, got %s", got.TakenAt.Time)
	}
	if got.CameraMake.Valid || got.CameraModel.Valid {
		t.Errorf("camera fields should be NULL, got %+v / %+v", got.CameraMake, got.CameraModel)
	}
	if _, source := got.EffectiveTakenAt(); source != "upload" {
		t.Errorf("source = %q, want upload", source)
	}
}

// The serialized photo must always expose a usable taken_at plus its source,
// because three separate UI surfaces depend on those fields.
func TestPhotoJSON_ExposesCaptureMetadata(t *testing.T) {
	uploaded := time.Date(2026, time.March, 14, 18, 0, 0, 0, time.UTC)
	captured := time.Date(2026, time.March, 14, 14, 31, 0, 0, time.UTC)

	var withEXIF struct {
		TakenAt       string  `json:"taken_at"`
		TakenAtSource string  `json:"taken_at_source"`
		TakenAtEXIF   *string `json:"taken_at_exif"`
		CameraLabel   string  `json:"camera_label"`
	}
	raw, err := json.Marshal(db.Photo{
		CreatedAt:   uploaded,
		UpdatedAt:   uploaded,
		TakenAt:     sql.NullTime{Time: captured, Valid: true},
		CameraMake:  sql.NullString{String: "Apple", Valid: true},
		CameraModel: sql.NullString{String: "iPhone 15 Pro", Valid: true},
	})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if err := json.Unmarshal(raw, &withEXIF); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if withEXIF.TakenAtSource != "exif" {
		t.Errorf("taken_at_source = %q, want exif", withEXIF.TakenAtSource)
	}
	if withEXIF.TakenAt != captured.Format(time.RFC3339) {
		t.Errorf("taken_at = %q, want %q", withEXIF.TakenAt, captured.Format(time.RFC3339))
	}
	if withEXIF.TakenAtEXIF == nil {
		t.Error("taken_at_exif should be set when EXIF supplied the time")
	}
	if withEXIF.CameraLabel != "Apple iPhone 15 Pro" {
		t.Errorf("camera_label = %q", withEXIF.CameraLabel)
	}

	// Without EXIF: taken_at still populated (upload time), exif field null.
	raw, _ = json.Marshal(db.Photo{CreatedAt: uploaded, UpdatedAt: uploaded})
	var noEXIF struct {
		TakenAt       string  `json:"taken_at"`
		TakenAtSource string  `json:"taken_at_source"`
		TakenAtEXIF   *string `json:"taken_at_exif"`
	}
	if err := json.Unmarshal(raw, &noEXIF); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if noEXIF.TakenAtSource != "upload" {
		t.Errorf("taken_at_source = %q, want upload", noEXIF.TakenAtSource)
	}
	if noEXIF.TakenAt != uploaded.Format(time.RFC3339) {
		t.Errorf("taken_at = %q, want the upload time %q", noEXIF.TakenAt, uploaded.Format(time.RFC3339))
	}
	if noEXIF.TakenAtEXIF != nil {
		t.Errorf("taken_at_exif should be null, got %q", *noEXIF.TakenAtEXIF)
	}
}

// The anchor edge must survive the round trip, and a row written before the
// column existed must read back as left rather than as an empty string the
// renderer wouldn't understand.
func TestTextOverlay_AlignRoundTrip(t *testing.T) {
	_, q := newTestHandlers(t)
	ctx := context.Background()
	project := newProject(t, q)

	right := &db.TextOverlay{
		ProjectID: project.ID, Text: "", FontSize: 30, Color: "#FFFFFF", Opacity: 1,
		OrientationID: db.OrientationLandscape,
		Source:        db.TextSourcePhotoDate,
		TextAlign:     db.TextAlignRight,
	}
	if err := q.CreateTextOverlay(ctx, right); err != nil {
		t.Fatalf("creating right-anchored overlay: %v", err)
	}
	got, err := q.GetTextOverlay(ctx, right.ID)
	if err != nil || got == nil {
		t.Fatalf("reading back: %v", err)
	}
	if got.TextAlign != db.TextAlignRight {
		t.Errorf("text_align = %q, want %q", got.TextAlign, db.TextAlignRight)
	}

	// Updating an unrelated field must not lose the anchor.
	got.FontSize = 44
	if err := q.UpdateTextOverlay(ctx, got); err != nil {
		t.Fatalf("updating: %v", err)
	}
	after, _ := q.GetTextOverlay(ctx, right.ID)
	if after.TextAlign != db.TextAlignRight {
		t.Errorf("after update text_align = %q, want preserved", after.TextAlign)
	}

	// An overlay created without an explicit anchor defaults to left.
	plain := &db.TextOverlay{
		ProjectID: project.ID, Text: "Smile!", FontSize: 25, Color: "#FFFFFF", Opacity: 1,
		OrientationID: db.OrientationLandscape,
	}
	if err := q.CreateTextOverlay(ctx, plain); err != nil {
		t.Fatalf("creating default overlay: %v", err)
	}
	plainBack, _ := q.GetTextOverlay(ctx, plain.ID)
	if plainBack.AlignOrDefault() != db.TextAlignLeft {
		t.Errorf("default align = %q, want left", plainBack.AlignOrDefault())
	}
}

// Alignment goes through the HTTP layer, and a bad value is rejected rather
// than silently stored as something the renderer will ignore.
func TestCreateTextOverlay_TextAlignViaHTTP(t *testing.T) {
	h, q := newTestHandlers(t)
	project := newProject(t, q)
	path := "/api/admin/projects/" + itoa(project.ID) + "/text-overlay"

	rec := doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"source":     db.TextSourcePhotoDate,
		"text_align": db.TextAlignRight,
		"font_size":  24,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		TextAlign string `json:"text_align"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if created.TextAlign != db.TextAlignRight {
		t.Errorf("text_align = %q, want %q", created.TextAlign, db.TextAlignRight)
	}

	// Omitted → left, matching prior behavior.
	rec = doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"text": "Smile!",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	created.TextAlign = ""
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.TextAlign != db.TextAlignLeft {
		t.Errorf("omitted text_align = %q, want left", created.TextAlign)
	}

	// Center is accepted too.
	rec = doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"text":       "Smile!",
		"text_align": db.TextAlignCenter,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("center text_align: status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	created.TextAlign = ""
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.TextAlign != db.TextAlignCenter {
		t.Errorf("text_align = %q, want %q", created.TextAlign, db.TextAlignCenter)
	}

	// Unknown value rejected.
	rec = doJSONWithID(t, h.CreateTextOverlay, "POST", path, project.ID, map[string]any{
		"text":       "Smile!",
		"text_align": "middle",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown text_align: status = %d, want 400", rec.Code)
	}
}
