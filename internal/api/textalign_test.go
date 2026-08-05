package api

import (
	"testing"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
)

// An omitted alignment must default to left, so overlays created before this
// field existed — and clients that never send it — keep their old behavior.
func TestNormalizeTextAlign_DefaultsToLeft(t *testing.T) {
	got, err := normalizeTextAlign("")
	if err != nil {
		t.Fatalf("empty align: %v", err)
	}
	if got != db.TextAlignLeft {
		t.Errorf("align = %q, want %q", got, db.TextAlignLeft)
	}
}

func TestNormalizeTextAlign_AcceptsEveryAnchor(t *testing.T) {
	for _, want := range []string{db.TextAlignLeft, db.TextAlignCenter, db.TextAlignRight} {
		got, err := normalizeTextAlign(want)
		if err != nil {
			t.Errorf("%q rejected: %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("align %q stored as %q", want, got)
		}
	}
}

func TestNormalizeTextAlign_RejectsUnknown(t *testing.T) {
	for _, bad := range []string{"Left", "RIGHT", "Center", "start", "end", "justify", "middle"} {
		if _, err := normalizeTextAlign(bad); err == nil {
			t.Errorf("align %q should have been rejected", bad)
		}
	}
}

// The db constants and the imaging enum have to agree, since the API validates
// with one and the renderer switches on the other.
func TestTextAlignConstantsMatchImaging(t *testing.T) {
	if db.TextAlignLeft != string(imaging.TextAlignLeft) {
		t.Errorf("db %q != imaging %q", db.TextAlignLeft, imaging.TextAlignLeft)
	}
	if db.TextAlignCenter != string(imaging.TextAlignCenter) {
		t.Errorf("db %q != imaging %q", db.TextAlignCenter, imaging.TextAlignCenter)
	}
	if db.TextAlignRight != string(imaging.TextAlignRight) {
		t.Errorf("db %q != imaging %q", db.TextAlignRight, imaging.TextAlignRight)
	}
}
