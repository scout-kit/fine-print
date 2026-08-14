package imaging

import (
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"time"
)

// IPTC capture dates, read as a fallback for files that carry no EXIF.
//
// Photos exported from Photoshop, Lightroom and other desktop editors
// routinely lose their EXIF block but keep the IPTC "DateCreated" the editor
// wrote — which is the capture date the photographer sees in those apps. A
// desktop upload is often exactly such an export, so without this the kiosk
// falls back to the upload time for a photo whose real date is sitting in the
// file.
//
// IPTC lives in a JPEG APP13 segment, inside a Photoshop 8BIM resource block
// of type 0x0404, as a run of IIM datasets.

const (
	iptcRecordApplication = 2

	iptcDateCreated        = 55
	iptcTimeCreated        = 60
	iptcDigitalCreateDate  = 62
	iptcDigitalCreateTime  = 63
	photoshopResourceIPTC  = 0x0404
	photoshopSegmentHeader = "Photoshop 3.0\x00"
)

// errNoIPTCDate reports that the file carried no IPTC creation date.
var errNoIPTCDate = errors.New("no iptc date")

// readIPTCTakenAt returns the capture time recorded in a file's IPTC block.
// Like EXIF, IPTC dates are wall-clock readings, so the value is returned in
// UTC without shifting — what the photographer saw is what gets printed.
func readIPTCTakenAt(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}

	fields := map[byte]string{}
	for _, app13 := range jpegSegments(data, 0xED) {
		payload, ok := trimPrefix(app13, photoshopSegmentHeader)
		if !ok {
			continue
		}
		for tag, value := range iptcDatasets(photoshopResource(payload, photoshopResourceIPTC)) {
			// First value wins; a repeated dataset is a later, weaker copy.
			if _, seen := fields[tag]; !seen {
				fields[tag] = value
			}
		}
	}

	// DateCreated is the capture moment. DigitalCreation* describes when the
	// digital file was made, which for a scan or an export is later, so it is
	// only consulted when the stronger field is absent.
	for _, pair := range [][2]byte{
		{iptcDateCreated, iptcTimeCreated},
		{iptcDigitalCreateDate, iptcDigitalCreateTime},
	} {
		if t, err := parseIPTCDateTime(fields[pair[0]], fields[pair[1]]); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errNoIPTCDate
}

// jpegSegments returns the payloads of every marker segment of the given kind
// (0xED for APP13). Anything that isn't a well-formed JPEG yields nothing.
func jpegSegments(data []byte, marker byte) [][]byte {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}

	var out [][]byte
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			break
		}
		kind := data[i+1]
		// Start of scan: image data follows, no more metadata segments.
		if kind == 0xDA || kind == 0xD9 {
			break
		}
		// Standalone markers carry no length.
		if kind == 0x01 || (kind >= 0xD0 && kind <= 0xD7) {
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if size < 2 || i+2+size > len(data) {
			break
		}
		if kind == marker {
			out = append(out, data[i+4:i+2+size])
		}
		i += 2 + size
	}
	return out
}

// photoshopResource walks the 8BIM blocks in an APP13 payload and returns the
// data of the first one with the given resource id.
func photoshopResource(payload []byte, id uint16) []byte {
	for i := 0; i+12 <= len(payload); {
		if string(payload[i:i+4]) != "8BIM" {
			return nil
		}
		resourceID := binary.BigEndian.Uint16(payload[i+4 : i+6])
		i += 6

		// Pascal-style name, padded to an even total length.
		nameLen := int(payload[i])
		i += 1 + nameLen
		if (nameLen+1)%2 != 0 {
			i++
		}
		if i+4 > len(payload) {
			return nil
		}

		size := int(binary.BigEndian.Uint32(payload[i : i+4]))
		i += 4
		if size < 0 || i+size > len(payload) {
			return nil
		}
		if resourceID == id {
			return payload[i : i+size]
		}
		i += size
		if size%2 != 0 {
			i++
		}
	}
	return nil
}

// iptcDatasets parses IIM datasets, returning the application-record ones by
// their dataset number.
func iptcDatasets(block []byte) map[byte]string {
	out := map[byte]string{}
	for i := 0; i+5 <= len(block); {
		if block[i] != 0x1C {
			break
		}
		record, dataset := block[i+1], block[i+2]
		size := int(binary.BigEndian.Uint16(block[i+3 : i+5]))
		i += 5

		// The high bit marks an extended dataset, whose length is itself
		// variable-width. None of the fields read here use it, so stop rather
		// than guess at the rest of the block.
		if size&0x8000 != 0 {
			break
		}
		if i+size > len(block) {
			break
		}
		if record == iptcRecordApplication {
			out[dataset] = string(block[i : i+size])
		}
		i += size
	}
	return out
}

// parseIPTCDateTime combines an IPTC date ("CCYYMMDD") with its optional time
// ("HHMMSS" plus an optional "±HHMM" zone). A date with no usable time is
// midnight, which is how editors show a date-only photo.
func parseIPTCDateTime(date, clock string) (time.Time, error) {
	date = strings.TrimSpace(strings.TrimRight(date, "\x00"))
	if len(date) != 8 {
		return time.Time{}, errNoIPTCDate
	}

	clock = strings.TrimSpace(strings.TrimRight(clock, "\x00"))
	// The zone travels with the time field, but IPTC times are wall-clock
	// readings like EXIF's, so it is dropped rather than applied.
	if len(clock) > 6 {
		clock = clock[:6]
	}
	if len(clock) != 6 {
		clock = "000000"
	}

	t, err := time.ParseInLocation("20060102150405", date+clock, time.UTC)
	if err != nil {
		return time.Time{}, errNoIPTCDate
	}
	if t.Year() < 1900 {
		return time.Time{}, errNoIPTCDate
	}
	return t, nil
}

// trimPrefix reports whether b starts with prefix and returns the remainder.
func trimPrefix(b []byte, prefix string) ([]byte, bool) {
	if len(b) < len(prefix) || string(b[:len(prefix)]) != prefix {
		return nil, false
	}
	return b[len(prefix):], true
}
