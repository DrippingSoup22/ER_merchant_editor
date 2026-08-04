package gio

// Regression coverage for the 2026-07-31 "Default font renders as Cinzel"
// bug (see fonts.go's "BUG FIXED" note): gioui.org/text.Shaper silently
// resolves an EMPTY Typeface query to the first face loaded via
// text.WithCollection, so simply adding a custom font collection at all
// hijacks any code path that leaves Font.Typeface == "". Caught from a
// real user screenshot after this shipped. typefaceFor now never returns
// "" for any input (TestTypefaceForMapsEveryFontOption in
// settings_test.go covers that directly); this file additionally shapes
// real text through the real shaper to confirm the two embedded faces are
// genuinely distinct, not accidentally the same file twice.

import (
	"testing"

	"gioui.org/font"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// shapeAscent shapes a single rune with the given typeface (using the same
// shaper construction applyTheme performs) and returns its Ascent metric --
// a cheap per-face fingerprint good enough to tell "which face actually got
// picked" apart without a real render.
func shapeAscent(t *testing.T, shaper *text.Shaper, typeface font.Typeface) fixed.Int26_6 {
	t.Helper()
	shaper.LayoutString(text.Parameters{
		Font:    font.Font{Typeface: typeface},
		PxPerEm: fixed.I(16),
	}, "A")
	g, ok := shaper.NextGlyph()
	if !ok {
		t.Fatalf("typeface %q: no glyph shaped", typeface)
	}
	return g.Ascent
}

// TestLoraAndCinzelAreDistinctFaces shapes the same rune through both
// embedded typefaces via the real shaper construction (customFontCollection
// + text.WithCollection) and asserts they differ -- confirms both fonts
// actually loaded as distinct faces (not, say, Lora.ttf accidentally
// containing/aliasing to Cinzel).
func TestLoraAndCinzelAreDistinctFaces(t *testing.T) {
	shaper := text.NewShaper(text.WithCollection(customFontCollection()))
	cinzel := shapeAscent(t, shaper, "Cinzel")
	lora := shapeAscent(t, shaper, "Lora")
	if cinzel == lora {
		t.Fatalf("Cinzel and Lora shaped identically (Ascent=%v) -- expected distinct faces", cinzel)
	}
}
