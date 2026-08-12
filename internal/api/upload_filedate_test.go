package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
)

// postUploadWithFileTime uploads content, reporting a browser file
// modification time in the form field the guest UI sends.
func postUploadWithFileTime(t *testing.T, h http.HandlerFunc, content []byte, projectID uint64, fileModified string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("photo", "desktop-export.jpg")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	mw.WriteField("project_id", strconv.FormatUint(projectID, 10))
	if fileModified != "" {
		mw.WriteField("file_modified", fileModified)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// awaitPhoto polls until the photo's metadata has been written, since the
// upload handler reads it on a background goroutine.
func awaitPhoto(t *testing.T, q *db.Queries, id uint64, want func(*db.Photo) bool) *db.Photo {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last *db.Photo
	for time.Now().Before(deadline) {
		photo, err := q.GetPhoto(context.Background(), id)
		if err != nil {
			t.Fatalf("reading photo: %v", err)
		}
		last = photo
		if photo != nil && want(photo) {
			return photo
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last
}

// newestPhoto returns the last photo uploaded to a project.
func newestPhoto(t *testing.T, q *db.Queries, projectID uint64) *db.Photo {
	t.Helper()
	photos, err := q.ListPhotosByProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("listing photos: %v", err)
	}
	if len(photos) == 0 {
		t.Fatal("no photos in project")
	}
	return &photos[len(photos)-1]
}

// withIPTCDate splices an APP13 segment carrying an IPTC DateCreated into a
// JPEG, the way a desktop editor's export does.
func withIPTCDate(jpeg []byte, date, clock string) []byte {
	var iptc bytes.Buffer
	for _, ds := range []struct {
		id    byte
		value string
	}{{55, date}, {60, clock}} {
		iptc.Write([]byte{0x1C, 0x02, ds.id})
		binary.Write(&iptc, binary.BigEndian, uint16(len(ds.value)))
		iptc.WriteString(ds.value)
	}

	var resource bytes.Buffer
	resource.WriteString("Photoshop 3.0\x00")
	resource.WriteString("8BIM")
	binary.Write(&resource, binary.BigEndian, uint16(0x0404))
	resource.Write([]byte{0, 0}) // empty, padded Pascal name
	binary.Write(&resource, binary.BigEndian, uint32(iptc.Len()))
	resource.Write(iptc.Bytes())
	if iptc.Len()%2 != 0 {
		resource.WriteByte(0)
	}

	var out bytes.Buffer
	out.Write(jpeg[:2]) // SOI
	out.Write([]byte{0xFF, 0xED})
	binary.Write(&out, binary.BigEndian, uint16(resource.Len()+2))
	out.Write(resource.Bytes())
	out.Write(jpeg[2:])
	return out.Bytes()
}

// A photo exported from a desktop editor carries no capture time. The file's
// own date is the only thing left to date it by, and it is recorded as such.
func TestUpload_FileTimeDatesAPhotoWithNoMetadata(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)

	fileTime := time.Date(2022, 10, 15, 8, 15, 0, 0, time.Local)
	rec := postUploadWithFileTime(t, h.UploadPhoto, testJPEG(t, 90), project.ID,
		strconv.FormatInt(fileTime.UnixMilli(), 10))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}

	photo := awaitPhoto(t, q, newestPhoto(t, q, project.ID).ID, func(p *db.Photo) bool {
		return p.TakenAt.Valid
	})
	if !photo.TakenAt.Valid {
		t.Fatal("taken_at was never set from the file time")
	}

	at, source := photo.EffectiveTakenAt()
	if source != db.TakenAtSourceFile {
		t.Errorf("source = %q, want %q", source, db.TakenAtSourceFile)
	}
	if got, want := at.Format("2006-01-02 15:04"), fileTime.Format("2006-01-02 15:04"); got != want {
		t.Errorf("takenAt = %s, want %s", got, want)
	}
}

// The file's date is the weakest signal there is, so anything the photo itself
// recorded outranks it.
func TestUpload_MetadataDateBeatsFileTime(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)

	fileTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	content := withIPTCDate(testJPEG(t, 90), "20220822", "190006")
	rec := postUploadWithFileTime(t, h.UploadPhoto, content, project.ID,
		strconv.FormatInt(fileTime.UnixMilli(), 10))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}

	photo := awaitPhoto(t, q, newestPhoto(t, q, project.ID).ID, func(p *db.Photo) bool {
		return p.TakenAt.Valid
	})
	if !photo.TakenAt.Valid {
		t.Fatal("taken_at was never set")
	}

	at, source := photo.EffectiveTakenAt()
	if source != db.TakenAtSourceIPTC {
		t.Errorf("source = %q, want %q", source, db.TakenAtSourceIPTC)
	}
	if got, want := at.Format("2006-01-02 15:04:05"), "2022-08-22 19:00:06"; got != want {
		t.Errorf("takenAt = %s, want %s — the file's date should not have won", got, want)
	}
}

// The value comes from the client, so a nonsensical one is dropped and the
// photo falls back to the upload time rather than printing a bogus date.
func TestUpload_ImplausibleFileTimeIsIgnored(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)

	cases := []struct {
		name  string
		value string
	}{
		{"absent", ""},
		{"zero", "0"},
		{"negative", "-86400000"},
		{"not a number", "yesterday"},
		{"far future", strconv.FormatInt(time.Now().AddDate(2, 0, 0).UnixMilli(), 10)},
		{"before photography", strconv.FormatInt(time.Date(1780, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10)},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A distinct shade per case keeps each upload from tripping the
			// duplicate check on the previous one.
			rec := postUploadWithFileTime(t, h.UploadPhoto, testJPEG(t, uint8(10+i*20)), project.ID, tc.value)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
			}

			photo := newestPhoto(t, q, project.ID)
			// Give the background read a chance to write a date it shouldn't.
			awaitPhoto(t, q, photo.ID, func(p *db.Photo) bool { return p.PreviewKey.Valid })

			got, err := q.GetPhoto(context.Background(), photo.ID)
			if err != nil {
				t.Fatalf("reading photo: %v", err)
			}
			if got.TakenAt.Valid {
				t.Errorf("taken_at = %v, want NULL", got.TakenAt.Time)
			}
			if _, source := got.EffectiveTakenAt(); source != db.TakenAtSourceUpload {
				t.Errorf("source = %q, want %q", source, db.TakenAtSourceUpload)
			}
		})
	}
}
