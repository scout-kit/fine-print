//go:build unix

package diskguard

import (
	"fmt"
	"syscall"
)

// statfs returns total and available bytes for the filesystem holding path.
//
// Available-to-non-root (Bavail) is used rather than Bfree, which matches
// what `df` reports and what users see in the UI.
func statfs(path string) (total, free int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	blockSize := int64(stat.Bsize)
	return int64(stat.Blocks) * blockSize, int64(stat.Bavail) * blockSize, nil
}
