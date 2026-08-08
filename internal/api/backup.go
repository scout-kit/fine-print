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

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := addFileToTar(tw, tmpDBPath, "fine-print.db"); err != nil {
		log.Printf("backup: tar db: %v", err)
		return
	}

	// Originals — iterate the storage bucket and add each file.
	originalsDir := h.store.Path(storage.BucketOriginals, "")
	if err := addDirToTar(tw, originalsDir, "originals"); err != nil {
		log.Printf("backup: tar originals: %v", err)
	}

	// Overlays — used in project templates, can't be regenerated.
	overlaysDir := h.store.Path(storage.BucketOverlays, "")
	if err := addDirToTar(tw, overlaysDir, "overlays"); err != nil {
		log.Printf("backup: tar overlays: %v", err)
	}

	// Fonts — same reasoning as overlays.
	fontsDir := h.store.Path(storage.BucketFonts, "")
	if err := addDirToTar(tw, fontsDir, "fonts"); err != nil {
		log.Printf("backup: tar fonts: %v", err)
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
		// Guard against path traversal (".." or absolute paths).
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
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

	// DB
	if err := swapFile(h.cfg.Database.SQLitePath, filepath.Join(staging, "fine-print.db"), stamp); err != nil {
		writeError(w, http.StatusInternalServerError, "swap db: "+err.Error())
		return
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
