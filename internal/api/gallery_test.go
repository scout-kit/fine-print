package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/scout-kit/fine-print/internal/db"
)

// galleryEntry mirrors the gallery response shape. session_id is included so
// the tests can assert it is *absent* from the JSON.
type galleryEntry struct {
	ID        uint64 `json:"id"`
	ProjectID uint64 `json:"project_id"`
	IsMine    bool   `json:"is_mine"`
	SessionID string `json:"session_id"`
}

func newProjectWithVisibility(t *testing.T, q *db.Queries, name string, visibility uint) *db.Project {
	t.Helper()
	p := &db.Project{Name: name, VisibilityID: visibility, ProjectTypeID: db.ProjectTypeStandard}
	if err := q.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("creating %s project: %v", name, err)
	}
	return p
}

func newPhotoIn(t *testing.T, q *db.Queries, projectID uint64, session string, status uint) *db.Photo {
	t.Helper()
	p := &db.Photo{ProjectID: projectID, SessionID: session, StatusID: status}
	if err := q.CreatePhoto(context.Background(), p); err != nil {
		t.Fatalf("creating photo: %v", err)
	}
	return p
}

// getGallery calls the handler with an optional project_id and guest cookie.
func getGallery(t *testing.T, h http.HandlerFunc, projectID string, session string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/gallery"
	if projectID != "" {
		url += "?project_id=" + projectID
	}
	req := httptest.NewRequest("GET", url, nil)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "fineprint_guest", Value: session})
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func decodeGallery(t *testing.T, rec *httptest.ResponseRecorder) []galleryEntry {
	t.Helper()
	var out []galleryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding gallery: %v; body = %s", err, rec.Body.String())
	}
	return out
}

func projectIDs(entries []galleryEntry) map[uint64]bool {
	seen := map[uint64]bool{}
	for _, e := range entries {
		seen[e.ProjectID] = true
	}
	return seen
}

// The bug: the unscoped gallery selected every photo with no visibility join,
// so photos from hidden (link/QR-only) and private projects were listed to any
// guest who opened the gallery page.
func TestGallery_UnscopedExcludesHiddenAndPrivateProjects(t *testing.T) {
	h, q := newTestHandlers(t)

	public := newProjectWithVisibility(t, q, "Public Event", db.VisibilityPublic)
	hidden := newProjectWithVisibility(t, q, "Hidden Event", db.VisibilityHidden)
	private := newProjectWithVisibility(t, q, "Private Event", db.VisibilityPrivate)

	newPhotoIn(t, q, public.ID, "sess-a", db.PhotoStatusApproved)
	newPhotoIn(t, q, hidden.ID, "sess-b", db.PhotoStatusApproved)
	newPhotoIn(t, q, private.ID, "sess-c", db.PhotoStatusApproved)

	rec := getGallery(t, h.Gallery, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	seen := projectIDs(decodeGallery(t, rec))
	if !seen[public.ID] {
		t.Error("public project photo should be listed")
	}
	if seen[hidden.ID] {
		t.Error("hidden project photo leaked into the unscoped gallery")
	}
	if seen[private.ID] {
		t.Error("private project photo leaked into the unscoped gallery")
	}
}

// Rejected photos stayed hidden before this change and must continue to.
func TestGallery_UnscopedStillExcludesRejected(t *testing.T) {
	h, q := newTestHandlers(t)
	public := newProjectWithVisibility(t, q, "Public Event", db.VisibilityPublic)

	keep := newPhotoIn(t, q, public.ID, "sess-a", db.PhotoStatusApproved)
	rejected := newPhotoIn(t, q, public.ID, "sess-a", db.PhotoStatusRejected)

	entries := decodeGallery(t, getGallery(t, h.Gallery, "", ""))

	ids := map[uint64]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	if !ids[keep.ID] {
		t.Error("approved photo should be listed")
	}
	if ids[rejected.ID] {
		t.Error("rejected photo should not be listed")
	}
}

// A public project may be requested by id.
func TestGallery_ScopedToPublicProjectSucceeds(t *testing.T) {
	h, q := newTestHandlers(t)
	public := newProjectWithVisibility(t, q, "Public Event", db.VisibilityPublic)
	photo := newPhotoIn(t, q, public.ID, "sess-a", db.PhotoStatusApproved)

	rec := getGallery(t, h.Gallery, strconv.FormatUint(public.ID, 10), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	entries := decodeGallery(t, rec)
	if len(entries) != 1 || entries[0].ID != photo.ID {
		t.Errorf("got %+v, want just photo %d", entries, photo.ID)
	}
}

// Project ids are sequential and trivially enumerated, so fetching a hidden
// project's gallery by id would defeat its unguessable slug entirely.
func TestGallery_ScopedToHiddenOrPrivateProjectIs404(t *testing.T) {
	h, q := newTestHandlers(t)

	hidden := newProjectWithVisibility(t, q, "Hidden Event", db.VisibilityHidden)
	private := newProjectWithVisibility(t, q, "Private Event", db.VisibilityPrivate)
	newPhotoIn(t, q, hidden.ID, "sess-b", db.PhotoStatusApproved)
	newPhotoIn(t, q, private.ID, "sess-c", db.PhotoStatusApproved)

	for _, tc := range []struct {
		name string
		id   uint64
	}{
		{"hidden", hidden.ID},
		{"private", private.ID},
	} {
		rec := getGallery(t, h.Gallery, strconv.FormatUint(tc.id, 10), "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s project: status = %d, want 404; body = %s", tc.name, rec.Code, rec.Body.String())
		}
		// And nothing from it may appear in the body.
		if strings.Contains(rec.Body.String(), "\"project_id\"") {
			t.Errorf("%s project: response contained photo data: %s", tc.name, rec.Body.String())
		}
	}
}

// A nonexistent id returns the same 404 as a non-public one, so responses
// can't be used to probe which project ids exist.
func TestGallery_UnknownProjectIsIndistinguishableFromHidden(t *testing.T) {
	h, q := newTestHandlers(t)
	hidden := newProjectWithVisibility(t, q, "Hidden Event", db.VisibilityHidden)

	missing := getGallery(t, h.Gallery, "999999", "")
	hiddenRec := getGallery(t, h.Gallery, strconv.FormatUint(hidden.ID, 10), "")

	if missing.Code != http.StatusNotFound || hiddenRec.Code != http.StatusNotFound {
		t.Fatalf("codes = %d / %d, want both 404", missing.Code, hiddenRec.Code)
	}
	if missing.Body.String() != hiddenRec.Body.String() {
		t.Errorf("responses differ, allowing id probing:\n missing = %s\n hidden  = %s",
			missing.Body.String(), hiddenRec.Body.String())
	}
}

func TestGallery_RejectsMalformedProjectID(t *testing.T) {
	h, _ := newTestHandlers(t)
	rec := getGallery(t, h.Gallery, "not-a-number", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// Guest session ids are accepted verbatim from the cookie and are what
// DeleteOwnPhoto authorizes against, so publishing them let any guest adopt
// another's session and delete their photos.
func TestGallery_DoesNotPublishSessionIDs(t *testing.T) {
	h, q := newTestHandlers(t)
	public := newProjectWithVisibility(t, q, "Public Event", db.VisibilityPublic)
	newPhotoIn(t, q, public.ID, "victim-session-token", db.PhotoStatusApproved)

	rec := getGallery(t, h.Gallery, "", "")

	if strings.Contains(rec.Body.String(), "victim-session-token") {
		t.Errorf("gallery response leaked a session id: %s", rec.Body.String())
	}
	for _, e := range decodeGallery(t, rec) {
		if e.SessionID != "" {
			t.Errorf("session_id present in response: %q", e.SessionID)
		}
	}
}

// Ownership is resolved server-side, because the guest cookie is HttpOnly and
// the browser can't read it.
func TestGallery_IsMineReflectsRequesterCookie(t *testing.T) {
	h, q := newTestHandlers(t)
	public := newProjectWithVisibility(t, q, "Public Event", db.VisibilityPublic)

	mine := newPhotoIn(t, q, public.ID, "sess-mine", db.PhotoStatusApproved)
	theirs := newPhotoIn(t, q, public.ID, "sess-theirs", db.PhotoStatusApproved)

	entries := decodeGallery(t, getGallery(t, h.Gallery, "", "sess-mine"))
	byID := map[uint64]galleryEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	if !byID[mine.ID].IsMine {
		t.Error("own photo should have is_mine = true")
	}
	if byID[theirs.ID].IsMine {
		t.Error("another guest's photo must not have is_mine = true")
	}

	// No cookie at all: nothing is mine.
	for _, e := range decodeGallery(t, getGallery(t, h.Gallery, "", "")) {
		if e.IsMine {
			t.Errorf("photo %d reported is_mine with no session cookie", e.ID)
		}
	}
}
