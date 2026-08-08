package api

import (
	"net/http"
	"strconv"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// Gallery returns photos for the public guest gallery view.
//
// This is an unauthenticated endpoint, so it only ever exposes photos from
// projects marked public. Hidden projects are reachable by link/QR precisely
// because they are meant to stay out of listings like this one, and private
// projects are not guest-visible at all.
//
// Optional query param: ?project_id=N narrows to one project. That project
// must also be public — allowing hidden projects to be fetched by numeric id
// would defeat the point of their unguessable slug, since ids are sequential
// and trivially enumerated.
func (h *Handlers) Gallery(w http.ResponseWriter, r *http.Request) {
	var photos []db.Photo
	var err error

	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr != "" {
		projectID, parseErr := strconv.ParseUint(projectIDStr, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		project, projErr := h.queries.GetProject(r.Context(), projectID)
		if projErr != nil || project == nil || project.VisibilityID != db.VisibilityPublic {
			// Same 404 for "no such project" and "not public", so the
			// response can't be used to probe which ids exist.
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		photos, err = h.queries.ListGalleryPhotos(r.Context(), projectID)
	} else {
		photos, err = h.queries.ListPublicGalleryPhotos(r.Context())
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list gallery")
		return
	}

	// IsMine is resolved server-side from the requester's own cookie. The
	// raw session_id is deliberately not published: guest sessions are
	// accepted verbatim from the cookie, so handing out other guests'
	// session ids let anyone adopt one and delete that guest's photos.
	type galleryPhoto struct {
		ID         uint64 `json:"id"`
		ProjectID  uint64 `json:"project_id"`
		StatusID   uint   `json:"status_id"`
		Status     string `json:"status"`
		HasPreview bool   `json:"has_preview"`
		HasRender  bool   `json:"has_render"`
		IsMine     bool   `json:"is_mine"`
		CreatedAt  string `json:"created_at"`
	}

	// Absent cookie leaves this empty, which matches no photo.
	var session string
	if cookie, cerr := r.Cookie("fineprint_guest"); cerr == nil {
		session = cookie.Value
	}

	result := make([]galleryPhoto, 0, len(photos))
	for _, p := range photos {
		result = append(result, galleryPhoto{
			ID:         p.ID,
			ProjectID:  p.ProjectID,
			StatusID:   p.StatusID,
			Status:     db.PhotoStatusName(p.StatusID),
			HasPreview: p.PreviewKey.Valid,
			HasRender:  p.RenderedKey.Valid,
			IsMine:     session != "" && p.SessionID == session,
			CreatedAt:  p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// DeleteOwnPhoto allows a guest to delete their own photo (matched by session).
func (h *Handlers) DeleteOwnPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid photo id")
		return
	}

	cookie, err := r.Cookie("fineprint_guest")
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "no session")
		return
	}

	photo, err := h.queries.GetPhoto(r.Context(), id)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}

	if photo.SessionID != cookie.Value {
		writeError(w, http.StatusForbidden, "not your photo")
		return
	}

	if photo.StatusID == db.PhotoStatusPrinting {
		writeError(w, http.StatusBadRequest, "cannot delete while printing")
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

	h.queries.DeletePhoto(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
