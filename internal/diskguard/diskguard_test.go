package diskguard_test

import (
	"testing"

	"github.com/scout-kit/fine-print/internal/diskguard"
)

func TestUsage_ReturnsSensibleNumbersForTempDir(t *testing.T) {
	g := diskguard.New(t.TempDir(), 0) // default threshold
	u, err := g.Usage()
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if u.TotalBytes <= 0 {
		t.Error("total bytes should be positive")
	}
	if u.FreeBytes < 0 {
		t.Error("free bytes shouldn't be negative")
	}
	if u.UsedFraction < 0 || u.UsedFraction > 1 {
		t.Errorf("used fraction out of range: %v", u.UsedFraction)
	}
	if u.MinFreeBytes != diskguard.DefaultMinFreeBytes {
		t.Errorf("default min free: got %d, want %d", u.MinFreeBytes, diskguard.DefaultMinFreeBytes)
	}
}

func TestAllow_RefusesBelowMinFree(t *testing.T) {
	// Set the min-free to something absurdly large so the check fails on
	// any real filesystem.
	g := diskguard.New(t.TempDir(), 1<<62)
	ok, usage, err := g.Allow(0)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if ok {
		t.Error("should refuse when min-free exceeds total capacity")
	}
	if usage.AboveMinFree {
		t.Error("AboveMinFree should be false")
	}
	if usage.Message == "" {
		t.Error("blocked usage should have a user-facing message")
	}
}

func TestAllow_AccountsForApproxUploadSize(t *testing.T) {
	g := diskguard.New(t.TempDir(), 1) // effectively disabled floor
	// Ask for more bytes than exist on the volume.
	u, err := g.Usage()
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	ok, _, _ := g.Allow(u.FreeBytes + 1<<30)
	if ok {
		t.Error("should refuse when approxBytes would drop free below min")
	}
}

func TestSetMinFreeBytes_UpdatesFutureChecks(t *testing.T) {
	g := diskguard.New(t.TempDir(), 1<<62)
	if ok, _, _ := g.Allow(0); ok {
		t.Fatal("should start refused")
	}
	g.SetMinFreeBytes(1) // effectively disable
	if ok, _, _ := g.Allow(0); !ok {
		t.Error("should allow after threshold reduced")
	}
}

func TestReadyzStatus_OKWhenHealthy(t *testing.T) {
	g := diskguard.New(t.TempDir(), 1) // tiny floor
	s, err := g.ReadyzStatus()
	if err != nil {
		t.Fatalf("readyz: %v", err)
	}
	if s != "ok" {
		t.Errorf("got %q, want ok", s)
	}
}

func TestReadyzStatus_FailsBelowThreshold(t *testing.T) {
	g := diskguard.New(t.TempDir(), 1<<62)
	s, _ := g.ReadyzStatus()
	if s == "" || s == "ok" {
		t.Errorf("expected failure message, got %q", s)
	}
}
