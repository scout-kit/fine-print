package storage

import "io"

// Store defines the interface for file storage operations.
type Store interface {
	// Save writes data to the given key (relative path within a bucket).
	Save(bucket, key string, r io.Reader) error
	// Open returns a reader for the given key.
	Open(bucket, key string) (io.ReadCloser, error)
	// Delete removes the file at the given key.
	Delete(bucket, key string) error
	// Path returns the absolute filesystem path for a key.
	Path(bucket, key string) string
	// Exists checks if a file exists at the given key.
	Exists(bucket, key string) bool
}

// Bucket names for organizing stored files.
const (
	BucketOriginals = "originals"
	BucketPreviews  = "previews"
	BucketRendered  = "rendered"
	BucketOverlays  = "overlays"
	BucketFonts     = "fonts"
)
