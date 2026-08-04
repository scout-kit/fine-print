package api

import (
	"net/http"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/imaging"
)

// DateFormatOption describes one date preset for the admin UI picker.
type DateFormatOption struct {
	Key     string `json:"key"`
	Example string `json:"example"`
	Default bool   `json:"default"`
}

// ListDateFormats returns the date/datetime presets a text overlay can use,
// each rendered against a sample timestamp. Serving these from the backend
// keeps the picker's examples honest — they come from the same formatter that
// renders the print, so the UI can't drift from what actually gets printed.
func (h *Handlers) ListDateFormats(w http.ResponseWriter, r *http.Request) {
	// A sample instant chosen so every field is unambiguous: a two-digit day
	// distinct from the month, and an afternoon time so 12-hour presets show
	// PM. Fixed rather than time.Now() so the picker is stable.
	sample := time.Date(2026, time.March, 14, 14, 31, 0, 0, time.UTC)

	build := func(source imaging.TextSource, def imaging.DateFormat) []DateFormatOption {
		presets := imaging.DateFormatsFor(source)
		opts := make([]DateFormatOption, 0, len(presets))
		for _, f := range presets {
			opts = append(opts, DateFormatOption{
				Key:     string(f),
				Example: imaging.FormatDate(sample, source, f),
				Default: f == def,
			})
		}
		return opts
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sample": sample.Format(time.RFC3339),
		"sources": []map[string]any{
			{
				"key":     db.TextSourceStatic,
				"label":   "Static text",
				"formats": []DateFormatOption{},
			},
			{
				"key":     db.TextSourcePhotoDate,
				"label":   "Date photo was taken",
				"formats": build(imaging.TextSourcePhotoDate, imaging.DefaultDateFormat),
			},
			{
				"key":     db.TextSourcePhotoDateTime,
				"label":   "Date + time photo was taken",
				"formats": build(imaging.TextSourcePhotoDateTime, imaging.DefaultDateTimeFormat),
			},
		},
	})
}
