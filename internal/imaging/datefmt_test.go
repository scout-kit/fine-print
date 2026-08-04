package imaging

import (
	"testing"
	"time"
)

// A fixed sample so expectations are stable: two-digit day distinct from the
// month, afternoon time so 12-hour presets show PM.
var sampleTime = time.Date(2026, time.March, 14, 14, 31, 0, 0, time.UTC)

func TestFormatDate_Presets(t *testing.T) {
	tests := []struct {
		format DateFormat
		source TextSource
		want   string
	}{
		{DateFormatLong, TextSourcePhotoDate, "March 14, 2026"},
		{DateFormatMedium, TextSourcePhotoDate, "Mar 14, 2026"},
		{DateFormatUS, TextSourcePhotoDate, "03/14/2026"},
		{DateFormatEU, TextSourcePhotoDate, "14/03/2026"},
		{DateFormatISO, TextSourcePhotoDate, "2026-03-14"},

		{DateTimeFormatLong, TextSourcePhotoDateTime, "March 14, 2026 at 2:31 PM"},
		{DateTimeFormatMedium, TextSourcePhotoDateTime, "Mar 14, 2026 2:31 PM"},
		{DateTimeFormatUS, TextSourcePhotoDateTime, "03/14/2026 2:31 PM"},
		{DateTimeFormatEU, TextSourcePhotoDateTime, "14/03/2026 14:31"},
		{DateTimeFormatISO, TextSourcePhotoDateTime, "2026-03-14 14:31"},
		{DateTimeFormatTime, TextSourcePhotoDateTime, "2:31 PM"},
	}
	for _, tc := range tests {
		if got := FormatDate(sampleTime, tc.source, tc.format); got != tc.want {
			t.Errorf("FormatDate(%q) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

// A stored format that no longer exists must not leak a Go layout string onto
// a print — it falls back to the source's default.
func TestFormatDate_UnknownFormatFallsBackPerSource(t *testing.T) {
	if got := FormatDate(sampleTime, TextSourcePhotoDate, "nonsense"); got != "March 14, 2026" {
		t.Errorf("date fallback = %q, want the long date preset", got)
	}
	if got := FormatDate(sampleTime, TextSourcePhotoDateTime, "nonsense"); got != "March 14, 2026 at 2:31 PM" {
		t.Errorf("datetime fallback = %q, want the long datetime preset", got)
	}
	if got := FormatDate(sampleTime, TextSourcePhotoDate, ""); got != "March 14, 2026" {
		t.Errorf("empty format = %q, want the long date preset", got)
	}
}

func TestResolveTextContent(t *testing.T) {
	tests := []struct {
		name   string
		static string
		source TextSource
		format DateFormat
		at     time.Time
		want   string
	}{
		{"static passes through", "Smile!", TextSourceStatic, "", sampleTime, "Smile!"},
		{"static ignores format", "Smile!", TextSourceStatic, DateFormatISO, sampleTime, "Smile!"},
		{"date replaces text", "unused", TextSourcePhotoDate, DateFormatISO, sampleTime, "2026-03-14"},
		{"datetime replaces text", "unused", TextSourcePhotoDateTime, DateTimeFormatISO, sampleTime, "2026-03-14 14:31"},
		// A date source with no timestamp yields empty so the renderer skips
		// it rather than drawing a stray label.
		{"date with zero time is empty", "unused", TextSourcePhotoDate, DateFormatISO, time.Time{}, ""},
		// An empty static overlay is also skipped.
		{"empty static is empty", "", TextSourceStatic, "", sampleTime, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveTextContent(tc.static, tc.source, tc.format, tc.at); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTextSource_Validity(t *testing.T) {
	for _, s := range []TextSource{TextSourceStatic, TextSourcePhotoDate, TextSourcePhotoDateTime} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []TextSource{"", "photo_year", "PHOTO_DATE", "date"} {
		if TextSource(s).Valid() {
			t.Errorf("%q should not be valid", s)
		}
	}
	if TextSourceStatic.IsDateSource() {
		t.Error("static is not a date source")
	}
	if !TextSourcePhotoDate.IsDateSource() || !TextSourcePhotoDateTime.IsDateSource() {
		t.Error("photo_date and photo_datetime are date sources")
	}
}

// Every preset offered for a source must agree with that source about whether
// it renders a time, or the UI can offer a format the API then rejects.
func TestDateFormatsFor_MatchSourceTimeExpectation(t *testing.T) {
	for _, f := range DateFormatsFor(TextSourcePhotoDate) {
		if !f.Valid() {
			t.Errorf("date preset %q is not a known layout", f)
		}
		if f.IncludesTime() {
			t.Errorf("date preset %q renders a time but is offered for photo_date", f)
		}
	}
	for _, f := range DateFormatsFor(TextSourcePhotoDateTime) {
		if !f.Valid() {
			t.Errorf("datetime preset %q is not a known layout", f)
		}
		if !f.IncludesTime() {
			t.Errorf("datetime preset %q renders no time but is offered for photo_datetime", f)
		}
	}
	if got := DateFormatsFor(TextSourceStatic); got != nil {
		t.Errorf("static source should offer no presets, got %v", got)
	}
}

// The defaults must themselves be legal for their source.
func TestDefaultFormatsAreValidForTheirSource(t *testing.T) {
	if !DefaultDateFormat.Valid() || DefaultDateFormat.IncludesTime() {
		t.Errorf("DefaultDateFormat %q must be a valid date-only preset", DefaultDateFormat)
	}
	if !DefaultDateTimeFormat.Valid() || !DefaultDateTimeFormat.IncludesTime() {
		t.Errorf("DefaultDateTimeFormat %q must be a valid time-bearing preset", DefaultDateTimeFormat)
	}
}
