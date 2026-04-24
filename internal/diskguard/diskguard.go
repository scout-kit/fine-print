// Package diskguard monitors free space on the data volume and provides
// a gate for upload operations. Falling below a configurable minimum
// rejects new uploads with HTTP 507 until space is freed.
package diskguard

import (
	"fmt"
	"sync"
	"syscall"
	"time"
)

// DefaultMinFreeBytes is the floor below which uploads are refused.
// Chosen to leave enough room for a handful of full-resolution uploads
// plus their rendered copies on an SD card.
const DefaultMinFreeBytes int64 = 2 * 1024 * 1024 * 1024 // 2 GiB

// WarningThreshold is the used fraction above which the admin UI shows
// a persistent banner, regardless of absolute free space.
const WarningThreshold = 0.90

// Usage is a point-in-time snapshot of filesystem usage for the data dir.
type Usage struct {
	TotalBytes    int64   `json:"total_bytes"`
	FreeBytes     int64   `json:"free_bytes"`
	UsedBytes     int64   `json:"used_bytes"`
	UsedFraction  float64 `json:"used_fraction"` // 0.0–1.0
	MinFreeBytes  int64   `json:"min_free_bytes"`
	AboveMinFree  bool    `json:"above_min_free"`
	WarnThreshold float64 `json:"warn_threshold"`
	WarnActive    bool    `json:"warn_active"`
	Message       string  `json:"message"` // Human-readable line for the UI banner.
	CheckedAt     string  `json:"checked_at"`
}

// Guard checks available space for a given path.
type Guard struct {
	path         string
	mu           sync.RWMutex
	minFreeBytes int64
}

func New(path string, minFreeBytes int64) *Guard {
	if minFreeBytes <= 0 {
		minFreeBytes = DefaultMinFreeBytes
	}
	return &Guard{path: path, minFreeBytes: minFreeBytes}
}

// SetMinFreeBytes updates the minimum-free threshold at runtime (e.g. when
// the admin edits it in settings).
func (g *Guard) SetMinFreeBytes(n int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n > 0 {
		g.minFreeBytes = n
	}
}

// Usage returns a fresh snapshot of disk usage.
func (g *Guard) Usage() (Usage, error) {
	g.mu.RLock()
	minFree := g.minFreeBytes
	path := g.path
	g.mu.RUnlock()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Usage{}, fmt.Errorf("statfs %s: %w", path, err)
	}

	// Use available-to-non-root (Bavail) rather than Bfree, which matches
	// what `df` reports and what users see in the UI.
	blockSize := int64(stat.Bsize)
	total := int64(stat.Blocks) * blockSize
	free := int64(stat.Bavail) * blockSize
	used := total - free
	usedFrac := 0.0
	if total > 0 {
		usedFrac = float64(used) / float64(total)
	}

	u := Usage{
		TotalBytes:    total,
		FreeBytes:     free,
		UsedBytes:     used,
		UsedFraction:  usedFrac,
		MinFreeBytes:  minFree,
		AboveMinFree:  free >= minFree,
		WarnThreshold: WarningThreshold,
		WarnActive:    usedFrac >= WarningThreshold,
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	// Build the banner message. 100% case gets a distinct phrasing so
	// admins see "disk is full" rather than an ambiguous percentage.
	switch {
	case !u.AboveMinFree && free <= 0:
		u.Message = "Disk is full. Uploads are blocked until space is freed."
	case !u.AboveMinFree:
		u.Message = fmt.Sprintf("Only %s free — below the %s upload limit. Uploads are blocked.",
			formatBytes(free), formatBytes(minFree))
	case u.WarnActive:
		u.Message = fmt.Sprintf("Disk is %.0f%% full — %s of %s used, %s free.",
			usedFrac*100, formatBytes(used), formatBytes(total), formatBytes(free))
	}

	return u, nil
}

// Allow reports whether an upload of approxBytes can proceed. A zero or
// negative approxBytes only checks the min-free floor.
func (g *Guard) Allow(approxBytes int64) (bool, Usage, error) {
	u, err := g.Usage()
	if err != nil {
		return false, u, err
	}
	if !u.AboveMinFree {
		return false, u, nil
	}
	if approxBytes > 0 && u.FreeBytes-approxBytes < u.MinFreeBytes {
		return false, u, nil
	}
	return true, u, nil
}

// ReadyzStatus returns "ok" when the guard is healthy or a failure
// message when free space is below the min-free threshold. Meant to be
// embedded in the /readyz response.
func (g *Guard) ReadyzStatus() (string, error) {
	u, err := g.Usage()
	if err != nil {
		return "", err
	}
	if !u.AboveMinFree {
		return u.Message, nil
	}
	return "ok", nil
}

func formatBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
