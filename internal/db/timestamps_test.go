package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
)

func newTestDB(t *testing.T) *db.Queries {
	t.Helper()
	dbx, err := db.Open(config.DatabaseConfig{
		Driver:     "sqlite",
		SQLitePath: filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })
	if err := db.Migrate(dbx, "sqlite"); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db.NewQueries(dbx)
}

func newTestPhoto(t *testing.T, q *db.Queries) *db.Photo {
	t.Helper()
	ctx := context.Background()
	project := &db.Project{Name: "Event", VisibilityID: db.VisibilityPublic, ProjectTypeID: db.ProjectTypeStandard}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("creating project: %v", err)
	}
	photo := &db.Photo{ProjectID: project.ID, SessionID: "sess", StatusID: db.PhotoStatusUploaded}
	if err := q.CreatePhoto(ctx, photo); err != nil {
		t.Fatalf("creating photo: %v", err)
	}
	return photo
}

// A capture time whose zone is named numerically — what EXIF's
// OffsetTimeOriginal produces — used to be stored in a form the SQLite driver
// could not read back, so every later SELECT over the project's photos failed
// to scan and the whole list 500'd.
func TestCaptureMetadata_NumericZoneStaysReadable(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	photo := newTestPhoto(t, q)

	// time.FixedZone("-04:00", …) is exactly what parseEXIFOffset builds.
	taken := time.Date(2026, 8, 6, 9, 6, 37, 0, time.FixedZone("-04:00", -4*3600))
	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, taken, db.TakenAtSourceEXIF, "Canon", "EOS R6"); err != nil {
		t.Fatalf("storing capture metadata: %v", err)
	}

	// The read that used to fail.
	photos, err := q.ListPhotosByProject(ctx, photo.ProjectID)
	if err != nil {
		t.Fatalf("listing photos: %v", err)
	}
	if len(photos) != 1 {
		t.Fatalf("got %d photos, want 1", len(photos))
	}

	got := photos[0]
	if !got.TakenAt.Valid {
		t.Fatal("taken_at came back NULL")
	}
	// The camera's own reading is what a date overlay prints, so the wall
	// clock must survive the round trip unshifted.
	if want := "2026-08-06 09:06:37"; got.TakenAt.Time.Format("2006-01-02 15:04:05") != want {
		t.Errorf("taken_at = %s, want %s", got.TakenAt.Time.Format("2006-01-02 15:04:05"), want)
	}
}

// Every other read path over the same row must survive it too.
func TestCaptureMetadata_NumericZoneReadableEverywhere(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	photo := newTestPhoto(t, q)

	taken := time.Date(2026, 8, 6, 9, 6, 37, 0, time.FixedZone("-04:00", -4*3600))
	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, taken, db.TakenAtSourceEXIF, "Canon", "EOS R6"); err != nil {
		t.Fatalf("storing capture metadata: %v", err)
	}
	// Status updates and previews write timestamps of their own.
	if err := q.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusApproved); err != nil {
		t.Fatalf("updating status: %v", err)
	}
	if err := q.UpdatePhotoPreview(ctx, photo.ID, "1.jpg", 800, 600, 1234, "image/jpeg"); err != nil {
		t.Fatalf("updating preview: %v", err)
	}

	if _, err := q.GetPhoto(ctx, photo.ID); err != nil {
		t.Errorf("GetPhoto: %v", err)
	}
	if _, err := q.ListPhotosBySession(ctx, "sess"); err != nil {
		t.Errorf("ListPhotosBySession: %v", err)
	}
	if _, err := q.ListPhotosByStatus(ctx, db.PhotoStatusApproved); err != nil {
		t.Errorf("ListPhotosByStatus: %v", err)
	}
	if _, err := q.GetPhotosByIDs(ctx, []uint64{photo.ID}); err != nil {
		t.Errorf("GetPhotosByIDs: %v", err)
	}
}

// A file with no capture time leaves taken_at NULL, so consumers fall back to
// the upload time rather than printing a zero date.
func TestCaptureMetadata_ZeroTimeStoresNull(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	photo := newTestPhoto(t, q)

	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, time.Time{}, "", "Canon", ""); err != nil {
		t.Fatalf("storing capture metadata: %v", err)
	}

	got, err := q.GetPhoto(ctx, photo.ID)
	if err != nil {
		t.Fatalf("reading photo back: %v", err)
	}
	if got.TakenAt.Valid {
		t.Errorf("taken_at = %v, want NULL", got.TakenAt.Time)
	}
}

// The recorded source travels with the date, so the UI can qualify one the
// camera never took.
func TestCaptureMetadata_SourceRoundTrips(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()

	for _, source := range []string{db.TakenAtSourceEXIF, db.TakenAtSourceIPTC, db.TakenAtSourceFile} {
		photo := newTestPhoto(t, q)
		taken := time.Date(2022, 10, 15, 8, 15, 0, 0, time.UTC)
		if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, taken, source, "", ""); err != nil {
			t.Fatalf("%s: storing capture metadata: %v", source, err)
		}

		got, err := q.GetPhoto(ctx, photo.ID)
		if err != nil {
			t.Fatalf("%s: reading photo back: %v", source, err)
		}
		at, gotSource := got.EffectiveTakenAt()
		if gotSource != source {
			t.Errorf("source = %q, want %q", gotSource, source)
		}
		if !at.Equal(taken) {
			t.Errorf("%s: takenAt = %v, want %v", source, at, taken)
		}
	}
}

// Rows written before the source column existed carry a date that could only
// have come from EXIF, and must keep reading that way.
func TestCaptureMetadata_MissingSourceReadsAsEXIF(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	photo := newTestPhoto(t, q)

	taken := time.Date(2024, 3, 14, 14, 31, 0, 0, time.UTC)
	if err := q.UpdatePhotoCaptureMetadata(ctx, photo.ID, taken, "", "", ""); err != nil {
		t.Fatalf("storing capture metadata: %v", err)
	}

	got, err := q.GetPhoto(ctx, photo.ID)
	if err != nil {
		t.Fatalf("reading photo back: %v", err)
	}
	if _, source := got.EffectiveTakenAt(); source != db.TakenAtSourceEXIF {
		t.Errorf("source = %q, want %q", source, db.TakenAtSourceEXIF)
	}
}

// With no date at all, the upload time stands in and says so.
func TestCaptureMetadata_NoDateReportsUploadSource(t *testing.T) {
	q := newTestDB(t)
	photo, err := newTestDBPhotoRead(t, q)
	if err != nil {
		t.Fatalf("reading photo back: %v", err)
	}
	at, source := photo.EffectiveTakenAt()
	if source != db.TakenAtSourceUpload {
		t.Errorf("source = %q, want %q", source, db.TakenAtSourceUpload)
	}
	if !at.Equal(photo.CreatedAt) {
		t.Errorf("takenAt = %v, want the upload time %v", at, photo.CreatedAt)
	}
}

func newTestDBPhotoRead(t *testing.T, q *db.Queries) (*db.Photo, error) {
	t.Helper()
	photo := newTestPhoto(t, q)
	return q.GetPhoto(context.Background(), photo.ID)
}

// Admin sessions are looked up with an expiry comparison, so their timestamps
// have to be stored in a form that both reads back and compares correctly.
func TestAdminSession_ExpiryRoundTrips(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()

	live := &db.AdminSession{Token: "live", ExpiresAt: time.Now().Add(time.Hour)}
	if err := q.CreateAdminSession(ctx, live); err != nil {
		t.Fatalf("creating session: %v", err)
	}
	expired := &db.AdminSession{Token: "expired", ExpiresAt: time.Now().Add(-time.Hour)}
	if err := q.CreateAdminSession(ctx, expired); err != nil {
		t.Fatalf("creating expired session: %v", err)
	}

	got, err := q.GetAdminSessionByToken(ctx, "live")
	if err != nil {
		t.Fatalf("looking up live session: %v", err)
	}
	if got == nil {
		t.Error("live session was not found")
	}

	got, err = q.GetAdminSessionByToken(ctx, "expired")
	if err != nil {
		t.Fatalf("looking up expired session: %v", err)
	}
	if got != nil {
		t.Error("expired session was returned")
	}
}
