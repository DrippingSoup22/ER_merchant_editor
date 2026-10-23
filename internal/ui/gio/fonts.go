package gio

// Selectable UI fonts. Both are Google Fonts, SIL Open Font License (see
// the vendored assets/fonts/*-OFL.txt) -- free to embed and redistribute.
//
// Lora is the default; Cinzel is the free face most often matched to the
// game's own proprietary UI serif. Its lowercase is cap-height by design (a
// titling face), not a rendering bug.
//
// Gio's text.Shaper treats the FIRST face passed to text.WithCollection as the
// face for every empty-Typeface query (gioui.org/text's shapeAndWrapText), so a
// custom collection silently takes over th.Face == "". typefaceFor therefore
// never returns "": unknown or legacy Settings.Font values fall through to
// "Lora" explicitly. fonts_test.go guards it.

import (
	_ "embed"
	"fmt"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed assets/fonts/Cinzel.ttf
var cinzelTTF []byte

//go:embed assets/fonts/Lora.ttf
var loraTTF []byte

// fontOptions maps each Settings.Font value to its combo label, in display
// order. "lora" is the default/baseline (first).
var fontOptions = []struct {
	value, label string
}{
	{"lora", "Lora"},
	{"cinzel", "Cinzel"},
}

// typefaceFor resolves a Settings.Font value to the font.Typeface th.Face
// should be set to. Any unrecognized value (including "", covering old
// configs from before Lora became the default) resolves to Lora, same as
// the explicit "lora" value -- never the empty string, see the BUG FIXED
// note above.
func typefaceFor(fontSetting string) font.Typeface {
	switch fontSetting {
	case "cinzel":
		return "Cinzel"
	default:
		return "Lora"
	}
}

// customFontCollection parses the embedded Cinzel/Lora faces into a
// collection for text.NewShaper, additive to (not replacing) Gio's own
// system-font lookup -- any glyph neither face covers (e.g. non-Latin item
// names) still falls back to whatever the shaper's system-font search
// finds.
func customFontCollection() []font.FontFace {
	var out []font.FontFace
	for _, ttf := range [][]byte{cinzelTTF, loraTTF} {
		face, err := opentype.Parse(ttf)
		if err != nil {
			panic(fmt.Errorf("parse embedded font: %w", err))
		}
		out = append(out, font.FontFace{Font: face.Font(), Face: face})
	}
	return out
}
