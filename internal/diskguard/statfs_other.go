//go:build !unix

package diskguard

import "fmt"

// statfs is unimplemented outside Unix. Callers surface the error, which
// leaves /readyz reporting the disk check as failed rather than silently
// pretending there's space — matching the hotspot package's stub approach
// for platforms the kiosk doesn't target yet.
func statfs(path string) (total, free int64, err error) {
	return 0, 0, fmt.Errorf("disk usage checks are not implemented on this platform")
}
