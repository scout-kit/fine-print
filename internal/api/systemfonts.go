package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type systemFont struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	CSSName string `json:"css_name"`
}

// ListSystemFonts returns available system fonts + uploaded fonts.
func (h *Handlers) ListSystemFonts(w http.ResponseWriter, r *http.Request) {
	var fonts []systemFont

	// Scan system font directories
	for _, dir := range fontDirs() {
		scanFontDir(dir, &fonts)
	}

	// Sort by name
	sort.Slice(fonts, func(i, j int) bool {
		return fonts[i].Name < fonts[j].Name
	})

	// Deduplicate by name (keep first occurrence)
	seen := make(map[string]bool)
	unique := make([]systemFont, 0, len(fonts))
	for _, f := range fonts {
		if !seen[f.Name] {
			seen[f.Name] = true
			unique = append(unique, f)
		}
	}

	// Add uploaded fonts
	uploaded, _ := h.queries.ListFonts(r.Context())
	for _, f := range uploaded {
		path := h.store.Path("fonts", f.StorageKey)
		unique = append(unique, systemFont{
			Name:    f.Name + " (uploaded)",
			Path:    path,
			CSSName: f.Name,
		})
	}

	writeJSON(w, http.StatusOK, unique)
}

func fontDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
			"/Library/Fonts",
			filepath.Join(os.Getenv("HOME"), "Library/Fonts"),
		}
	case "linux":
		return []string{
			"/usr/share/fonts",
			"/usr/local/share/fonts",
			filepath.Join(os.Getenv("HOME"), ".fonts"),
			filepath.Join(os.Getenv("HOME"), ".local/share/fonts"),
		}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("WINDIR"), "Fonts"),
		}
	default:
		return nil
	}
}

func scanFontDir(dir string, fonts *[]systemFont) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			// Recurse one level
			scanFontDir(filepath.Join(dir, e.Name()), fonts)
			continue
		}

		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
			continue
		}

		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))

		// Derive CSS font-family name by stripping style suffixes
		cssName := name
		for _, suffix := range []string{" Bold Italic", " Bold", " Italic", " Regular", " Light", " Medium", " Thin", " Black", " Condensed", " Narrow", " Rounded", " Outline"} {
			cssName = strings.TrimSuffix(cssName, suffix)
		}
		// Also handle "HB" suffix (e.g., ArialHB)
		cssName = strings.TrimSuffix(cssName, "HB")

		*fonts = append(*fonts, systemFont{
			Name:    name,
			CSSName: cssName,
			Path: filepath.Join(dir, e.Name()),
		})
	}
}
