package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- EXIF fixture builder ----------------------------------------------------
//
// Rather than commit a binary fixture, the tests synthesize a JPEG carrying a
// real little-endian TIFF/EXIF block. That keeps the expected bytes visible
// and lets each test vary a single tag.

const (
	tagIDMake             = 0x010F
	tagIDModel            = 0x0110
	tagIDExifIFDPointer   = 0x8769
	tagIDDateTimeOriginal = 0x9003
	tagIDOffsetTimeOrig   = 0x9011

	typeASCII = 2
	typeLONG  = 4
)

// ifdEntry is one 12-byte IFD record awaiting offset resolution.
type ifdEntry struct {
	tag     uint16
	typ     uint16
	count   uint32
	inline  uint32 // used when the value fits in 4 bytes
	data    []byte // used when it doesn't; offset is patched in
	hasData bool
}

func asciiEntry(tag uint16, value string) ifdEntry {
	b := append([]byte(value), 0) // ASCII values are NUL-terminated
	e := ifdEntry{tag: tag, typ: typeASCII, count: uint32(len(b))}
	if len(b) <= 4 {
		var buf [4]byte
		copy(buf[:], b)
		e.inline = binary.LittleEndian.Uint32(buf[:])
		return e
	}
	e.data = b
	e.hasData = true
	return e
}

// buildIFD lays out an IFD at ifdOffset, returning the IFD bytes (including
// the trailing next-IFD pointer) plus the out-of-line data blob that follows.
func buildIFD(ifdOffset uint32, entries []ifdEntry, nextIFD uint32) []byte {
	// Entries are sorted by tag, as the TIFF spec requires.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j-1].tag > entries[j].tag; j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}

	ifdLen := uint32(2 + 12*len(entries) + 4)
	dataStart := ifdOffset + ifdLen

	var ifd bytes.Buffer
	var data bytes.Buffer
	binary.Write(&ifd, binary.LittleEndian, uint16(len(entries)))
	for _, e := range entries {
		binary.Write(&ifd, binary.LittleEndian, e.tag)
		binary.Write(&ifd, binary.LittleEndian, e.typ)
		binary.Write(&ifd, binary.LittleEndian, e.count)
		if e.hasData {
			binary.Write(&ifd, binary.LittleEndian, dataStart+uint32(data.Len()))
			data.Write(e.data)
		} else {
			binary.Write(&ifd, binary.LittleEndian, e.inline)
		}
	}
	binary.Write(&ifd, binary.LittleEndian, nextIFD)

	return append(ifd.Bytes(), data.Bytes()...)
}

// exifFixture describes the tags to embed. Empty fields are omitted entirely,
// which is how real files with missing metadata behave.
type exifFixture struct {
	make             string
	model            string
	dateTimeOriginal string
	offsetTime       string
}

// buildTIFF assembles a complete little-endian TIFF block: IFD0 with camera
// identity plus a pointer to an ExifIFD holding the timestamp tags.
func buildTIFF(f exifFixture) []byte {
	var exifEntries []ifdEntry
	if f.dateTimeOriginal != "" {
		exifEntries = append(exifEntries, asciiEntry(tagIDDateTimeOriginal, f.dateTimeOriginal))
	}
	if f.offsetTime != "" {
		exifEntries = append(exifEntries, asciiEntry(tagIDOffsetTimeOrig, f.offsetTime))
	}

	var ifd0Entries []ifdEntry
	if f.make != "" {
		ifd0Entries = append(ifd0Entries, asciiEntry(tagIDMake, f.make))
	}
	if f.model != "" {
		ifd0Entries = append(ifd0Entries, asciiEntry(tagIDModel, f.model))
	}

	const headerLen = 8 // "II", 0x002A, IFD0 offset
	const ifd0Offset = headerLen

	// The ExifIFD pointer needs the ExifIFD's offset, which depends on IFD0's
	// own length — so compute IFD0's size with the pointer entry included.
	entryCount := len(ifd0Entries)
	if len(exifEntries) > 0 {
		entryCount++
	}
	ifd0Len := uint32(2 + 12*entryCount + 4)
	// Out-of-line data for IFD0's ASCII values sits between IFD0 and ExifIFD.
	var ifd0DataLen uint32
	for _, e := range ifd0Entries {
		if e.hasData {
			ifd0DataLen += uint32(len(e.data))
		}
	}
	exifIFDOffset := ifd0Offset + ifd0Len + ifd0DataLen

	if len(exifEntries) > 0 {
		ifd0Entries = append(ifd0Entries, ifdEntry{
			tag: tagIDExifIFDPointer, typ: typeLONG, count: 1, inline: exifIFDOffset,
		})
	}

	var buf bytes.Buffer
	buf.WriteString("II")
	binary.Write(&buf, binary.LittleEndian, uint16(0x002A))
	binary.Write(&buf, binary.LittleEndian, uint32(ifd0Offset))
	buf.Write(buildIFD(ifd0Offset, ifd0Entries, 0))
	if len(exifEntries) > 0 {
		buf.Write(buildIFD(exifIFDOffset, exifEntries, 0))
	}
	return buf.Bytes()
}

// writeJPEGWithEXIF writes a minimal JPEG whose APP1 segment carries the EXIF
// block, and returns its path.
func writeJPEGWithEXIF(t *testing.T, f exifFixture) string {
	t.Helper()

	tiff := buildTIFF(f)
	payload := append([]byte("Exif\x00\x00"), tiff...)

	var jpeg bytes.Buffer
	jpeg.Write([]byte{0xFF, 0xD8}) // SOI
	jpeg.Write([]byte{0xFF, 0xE1}) // APP1
	// Segment length covers the length field itself.
	binary.Write(&jpeg, binary.BigEndian, uint16(len(payload)+2))
	jpeg.Write(payload)
	jpeg.Write([]byte{0xFF, 0xD9}) // EOI

	path := filepath.Join(t.TempDir(), "fixture.jpg")
	if err := os.WriteFile(path, jpeg.Bytes(), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// --- Tests -------------------------------------------------------------------

func TestReadMetadata_ExtractsCaptureTimeAndCamera(t *testing.T) {
	path := writeJPEGWithEXIF(t, exifFixture{
		make:             "Canon",
		model:            "Canon EOS R6",
		dateTimeOriginal: "2026:03:14 14:31:07",
	})

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if !md.HasTakenAt() {
		t.Fatal("expected a capture timestamp")
	}
	want := time.Date(2026, time.March, 14, 14, 31, 7, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("TakenAt = %s, want %s", md.TakenAt, want)
	}
	if md.CameraMake != "Canon" {
		t.Errorf("CameraMake = %q, want Canon", md.CameraMake)
	}
	if md.CameraModel != "Canon EOS R6" {
		t.Errorf("CameraModel = %q, want Canon EOS R6", md.CameraModel)
	}
}

// EXIF timestamps are wall-clock readings with no zone. They must be returned
// verbatim, not shifted into the server's timezone, or a photo taken at 2:31 PM
// prints some other time depending on where the kiosk is running.
func TestReadMetadata_DoesNotShiftWallClockTime(t *testing.T) {
	path := writeJPEGWithEXIF(t, exifFixture{dateTimeOriginal: "2026:03:14 14:31:07"})

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if h, m := md.TakenAt.Hour(), md.TakenAt.Minute(); h != 14 || m != 31 {
		t.Errorf("wall clock = %02d:%02d, want 14:31", h, m)
	}
}

// A file with no EXIF at all is an ordinary case, reported distinctly so the
// caller can stay quiet about it.
func TestReadMetadata_NoEXIFReportsSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.jpg")
	// Minimal JPEG with no APP1 segment.
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	_, err := ReadMetadata(path)
	if !errors.Is(err, ErrNoEXIF) {
		t.Fatalf("err = %v, want ErrNoEXIF", err)
	}
}

// Camera identity without a timestamp should still come through, so the
// metadata panel can show the camera even when the date falls back.
func TestReadMetadata_CameraWithoutTimestamp(t *testing.T) {
	path := writeJPEGWithEXIF(t, exifFixture{make: "Apple", model: "iPhone 15 Pro"})

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if md.HasTakenAt() {
		t.Errorf("expected no timestamp, got %s", md.TakenAt)
	}
	if md.CameraMake != "Apple" || md.CameraModel != "iPhone 15 Pro" {
		t.Errorf("camera = %q / %q, want Apple / iPhone 15 Pro", md.CameraMake, md.CameraModel)
	}
}

// Cameras with an unset clock write all-zero placeholders. Those must be
// rejected so the photo falls back to upload time instead of printing year 0.
func TestReadMetadata_RejectsPlaceholderTimestamp(t *testing.T) {
	path := writeJPEGWithEXIF(t, exifFixture{dateTimeOriginal: "0000:00:00 00:00:00"})

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if md.HasTakenAt() {
		t.Errorf("placeholder timestamp should be rejected, got %s", md.TakenAt)
	}
}

func TestReadMetadata_MissingFileErrors(t *testing.T) {
	if _, err := ReadMetadata(filepath.Join(t.TempDir(), "nope.jpg")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestParseEXIFTime_Layouts(t *testing.T) {
	want := time.Date(2026, time.March, 14, 14, 31, 7, 0, time.UTC)
	for _, raw := range []string{
		"2026:03:14 14:31:07",
		"2026-03-14 14:31:07",
		"2026:03:14T14:31:07",
	} {
		got, err := parseEXIFTime(raw, "")
		if err != nil {
			t.Errorf("parseEXIFTime(%q) error: %v", raw, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("parseEXIFTime(%q) = %s, want %s", raw, got, want)
		}
	}

	// Seconds are optional on some encoders.
	got, err := parseEXIFTime("2026:03:14 14:31", "")
	if err != nil {
		t.Fatalf("minute-precision timestamp: %v", err)
	}
	if got.Second() != 0 || got.Minute() != 31 {
		t.Errorf("got %s, want 14:31:00", got)
	}
}

// An OffsetTime records the zone but must not move the wall clock.
func TestParseEXIFTime_OffsetRecordsZoneWithoutShifting(t *testing.T) {
	got, err := parseEXIFTime("2026:03:14 14:31:07", "-05:00")
	if err != nil {
		t.Fatalf("parseEXIFTime: %v", err)
	}
	if h, m := got.Hour(), got.Minute(); h != 14 || m != 31 {
		t.Errorf("wall clock = %02d:%02d, want 14:31", h, m)
	}
	if _, offset := got.Zone(); offset != -5*3600 {
		t.Errorf("zone offset = %d, want %d", offset, -5*3600)
	}
}

func TestParseEXIFTime_Rejects(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a date", "0000:00:00 00:00:00", "\x00\x00\x00"} {
		if _, err := parseEXIFTime(raw, ""); err == nil {
			t.Errorf("parseEXIFTime(%q) should have failed", raw)
		}
	}
}

func TestCleanEXIFString_TrimsNULPadding(t *testing.T) {
	if got := cleanEXIFString("Canon\x00\x00"); got != "Canon" {
		t.Errorf("got %q, want Canon", got)
	}
	if got := cleanEXIFString("  NIKON  "); got != "NIKON" {
		t.Errorf("got %q, want NIKON", got)
	}
}
