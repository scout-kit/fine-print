package printer_test

import (
	"context"
	"testing"
	"time"
)

// timeoutCtx returns a context that auto-cancels after d. Centralises
// the boilerplate for short-lived monitor/queue tests.
func timeoutCtx(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}
