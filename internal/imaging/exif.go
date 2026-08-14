package imaging

import (
	"errors"
	"fmt"
	"strings"
	"time"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// DateSource records where a photo's timestamp came from. It's surfaced in
// the UI so a printed date is never a silent guess.
type DateSource string

const (
	// DateSourceEXIF means the camera recorded a capture time.
	DateSourceEXIF DateSource = "exif"
	// DateSourceIPTC means the EXIF timestamp was absent and the date came
	// from an IPTC DateCreated — what a desktop editor leaves behind when it
	// strips EXIF on export.
	DateSourceIPTC DateSource = "iptc"
	// DateSourceUpload means no timestamp was available in the file and the
	// caller should fall back to the upload time.
	DateSourceUpload DateSource = "upload"
)

// Metadata is the subset of a file's metadata the kiosk cares about.
type Metadata struct {
	// TakenAt is the capture time, in the camera's local wall-clock time.
	// Zero when the file carried no usable timestamp.
	TakenAt time.Time
	// TakenAtSource says which of the file's timestamps TakenAt came from.
	// Empty when there was none.
	TakenAtSource DateSource
	// CameraMake and CameraModel are empty when absent.
	CameraMake  string
	CameraModel string
}

// HasTakenAt reports whether a capture timestamp was found.
func (m Metadata) HasTakenAt() bool { return !m.TakenAt.IsZero() }

// EXIF tag IDs we read. DateTimeOriginal is the capture moment;
// DateTimeDigitized and DateTime are progressively weaker fallbacks that
// some phones and scanners populate instead.
const (
	tagDateTimeOriginal  = "DateTimeOriginal"
	tagDateTimeDigitized = "DateTimeDigitized"
	tagDateTime          = "DateTime"
	tagOffsetTimeOrig    = "OffsetTimeOriginal"
	tagMake              = "Make"
	tagModel             = "Model"
)

// ErrNoEXIF reports that the file carried no EXIF block at all. Callers
// generally treat this the same as "no timestamp" — it's separated only so
// logging can distinguish "not a camera file" from "malformed EXIF".
var ErrNoEXIF = errors.New("no exif data")

// ReadMetadata extracts capture time and camera identity from the image at
// path. A file with no EXIF is not an error condition for the caller's
// purposes, but is reported as ErrNoEXIF so it can be logged distinctly.
//
// EXIF timestamps carry no timezone. They are wall-clock readings from the
// camera, which is exactly what should be printed ("taken at 2:31 PM"), so
// they are returned as-is in UTC rather than being shifted into the server's
// zone. When the file also carries OffsetTimeOriginal we keep the wall-clock
// reading and only record the zone, so formatting stays stable either way.
func ReadMetadata(path string) (Metadata, error) {
	rawExif, err := exif.SearchFileAndExtractExif(path)
	if err != nil {
		// The library returns its own sentinel for "scanned the whole file,
		// found no EXIF marker". Such a file may still carry an IPTC date —
		// desktop editors strip EXIF and keep IPTC — so try that before
		// giving up.
		if errors.Is(err, exif.ErrNoExif) {
			if t, iptcErr := readIPTCTakenAt(path); iptcErr == nil {
				return Metadata{TakenAt: t, TakenAtSource: DateSourceIPTC}, nil
			}
			return Metadata{}, ErrNoEXIF
		}
		return Metadata{}, fmt.Errorf("extracting exif from %s: %w", path, err)
	}

	entries, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return Metadata{}, fmt.Errorf("parsing exif from %s: %w", path, err)
	}

	// Collect the tags of interest in one pass. Later IFDs can repeat a tag
	// name; first value wins, which favours IFD0/ExifIFD over thumbnail IFDs.
	found := map[string]string{}
	for _, e := range entries {
		switch e.TagName {
		case tagDateTimeOriginal, tagDateTimeDigitized, tagDateTime,
			tagOffsetTimeOrig, tagMake, tagModel:
			if _, seen := found[e.TagName]; seen {
				continue
			}
			if s, ok := exifString(e); ok {
				found[e.TagName] = s
			}
		}
	}

	md := Metadata{
		CameraMake:  cleanEXIFString(found[tagMake]),
		CameraModel: cleanEXIFString(found[tagModel]),
	}

	// Strongest timestamp first.
	for _, tag := range []string{tagDateTimeOriginal, tagDateTimeDigitized, tagDateTime} {
		raw := found[tag]
		if raw == "" {
			continue
		}
		if t, err := parseEXIFTime(raw, found[tagOffsetTimeOrig]); err == nil {
			md.TakenAt = t
			md.TakenAtSource = DateSourceEXIF
			break
		}
	}

	// An EXIF block is often present but dateless on an exported file — it
	// keeps only the pixel dimensions. IPTC is where the editor left the date.
	if !md.HasTakenAt() {
		if t, err := readIPTCTakenAt(path); err == nil {
			md.TakenAt = t
			md.TakenAtSource = DateSourceIPTC
		}
	}

	return md, nil
}

// exifString renders a flat EXIF entry's value as a string when it is
// textual. Numeric and binary tags are skipped — every tag we read is ASCII.
func exifString(e exif.ExifTag) (string, bool) {
	if e.Value == nil {
		return "", false
	}
	if s, ok := e.Value.(string); ok {
		return s, true
	}
	// Undefined-type tags surface as byte slices on some encoders.
	if b, ok := e.Value.([]byte); ok {
		return string(b), true
	}
	if e.TagTypeId == exifcommon.TypeAscii || e.TagTypeId == exifcommon.TypeAsciiNoNul {
		return e.Formatted, true
	}
	return "", false
}

// cleanEXIFString trims the NUL padding and stray whitespace that cameras
// leave in fixed-width ASCII fields.
func cleanEXIFString(s string) string {
	return strings.TrimSpace(strings.TrimRight(s, "\x00"))
}

// exifTimeLayouts covers the standard form plus the variants seen in the
// wild: some encoders use dashes in the date, others omit seconds.
var exifTimeLayouts = []string{
	"2006:01:02 15:04:05",
	"2006-01-02 15:04:05",
	"2006:01:02 15:04",
	"2006-01-02 15:04",
	"2006:01:02T15:04:05",
	"2006-01-02T15:04:05",
}

// parseEXIFTime parses an EXIF datetime string. offset, when present, is an
// EXIF OffsetTime value like "-05:00"; it is recorded as the location so the
// zone is not lost, but the wall-clock reading is never shifted.
func parseEXIFTime(raw, offset string) (time.Time, error) {
	s := cleanEXIFString(raw)
	if s == "" {
		return time.Time{}, errors.New("empty exif timestamp")
	}
	// Cameras with an unset clock write all-zero placeholders.
	if strings.HasPrefix(s, "0000") || strings.TrimLeft(s, "0:- ") == "" {
		return time.Time{}, errors.New("placeholder exif timestamp")
	}

	loc := time.UTC
	if z, err := parseEXIFOffset(offset); err == nil {
		loc = z
	}

	for _, layout := range exifTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized exif timestamp %q", s)
}

// parseEXIFOffset turns an EXIF OffsetTime value ("+09:00", "-05:00") into a
// fixed zone.
func parseEXIFOffset(offset string) (*time.Location, error) {
	s := cleanEXIFString(offset)
	if s == "" {
		return nil, errors.New("no offset")
	}
	t, err := time.Parse("-07:00", s)
	if err != nil {
		return nil, err
	}
	_, secs := t.Zone()
	return time.FixedZone(s, secs), nil
}
