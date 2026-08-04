package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scout-kit/fine-print/internal/storage"
)

// BackupDownload streams a gzipped tar containing a SQLite snapshot of
// the DB (via VACUUM INTO for a consistent read) and the originals/
// storage bucket. Rendered/preview/overlay buckets are excluded — they
// can be regenerated from the originals and the DB.
func (h *Handlers) BackupDownload(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Database.Driver != "sqlite" {
		writeError(w, http.StatusNotImplemented, "backup is only supported for sqlite databases")
		return
	}

	// VACUUM INTO a temp file so concurrent writes don't produce a torn
	// snapshot. This copies the DB into a fresh on-disk file that we can
	// then tar and stream.
	tmpDB, err := os.CreateTemp("", "fine-print-backup-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create temp file")
		return
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	defer os.Remove(tmpDBPath)

	// VACUUM INTO requires a path literal; escape single quotes defensively.
	safePath := strings.ReplaceAll(tmpDBPath, "'", "''")
	// Remove the temp file VACUUM INTO created itself; it refuses to
	// write to an existing path.
	_ = os.Remove(tmpDBPath)
	if _, err := h.queries.ExecDirect(r.Context(), fmt.Sprintf("VACUUM INTO '%s'", safePath)); err != nil {
		writeError(w, http.StatusInternalServerError, "snapshot failed: "+err.Error())
		return
	}

	filename := fmt.Sprintf("fine-print-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// The archive is streamed rather than buffered — originals can run to
	// gigabytes and this box is tight enough on space to ship a disk guard,
	// so staging a full copy is not an option.
	//
	// The consequence is that 200 and the headers are committed before the
	// first failure can occur, and neither tar/gzip Close is deferred: those
	// writers emit a valid end-of-archive marker, which would dress a
	// half-written backup up as a complete one. On failure we abort the
	// response instead, so the client sees a truncated transfer and a
	// corrupt archive rather than a silent partial success.
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	if err := func() error {
		if err := addFileToTar(tw, tmpDBPath, "fine-print.db"); err != nil {
			return fmt.Errorf("db snapshot: %w", err)
		}
		// Originals — iterate the storage bucket and add each file.
		if err := addDirToTar(tw, h.store.Path(storage.BucketOriginals, ""), "originals"); err != nil {
			return fmt.Errorf("originals: %w", err)
		}
		// Overlays — used in project templates, can't be regenerated.
		if err := addDirToTar(tw, h.store.Path(storage.BucketOverlays, ""), "overlays"); err != nil {
			return fmt.Errorf("overlays: %w", err)
		}
		// Fonts — same reasoning as overlays.
		if err := addDirToTar(tw, h.store.Path(storage.BucketFonts, ""), "fonts"); err != nil {
			return fmt.Errorf("fonts: %w", err)
		}
		// Only a clean finish gets the end-of-archive marker.
		if err := tw.Close(); err != nil {
			return fmt.Errorf("finalizing tar: %w", err)
		}
		return gz.Close()
	}(); err != nil {
		log.Printf("backup: aborting partial archive: %v", err)
		// Deliberately no tw.Close()/gz.Close() here. ErrAbortHandler makes
		// net/http drop the connection without logging a stack trace, which
		// is the only way to signal failure once the status line is out.
		panic(http.ErrAbortHandler)
	}
}

// BackupRestore accepts a tar.gz produced by BackupDownload, unpacks it
// to a staging directory, and atomically replaces the live files. The
// caller must restart the service afterward so the DB is reopened.
func (h *Handlers) BackupRestore(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Database.Driver != "sqlite" {
		writeError(w, http.StatusNotImplemented, "restore is only supported for sqlite databases")
		return
	}

	// 256 MB restore cap — more than enough for the originals + DB of a
	// typical event but prevents runaway uploads from filling the disk.
	const maxRestoreBytes = 256 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreBytes)

	if err := r.ParseMultipartForm(maxRestoreBytes); err != nil {
		writeError(w, http.StatusBadRequest, "upload too large or invalid")
		return
	}

	file, _, err := r.FormFile("backup")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'backup' field")
		return
	}
	defer file.Close()

	staging, err := os.MkdirTemp(h.cfg.DataDir, "restore-staging-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create staging dir")
		return
	}
	defer os.RemoveAll(staging)

	gz, err := gzip.NewReader(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "not a gzip file: "+err.Error())
		return
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	sawDB := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "corrupt tar: "+err.Error())
			return
		}
		// Guard against path traversal (".." or absolute paths). Matching on
		// a bare ".." prefix would also reject legitimate names like
		// "..config", so test for the path element itself.
		name := filepath.Clean(hdr.Name)
		if name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || filepath.IsAbs(name) {
			writeError(w, http.StatusBadRequest, "unsafe path in archive: "+hdr.Name)
			return
		}
		target := filepath.Join(staging, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				writeError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
				return
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				writeError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
				return
			}
			out, err := os.Create(target)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "create: "+err.Error())
				return
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				writeError(w, http.StatusInternalServerError, "copy: "+err.Error())
				return
			}
			out.Close()
			if name == "fine-print.db" {
				sawDB = true
			}
		default:
			// Skip symlinks, block devices, etc.
		}
	}

	if !sawDB {
		writeError(w, http.StatusBadRequest, "archive is missing fine-print.db — not a Fine Print backup")
		return
	}

	// Swap time — move old data aside, promote staging into place.
	stamp := time.Now().UTC().Format("20060102-150405")

	// Drain the WAL into the current database first, so the copy we're about
	// to move aside is a complete, self-contained file someone can roll back
	// to. Best-effort: a busy checkpoint shouldn't block the restore.
	if _, err := h.queries.ExecDirect(r.Context(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("restore: pre-swap checkpoint failed (rollback copy may be incomplete): %v", err)
	}

	// DB
	if err := swapFile(h.cfg.Database.SQLitePath, filepath.Join(staging, "fine-print.db"), stamp); err != nil {
		writeError(w, http.StatusInternalServerError, "swap db: "+err.Error())
		return
	}
	// The database runs in WAL mode, so the live DB has -wal/-shm sidecars
	// holding frames that belong to the database we just moved aside. The
	// snapshot in the archive is a VACUUM'd standalone file with no WAL, so
	// leaving the old sidecars next to it would let SQLite replay foreign
	// frames over the restored data. Move them aside with the same stamp so
	// the restored DB is opened clean (and the old set stays recoverable).
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := h.cfg.Database.SQLitePath + suffix
		if _, err := os.Stat(sidecar); err != nil {
			continue
		}
		if err := os.Rename(sidecar, fmt.Sprintf("%s.bak-%s", sidecar, stamp)); err != nil {
			// Refuse to leave a half-restored DB behind a stale WAL.
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to clear stale %s sidecar: %v", suffix, err))
			return
		}
	}
	// Buckets — best-effort; absence is fine (an event with no uploads).
	for _, sub := range []string{"originals", "overlays", "fonts"} {
		src := filepath.Join(staging, sub)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(h.cfg.DataDir, sub)
		if err := swapDir(dst, src, stamp); err != nil {
			log.Printf("restore: swap %s: %v", sub, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "restored",
		"requires_restart": true,
		"message":          "Backup restored. Restart the service to reopen the database.",
	})
}

// swapFile renames dst to dst.bak-STAMP then renames src into its place.
// If dst doesn't exist we just rename src.
func swapFile(dst, src, stamp string) error {
	if _, err := os.Stat(dst); err == nil {
		backup := fmt.Sprintf("%s.bak-%s", dst, stamp)
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// swapDir is like swapFile but for directories — moves dst aside, then
// moves src into its place.
func swapDir(dst, src, stamp string) error {
	if _, err := os.Stat(dst); err == nil {
		backup := fmt.Sprintf("%s.bak-%s", dst, stamp)
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
	}
	return os.Rename(src, dst)
}

// addFileToTar writes a single on-disk file into the tar stream at
// tarName. Returns nil when the file doesn't exist.
func addFileToTar(tw *tar.Writer, path, tarName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = tarName
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// addDirToTar walks src and writes every regular file found into the tar
// stream under prefix/. Missing src is silently skipped.
func addDirToTar(tw *tar.Writer, src, prefix string) error {
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		tarName := filepath.ToSlash(filepath.Join(prefix, rel))
		return addFileToTar(tw, path, tarName)
	})
}
