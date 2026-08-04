package gio

// Selectable UI fonts. Both are Google Fonts, SIL Open Font License (see
// the vendored assets/fonts/*-OFL.txt) -- free to embed and redistribute.
//
// History (2026-07-31, all same day): shipped with a 3rd "Default" (system
// font) option plus EB Garamond and MedievalSharp alongside Cinzel/Lora.
// User feedback: EB Garamond and MedievalSharp were both "very ugly" --
// dropped. Lora ("the one that looks cleaner") promoted to the actual
// default (renamed from "Default" -- there's no separate system-font
// option anymore, Lora IS the baseline). "Elden Ring (Cinzel)" renamed to
// just "Cinzel", its real name -- Cinzel was picked after researching what
// the game's own UI font actually is: a proprietary serif (commonly
// identified as based on Agmena, with Mantinia-style engraved capitals for
// titles/logo) that isn't itself redistributable; Cinzel is the free
// alternative most consistently cited as matching its inscriptional
// dark-fantasy character. Its lowercase glyphs are themselves cap-height by
// design (a genuine "titling" typeface), not a rendering bug.
//
// BUG FIXED, same day (caught from a user screenshot): Gio's text.Shaper
// treats the FIRST face passed to text.WithCollection as the fallback for
// every EMPTY-Typeface query (th.Face == ""), not just for explicit
// lookups by name -- see gioui.org/text's shapeAndWrapText,
// `families := s.defaultFaces` when params.Font.Typeface == "". Loading a
// custom collection at all silently hijacked an empty Typeface into
// rendering as whichever face loaded first. Since Lora is now always the
// baseline (no more "" default case), typefaceFor never returns "" for any
// input -- unrecognized/legacy Settings.Font values fall through to "Lora"
// explicitly, same as valid ones, so this particular trap can't resurface.
// See fonts_test.go for the regression guard.

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
