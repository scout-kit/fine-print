package imaging

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// inkExtent returns the leftmost and rightmost x containing a non-black pixel,
// i.e. the horizontal span the drawn text actually occupies. Returns ok=false
// when nothing was drawn.
func inkExtent(img image.Image) (minX, maxX int, ok bool) {
	b := img.Bounds()
	minX, maxX = b.Max.X, b.Min.X
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0x2000 || g > 0x2000 || bl > 0x2000 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				ok = true
			}
		}
	}
	return minX, maxX, ok
}

// blackCanvas gives the text something to be measured against.
func blackCanvas(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	return img
}

func renderAt(t *testing.T, text string, align TextAlign, x float64) image.Image {
	t.Helper()
	out, err := RenderText(blackCanvas(1200, 300), TextParams{
		Text:     text,
		FontSize: 40,
		Color:    "#FFFFFF",
		X:        x,
		Y:        0.3,
		Opacity:  1,
		Align:    align,
	})
	if err != nil {
		t.Fatalf("RenderText(%q, %q): %v", text, align, err)
	}
	return out
}

// Two strings of very different length share a left edge when left-anchored.
// This is the pre-existing behavior and must not change.
func TestRenderText_LeftAlignPinsLeftEdge(t *testing.T) {
	shortMin, shortMax, ok := inkExtent(renderAt(t, "May 1, 2026", TextAlignLeft, 0.2))
	if !ok {
		t.Fatal("short string drew nothing")
	}
	longMin, longMax, ok := inkExtent(renderAt(t, "September 30, 2026", TextAlignLeft, 0.2))
	if !ok {
		t.Fatal("long string drew nothing")
	}

	if abs(longMin-shortMin) > 2 {
		t.Errorf("left edges differ: short=%d long=%d, want them pinned together", shortMin, longMin)
	}
	// Sanity: the longer date really is wider, so the test isn't vacuous.
	if longMax <= shortMax {
		t.Fatalf("long string (max x %d) is not wider than short (max x %d) — test is not measuring anything",
			longMax, shortMax)
	}
}

// The fix: right-anchored text keeps its right edge fixed and grows leftward,
// so a longer date can't creep past x toward the edge of the print.
func TestRenderText_RightAlignPinsRightEdge(t *testing.T) {
	shortMin, shortMax, ok := inkExtent(renderAt(t, "May 1, 2026", TextAlignRight, 0.8))
	if !ok {
		t.Fatal("short string drew nothing")
	}
	longMin, longMax, ok := inkExtent(renderAt(t, "September 30, 2026", TextAlignRight, 0.8))
	if !ok {
		t.Fatal("long string drew nothing")
	}

	if abs(longMax-shortMax) > 2 {
		t.Errorf("right edges differ: short=%d long=%d, want them pinned together", shortMax, longMax)
	}
	// The extra width must have gone to the left.
	if longMin >= shortMin {
		t.Errorf("long string starts at %d, short at %d — right-anchored text should extend leftward",
			longMin, shortMin)
	}
}

// The anchored edge should land at the requested fraction of the image width.
func TestRenderText_AnchorLandsAtRequestedX(t *testing.T) {
	const width = 1200
	const frac = 0.75
	want := int(frac * width)

	_, maxX, ok := inkExtent(renderAt(t, "September 30, 2026", TextAlignRight, frac))
	if !ok {
		t.Fatal("nothing drawn")
	}
	// Glyph side bearing means the last ink sits a hair inside the advance
	// width, so allow a few pixels of slack.
	if maxX > want || want-maxX > 12 {
		t.Errorf("right edge at x=%d, want just inside %d", maxX, want)
	}

	minX, _, ok := inkExtent(renderAt(t, "September 30, 2026", TextAlignLeft, frac))
	if !ok {
		t.Fatal("nothing drawn")
	}
	if minX < want || minX-want > 12 {
		t.Errorf("left edge at x=%d, want just inside %d", minX, want)
	}
}

// A right-anchored string long enough to overflow when left-anchored must stay
// on the canvas — the whole point of the feature.
func TestRenderText_RightAlignKeepsLongTextOnCanvas(t *testing.T) {
	const long = "September 30, 2026 at 11:59 PM"

	// Left-anchored near the right edge: runs off.
	_, leftMax, ok := inkExtent(renderAt(t, long, TextAlignLeft, 0.9))
	if !ok {
		t.Fatal("nothing drawn for the left-anchored case")
	}
	if leftMax < 1199 {
		t.Skipf("left-anchored text at x=0.9 ended at %d without clipping; "+
			"font is too narrow for this test to be meaningful", leftMax)
	}

	// Right-anchored at the same x: fully inside.
	rMin, rMax, ok := inkExtent(renderAt(t, long, TextAlignRight, 0.9))
	if !ok {
		t.Fatal("nothing drawn for the right-anchored case")
	}
	if rMax >= 1199 {
		t.Errorf("right-anchored text still reaches the canvas edge (max x %d)", rMax)
	}
	if rMin < 0 {
		t.Errorf("right-anchored text ran off the left edge (min x %d)", rMin)
	}
}

// An empty or unknown alignment behaves as left, so rows written before the
// column existed render exactly as they used to.
func TestRenderText_EmptyAlignBehavesAsLeft(t *testing.T) {
	blank, _, ok := inkExtent(renderAt(t, "September 30, 2026", "", 0.3))
	if !ok {
		t.Fatal("nothing drawn with empty align")
	}
	left, _, ok := inkExtent(renderAt(t, "September 30, 2026", TextAlignLeft, 0.3))
	if !ok {
		t.Fatal("nothing drawn with explicit left align")
	}
	if blank != left {
		t.Errorf("empty align rendered at %d, explicit left at %d — they must match", blank, left)
	}
}

func TestTextAlign_Valid(t *testing.T) {
	for _, a := range []TextAlign{TextAlignLeft, TextAlignRight} {
		if !a.Valid() {
			t.Errorf("%q should be valid", a)
		}
	}
	for _, a := range []TextAlign{"", "center", "LEFT", "start", "justify"} {
		if TextAlign(a).Valid() {
			t.Errorf("%q should not be valid", a)
		}
	}
	if DefaultTextAlign != TextAlignLeft {
		t.Errorf("DefaultTextAlign = %q, want left for backward compatibility", DefaultTextAlign)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
