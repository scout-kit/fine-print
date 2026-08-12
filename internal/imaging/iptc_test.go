package imaging

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- IPTC fixture builder ----------------------------------------------------
//
// Photos exported from a desktop editor lose their EXIF but keep an IPTC
// DateCreated. The fixtures below assemble that structure — APP13 → 8BIM
// resource 0x0404 → IIM datasets — so the expected bytes stay visible.

// iptcFixture lists the application-record datasets to embed, by number.
type iptcFixture map[byte]string

func buildIPTCBlock(f iptcFixture) []byte {
	var block bytes.Buffer
	// Datasets are written in ascending order, as writers emit them.
	for dataset := 0; dataset < 256; dataset++ {
		value, ok := f[byte(dataset)]
		if !ok {
			continue
		}
		block.WriteByte(0x1C)
		block.WriteByte(iptcRecordApplication)
		block.WriteByte(byte(dataset))
		binary.Write(&block, binary.BigEndian, uint16(len(value)))
		block.WriteString(value)
	}
	return block.Bytes()
}

// buildAPP13 wraps an IPTC block in the Photoshop resource container.
func buildAPP13(iptc []byte) []byte {
	var res bytes.Buffer
	res.WriteString("8BIM")
	binary.Write(&res, binary.BigEndian, uint16(photoshopResourceIPTC))
	res.WriteByte(0) // empty Pascal name...
	res.WriteByte(0) // ...padded to an even length
	binary.Write(&res, binary.BigEndian, uint32(len(iptc)))
	res.Write(iptc)
	if len(iptc)%2 != 0 {
		res.WriteByte(0)
	}

	return append([]byte(photoshopSegmentHeader), res.Bytes()...)
}

// writeJPEG assembles a minimal JPEG from the given segments, each a
// (marker, payload) pair, and returns its path.
func writeJPEG(t *testing.T, segments ...[]byte) string {
	t.Helper()

	var jpeg bytes.Buffer
	jpeg.Write([]byte{0xFF, 0xD8}) // SOI
	for i := 0; i+1 < len(segments); i += 2 {
		jpeg.Write([]byte{0xFF, segments[i][0]})
		binary.Write(&jpeg, binary.BigEndian, uint16(len(segments[i+1])+2))
		jpeg.Write(segments[i+1])
	}
	jpeg.Write([]byte{0xFF, 0xD9}) // EOI

	path := filepath.Join(t.TempDir(), "fixture.jpg")
	if err := os.WriteFile(path, jpeg.Bytes(), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func app13(iptc iptcFixture) [][]byte {
	return [][]byte{{0xED}, buildAPP13(buildIPTCBlock(iptc))}
}

func app1EXIF(f exifFixture) [][]byte {
	return [][]byte{{0xE1}, append([]byte("Exif\x00\x00"), buildTIFF(f)...)}
}

func flatten(groups ...[][]byte) [][]byte {
	var out [][]byte
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// --- Tests -------------------------------------------------------------------

// The case that sent us here: a photo exported from a desktop editor, with no
// EXIF at all, whose capture date survives only in IPTC.
func TestReadMetadata_FallsBackToIPTCDate(t *testing.T) {
	path := writeJPEG(t, app13(iptcFixture{
		iptcDateCreated: "20220822",
		iptcTimeCreated: "190006",
	})...)

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	want := time.Date(2022, 8, 22, 19, 0, 6, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}
}

// An EXIF block that carries no timestamp — an export keeps the pixel
// dimensions and drops everything else — must not stop the IPTC lookup.
func TestReadMetadata_DatelessEXIFStillFallsBackToIPTC(t *testing.T) {
	path := writeJPEG(t, flatten(
		app1EXIF(exifFixture{make: "Nikon"}),
		app13(iptcFixture{iptcDateCreated: "20221015", iptcTimeCreated: "081500"}),
	)...)

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	want := time.Date(2022, 10, 15, 8, 15, 0, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}
	if md.CameraMake != "Nikon" {
		t.Errorf("cameraMake = %q, want %q — EXIF identity should survive", md.CameraMake, "Nikon")
	}
}

// EXIF is the camera's own record, so it outranks whatever an editor wrote.
func TestReadMetadata_EXIFDateWinsOverIPTC(t *testing.T) {
	path := writeJPEG(t, flatten(
		app1EXIF(exifFixture{dateTimeOriginal: "2024:03:14 14:31:00"}),
		app13(iptcFixture{iptcDateCreated: "20220822", iptcTimeCreated: "190006"}),
	)...)

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	want := time.Date(2024, 3, 14, 14, 31, 0, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}
}

// A date with no companion time is a date, not a reason to discard it.
func TestReadMetadata_IPTCDateWithoutTimeIsMidnight(t *testing.T) {
	path := writeJPEG(t, app13(iptcFixture{iptcDateCreated: "20220822"})...)

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	want := time.Date(2022, 8, 22, 0, 0, 0, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}
}

// IPTC times may carry a zone suffix. Like EXIF, the wall-clock reading is
// what gets printed, so the suffix is dropped rather than applied.
func TestReadMetadata_IPTCTimeZoneSuffixDoesNotShiftTheClock(t *testing.T) {
	path := writeJPEG(t, app13(iptcFixture{
		iptcDateCreated: "20220822",
		iptcTimeCreated: "190006-0400",
	})...)

	md, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	want := time.Date(2022, 8, 22, 19, 0, 6, 0, time.UTC)
	if !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}
}

// DateCreated is the capture moment; the digital-creation pair describes when
// the file was made, so it only stands in when the stronger field is absent.
func TestReadMetadata_DigitalCreationIsTheWeakerFallback(t *testing.T) {
	digitalOnly := writeJPEG(t, app13(iptcFixture{
		iptcDigitalCreateDate: "20230101",
		iptcDigitalCreateTime: "120000",
	})...)
	md, err := ReadMetadata(digitalOnly)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	if want := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC); !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v", md.TakenAt, want)
	}

	both := writeJPEG(t, app13(iptcFixture{
		iptcDateCreated:       "20220822",
		iptcTimeCreated:       "190006",
		iptcDigitalCreateDate: "20230101",
		iptcDigitalCreateTime: "120000",
	})...)
	md, err = ReadMetadata(both)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	if want := time.Date(2022, 8, 22, 19, 0, 6, 0, time.UTC); !md.TakenAt.Equal(want) {
		t.Errorf("takenAt = %v, want %v — DateCreated should win", md.TakenAt, want)
	}
}

// A file with IPTC but no date reports ErrNoEXIF, so the caller falls back to
// the upload time instead of printing a zero date.
func TestReadMetadata_IPTCWithoutDateReportsNoEXIF(t *testing.T) {
	path := writeJPEG(t, app13(iptcFixture{25: "Photo Booth"})...)

	md, err := ReadMetadata(path)
	if err != ErrNoEXIF {
		t.Errorf("err = %v, want ErrNoEXIF", err)
	}
	if md.HasTakenAt() {
		t.Errorf("takenAt = %v, want zero", md.TakenAt)
	}
}

// Metadata comes from strangers' files, so malformed blocks must be inert
// rather than a panic or a bogus date.
func TestReadMetadata_MalformedIPTCIsIgnored(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"truncated 8BIM header", append([]byte(photoshopSegmentHeader), []byte("8BI")...)},
		{"resource size past end", append([]byte(photoshopSegmentHeader),
			[]byte{'8', 'B', 'I', 'M', 0x04, 0x04, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0x1C}...)},
		{"dataset length past end", append([]byte(photoshopSegmentHeader),
			[]byte{'8', 'B', 'I', 'M', 0x04, 0x04, 0, 0, 0, 0, 0, 6, 0x1C, 0x02, 0x37, 0xFF, 0x00, 0x01}...)},
		{"not a photoshop segment", []byte("Something else entirely")},
		{"empty", nil},
		{"placeholder date", buildAPP13(buildIPTCBlock(iptcFixture{iptcDateCreated: "00000000"}))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeJPEG(t, []byte{0xED}, tc.payload)
			md, err := ReadMetadata(path)
			if err != ErrNoEXIF {
				t.Errorf("err = %v, want ErrNoEXIF", err)
			}
			if md.HasTakenAt() {
				t.Errorf("takenAt = %v, want zero", md.TakenAt)
			}
		})
	}
}

// A file that isn't a JPEG at all must not be mined for stray bytes.
func TestReadIPTC_NonJPEGYieldsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("8BIM\x04\x04 not a jpeg"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	if _, err := readIPTCTakenAt(path); err == nil {
		t.Error("a non-JPEG should yield no IPTC date")
	}
}
