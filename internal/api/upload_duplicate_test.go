package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/settings"
	"github.com/scout-kit/fine-print/internal/storage"
)

// newUploadHandlers builds handlers with real storage and imaging, which
// UploadPhoto needs once a photo actually gets registered.
func newUploadHandlers(t *testing.T) (*api.Handlers, *db.Queries) {
	t.Helper()
	dir := t.TempDir()

	dbx, err := db.Open(config.DatabaseConfig{
		Driver:     "sqlite",
		SQLitePath: filepath.Join(dir, "test.db"),
	})
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })
	if err := db.Migrate(dbx, "sqlite"); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	q := db.NewQueries(dbx)
	store, err := storage.NewDiskStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}

	cfg := config.DefaultConfig()
	pipeline := imaging.NewPipeline(
		cfg.Imaging.PrintWidth,
		cfg.Imaging.PrintHeight,
		cfg.Imaging.PreviewMaxWidth,
		cfg.Imaging.JPEGQuality,
		cfg.Imaging.MaxUploadPixels,
	)

	h := api.NewHandlers(cfg, q, store, pipeline, nil, nil, nil, settings.NewStore(q), nil, nil)
	return h, q
}

// testJPEG returns a small decodable JPEG. shade varies the pixel values so
// callers can produce a second, genuinely different photo.
func testJPEG(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding test jpeg: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// postUpload performs a multipart upload, optionally with the guest cookie and
// the allow_duplicate confirmation.
func postUpload(t *testing.T, h http.HandlerFunc, content []byte, projectID uint64, session string, allowDuplicate bool) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("photo", "guest-photo.jpg")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := mw.WriteField("project_id", strconv.FormatUint(projectID, 10)); err != nil {
		t.Fatalf("writing project_id: %v", err)
	}
	if allowDuplicate {
		if err := mw.WriteField("allow_duplicate", "true"); err != nil {
			t.Fatalf("writing allow_duplicate: %v", err)
		}
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/photos", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "fineprint_guest", Value: session})
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response: %v; body = %s", err, rec.Body.String())
	}
	return out
}

func photoCount(t *testing.T, q *db.Queries, projectID uint64) int {
	t.Helper()
	photos, err := q.ListPhotosByProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("listing photos: %v", err)
	}
	return len(photos)
}

// The whole point: a re-upload is refused with a warning and, crucially,
// nothing is written — no row, so no orphaned photo if the guest backs out.
func TestUpload_DuplicateWarnsWithoutRegistering(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)
	content := testJPEG(t, 120)

	first := postUpload(t, h.UploadPhoto, content, project.ID, "sess-a", false)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: status = %d, want 201; body = %s", first.Code, first.Body.String())
	}

	second := postUpload(t, h.UploadPhoto, content, project.ID, "sess-a", false)
	if second.Code != http.StatusConflict {
		t.Fatalf("second upload: status = %d, want 409; body = %s", second.Code, second.Body.String())
	}
	if body := decodeBody(t, second); body["duplicate"] != true {
		t.Errorf("response missing duplicate marker: %s", second.Body.String())
	}
	if n := photoCount(t, q, project.ID); n != 1 {
		t.Errorf("project holds %d photos, want 1 — the duplicate was registered anyway", n)
	}
}

// The warning is not a ban: confirming registers the second copy.
func TestUpload_DuplicateAcceptedWhenConfirmed(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)
	content := testJPEG(t, 120)

	postUpload(t, h.UploadPhoto, content, project.ID, "sess-a", false)

	confirmed := postUpload(t, h.UploadPhoto, content, project.ID, "sess-a", true)
	if confirmed.Code != http.StatusCreated {
		t.Fatalf("confirmed upload: status = %d, want 201; body = %s", confirmed.Code, confirmed.Body.String())
	}
	if n := photoCount(t, q, project.ID); n != 2 {
		t.Errorf("project holds %d photos, want 2", n)
	}
}

// A different photo is never mistaken for a duplicate.
func TestUpload_DistinctPhotoIsNotDuplicate(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)

	postUpload(t, h.UploadPhoto, testJPEG(t, 120), project.ID, "sess-a", false)

	other := postUpload(t, h.UploadPhoto, testJPEG(t, 30), project.ID, "sess-a", false)
	if other.Code != http.StatusCreated {
		t.Fatalf("distinct upload: status = %d, want 201; body = %s", other.Code, other.Body.String())
	}
	if n := photoCount(t, q, project.ID); n != 2 {
		t.Errorf("project holds %d photos, want 2", n)
	}
}

// Duplicate detection is per project: the same photo submitted to two events
// is two separate prints, not a re-upload.
func TestUpload_SamePhotoInAnotherProjectIsNotDuplicate(t *testing.T) {
	h, q := newUploadHandlers(t)
	first := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)
	second := newProjectWithVisibility(t, q, "Reunion", db.VisibilityPublic)
	content := testJPEG(t, 120)

	postUpload(t, h.UploadPhoto, content, first.ID, "sess-a", false)

	rec := postUpload(t, h.UploadPhoto, content, second.ID, "sess-a", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("other project: status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
}

// The upload is hashed so later uploads have something to match against.
func TestUpload_RecordsContentHash(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)
	content := testJPEG(t, 120)

	postUpload(t, h.UploadPhoto, content, project.ID, "sess-a", false)

	found, err := q.FindPhotoByContentHash(context.Background(), project.ID, sha256Hex(content))
	if err != nil {
		t.Fatalf("looking up by hash: %v", err)
	}
	if found == nil {
		t.Fatal("uploaded photo was not recorded under its content hash")
	}
}

// The existing photo's id is a handle another guest could use to fetch its
// preview, so it goes only to the session that uploaded it. Everyone else
// gets the warning without the id.
func TestUpload_DuplicateIDOnlyDisclosedToItsUploader(t *testing.T) {
	h, q := newUploadHandlers(t)
	project := newProjectWithVisibility(t, q, "Wedding", db.VisibilityPublic)
	content := testJPEG(t, 120)

	postUpload(t, h.UploadPhoto, content, project.ID, "owner-session", false)

	own := decodeBody(t, postUpload(t, h.UploadPhoto, content, project.ID, "owner-session", false))
	if own["mine"] != true {
		t.Errorf("uploader's own duplicate reported mine = %v, want true", own["mine"])
	}
	if _, ok := own["photo_id"]; !ok {
		t.Error("uploader should be told which of their photos it duplicates")
	}

	other := decodeBody(t, postUpload(t, h.UploadPhoto, content, project.ID, "other-session", false))
	if other["mine"] != false {
		t.Errorf("another guest's duplicate reported mine = %v, want false", other["mine"])
	}
	if id, ok := other["photo_id"]; ok {
		t.Errorf("another guest's photo id leaked: %v", id)
	}
}
