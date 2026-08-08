package api

import (
	"testing"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
)

func TestNormalizeTextSource_Defaults(t *testing.T) {
	// An omitted source means the pre-existing behavior: literal text.
	source, format, err := normalizeTextSource("", "")
	if err != nil {
		t.Fatalf("empty source: %v", err)
	}
	if source != db.TextSourceStatic {
		t.Errorf("source = %q, want %q", source, db.TextSourceStatic)
	}
	if format.Valid {
		t.Errorf("static overlay should store a NULL date_format, got %q", format.String)
	}
}

// A static overlay must not retain a date format — it would be dead data that
// reappears confusingly if the source is later switched.
func TestNormalizeTextSource_StaticDropsFormat(t *testing.T) {
	_, format, err := normalizeTextSource(db.TextSourceStatic, string(imaging.DateFormatISO))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format.Valid {
		t.Errorf("expected NULL date_format, got %q", format.String)
	}
}

// A date source with no format is legal — the renderer applies the default.
func TestNormalizeTextSource_DateSourceAllowsEmptyFormat(t *testing.T) {
	source, format, err := normalizeTextSource(db.TextSourcePhotoDate, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != db.TextSourcePhotoDate {
		t.Errorf("source = %q, want %q", source, db.TextSourcePhotoDate)
	}
	if format.Valid {
		t.Errorf("expected NULL date_format, got %q", format.String)
	}
}

func TestNormalizeTextSource_AcceptsMatchingFormats(t *testing.T) {
	for _, f := range imaging.DateFormatsFor(imaging.TextSourcePhotoDate) {
		if _, got, err := normalizeTextSource(db.TextSourcePhotoDate, string(f)); err != nil {
			t.Errorf("photo_date + %q rejected: %v", f, err)
		} else if got.String != string(f) {
			t.Errorf("photo_date + %q stored as %q", f, got.String)
		}
	}
	for _, f := range imaging.DateFormatsFor(imaging.TextSourcePhotoDateTime) {
		if _, got, err := normalizeTextSource(db.TextSourcePhotoDateTime, string(f)); err != nil {
			t.Errorf("photo_datetime + %q rejected: %v", f, err)
		} else if got.String != string(f) {
			t.Errorf("photo_datetime + %q stored as %q", f, got.String)
		}
	}
}

// Crossing a source with the other family's preset would silently print
// something the admin didn't choose, so it's rejected rather than coerced.
func TestNormalizeTextSource_RejectsMismatchedFormats(t *testing.T) {
	if _, _, err := normalizeTextSource(db.TextSourcePhotoDate, string(imaging.DateTimeFormatISO)); err == nil {
		t.Error("photo_date with a time-bearing preset should be rejected")
	}
	if _, _, err := normalizeTextSource(db.TextSourcePhotoDateTime, string(imaging.DateFormatISO)); err == nil {
		t.Error("photo_datetime with a date-only preset should be rejected")
	}
}

func TestNormalizeTextSource_RejectsUnknownValues(t *testing.T) {
	if _, _, err := normalizeTextSource("photo_year", ""); err == nil {
		t.Error("unknown source should be rejected")
	}
	if _, _, err := normalizeTextSource(db.TextSourcePhotoDate, "yyyy-mm-dd"); err == nil {
		t.Error("unknown date_format should be rejected")
	}
}
