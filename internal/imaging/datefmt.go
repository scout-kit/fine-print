package imaging

import "time"

// TextSource says where a text overlay's printed content comes from.
type TextSource string

const (
	// TextSourceStatic prints the overlay's stored literal text.
	TextSourceStatic TextSource = "static"
	// TextSourcePhotoDate prints the photo's capture date.
	TextSourcePhotoDate TextSource = "photo_date"
	// TextSourcePhotoDateTime prints the photo's capture date and time.
	TextSourcePhotoDateTime TextSource = "photo_datetime"
)

// IsDateSource reports whether a source is derived from the photo timestamp
// rather than stored literal text.
func (s TextSource) IsDateSource() bool {
	return s == TextSourcePhotoDate || s == TextSourcePhotoDateTime
}

// Valid reports whether s is a source the renderer understands.
func (s TextSource) Valid() bool {
	switch s {
	case TextSourceStatic, TextSourcePhotoDate, TextSourcePhotoDateTime:
		return true
	}
	return false
}

// DateFormat is a named formatting preset. Presets are used rather than
// raw Go layouts (or strftime patterns) so the admin UI can offer a picker
// with live examples, and so a stored value can never render as the literal
// reference date after a typo.
type DateFormat string

// Date-only presets.
const (
	DateFormatLong   DateFormat = "long"   // January 2, 2006
	DateFormatMedium DateFormat = "medium" // Jan 2, 2006
	DateFormatUS     DateFormat = "us"     // 01/02/2006
	DateFormatEU     DateFormat = "eu"     // 02/01/2006
	DateFormatISO    DateFormat = "iso"    // 2006-01-02
)

// Date+time presets.
const (
	DateTimeFormatLong   DateFormat = "datetime_long"   // January 2, 2006 at 3:04 PM
	DateTimeFormatMedium DateFormat = "datetime_medium" // Jan 2, 2006 3:04 PM
	DateTimeFormatUS     DateFormat = "datetime_us"     // 01/02/2006 3:04 PM
	DateTimeFormatEU     DateFormat = "datetime_eu"     // 02/01/2006 15:04
	DateTimeFormatISO    DateFormat = "datetime_iso"    // 2006-01-02 15:04
	DateTimeFormatTime   DateFormat = "time_only"       // 3:04 PM
)

// dateLayouts maps each preset to its Go reference layout.
var dateLayouts = map[DateFormat]string{
	DateFormatLong:   "January 2, 2006",
	DateFormatMedium: "Jan 2, 2006",
	DateFormatUS:     "01/02/2006",
	DateFormatEU:     "02/01/2006",
	DateFormatISO:    "2006-01-02",

	DateTimeFormatLong:   "January 2, 2006 at 3:04 PM",
	DateTimeFormatMedium: "Jan 2, 2006 3:04 PM",
	DateTimeFormatUS:     "01/02/2006 3:04 PM",
	DateTimeFormatEU:     "02/01/2006 15:04",
	DateTimeFormatISO:    "2006-01-02 15:04",
	DateTimeFormatTime:   "3:04 PM",
}

// DefaultDateFormat is used when a date overlay has no format stored.
const DefaultDateFormat = DateFormatLong

// DefaultDateTimeFormat is used when a date+time overlay has no format stored.
const DefaultDateTimeFormat = DateTimeFormatLong

// Valid reports whether f names a known preset.
func (f DateFormat) Valid() bool {
	_, ok := dateLayouts[f]
	return ok
}

// IncludesTime reports whether the preset renders a time component. Used by
// the admin UI to keep the format list consistent with the chosen source.
func (f DateFormat) IncludesTime() bool {
	switch f {
	case DateTimeFormatLong, DateTimeFormatMedium, DateTimeFormatUS,
		DateTimeFormatEU, DateTimeFormatISO, DateTimeFormatTime:
		return true
	}
	return false
}

// DateFormatsFor returns the presets appropriate to a source, in display
// order. Returns nil for static sources.
func DateFormatsFor(source TextSource) []DateFormat {
	switch source {
	case TextSourcePhotoDate:
		return []DateFormat{
			DateFormatLong, DateFormatMedium, DateFormatUS, DateFormatEU, DateFormatISO,
		}
	case TextSourcePhotoDateTime:
		return []DateFormat{
			DateTimeFormatLong, DateTimeFormatMedium, DateTimeFormatUS,
			DateTimeFormatEU, DateTimeFormatISO, DateTimeFormatTime,
		}
	}
	return nil
}

// FormatDate renders t using the named preset. An unknown or empty preset
// falls back to the default for the source so a malformed stored value still
// prints a sensible date rather than a Go layout string.
func FormatDate(t time.Time, source TextSource, format DateFormat) string {
	layout, ok := dateLayouts[format]
	if !ok {
		def := DefaultDateFormat
		if source == TextSourcePhotoDateTime {
			def = DefaultDateTimeFormat
		}
		layout = dateLayouts[def]
	}
	return t.Format(layout)
}

// ResolveTextContent produces the string a text overlay should print.
// takenAt is the photo's effective timestamp — the caller has already
// resolved EXIF-vs-upload-time. A date source with a zero timestamp yields
// an empty string, which the renderer skips.
func ResolveTextContent(staticText string, source TextSource, format DateFormat, takenAt time.Time) string {
	if !source.IsDateSource() {
		return staticText
	}
	if takenAt.IsZero() {
		return ""
	}
	return FormatDate(takenAt, source, format)
}
