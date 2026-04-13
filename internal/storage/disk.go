package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DiskStore implements Store using the local filesystem.
type DiskStore struct {
	rootDir string
}

func NewDiskStore(rootDir string) (*DiskStore, error) {
	buckets := []string{
		BucketOriginals,
		BucketPreviews,
		BucketRendered,
		BucketOverlays,
		BucketFonts,
	}
	for _, b := range buckets {
		dir := filepath.Join(rootDir, b)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}
	return &DiskStore{rootDir: rootDir}, nil
}

func (s *DiskStore) Save(bucket, key string, r io.Reader) error {
	p := s.Path(bucket, key)

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", p, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("writing file %s: %w", p, err)
	}
	return nil
}

func (s *DiskStore) Open(bucket, key string) (io.ReadCloser, error) {
	p := s.Path(bucket, key)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", p, err)
	}
	return f, nil
}

func (s *DiskStore) Delete(bucket, key string) error {
	p := s.Path(bucket, key)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file %s: %w", p, err)
	}
	return nil
}

func (s *DiskStore) Path(bucket, key string) string {
	return filepath.Join(s.rootDir, bucket, key)
}

func (s *DiskStore) Exists(bucket, key string) bool {
	_, err := os.Stat(s.Path(bucket, key))
	return err == nil
}
