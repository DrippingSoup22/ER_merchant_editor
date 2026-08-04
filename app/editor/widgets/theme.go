package widgets

import "image/color"

// SetPalette switches this package's widget colors between the dark
// (default), light, and elden (Elden-Ring-flavored warm parchment/gold/
// bronze) looks. Called from the editor's theme switch on the UI goroutine —
// these variables are only ever read during layout on the same goroutine.
func SetPalette(theme string) {
	switch theme {
	case "light":
		// Warm paper tones matching editor.applyEditorPalette's "light" --
		// see its comment. Icon cells are deliberately a touch darker than
		// the panel so the grid reads as a grid of slots.
		comboBg = color.NRGBA{R: 0xFF, G: 0xFE, B: 0xFB, A: 0xFF}
		comboHoverBg = color.NRGBA{R: 0xE7, G: 0xE1, B: 0xD5, A: 0xFF}
		comboBorder = color.NRGBA{R: 0xB5, G: 0xAB, B: 0x98, A: 0xFF}
		comboSelBg = color.NRGBA{R: 0xD8, G: 0xDF, B: 0xEA, A: 0xFF}
		iconCellBg = color.NRGBA{R: 0xE4, G: 0xDF, B: 0xD4, A: 0xFF}
		iconCellHoverBg = color.NRGBA{R: 0xD5, G: 0xCE, B: 0xBE, A: 0xFF}
		iconCellBorder = color.NRGBA{R: 0xBE, G: 0xB4, B: 0xA1, A: 0xFF}
		cornerBadgeTextColor = color.NRGBA{R: 0x18, G: 0x16, B: 0x12, A: 0xFF}
		tooltipBg = color.NRGBA{R: 0xFF, G: 0xFE, B: 0xFB, A: 0xF8}
		tooltipEdge = color.NRGBA{R: 0xB5, G: 0xAB, B: 0x98, A: 0xFF}
		tooltipText = color.NRGBA{R: 0x24, G: 0x21, B: 0x1D, A: 0xFF}
		modalBg = color.NRGBA{R: 0xF8, G: 0xF6, B: 0xF1, A: 0xFF}
		modalBorderC = color.NRGBA{R: 0xB5, G: 0xAB, B: 0x98, A: 0xFF}
	case "elden":
		// Matched to the game's inventory screen alongside
		// editor.applyEditorPalette's "elden" -- see its comment. Item slots
		// there are near-black with a thin bronze edge and a warm brown
		// highlight on the hovered/selected one.
		comboBg = color.NRGBA{R: 0x16, G: 0x13, B: 0x0F, A: 0xFF}
		comboHoverBg = color.NRGBA{R: 0x27, G: 0x20, B: 0x15, A: 0xFF}
		comboBorder = color.NRGBA{R: 0x4A, G: 0x3E, B: 0x2A, A: 0xFF}
		comboSelBg = color.NRGBA{R: 0x3B, G: 0x2E, B: 0x18, A: 0xFF}
		iconCellBg = color.NRGBA{R: 0x12, G: 0x0F, B: 0x0B, A: 0xFF}
		iconCellHoverBg = color.NRGBA{R: 0x2A, G: 0x22, B: 0x16, A: 0xFF}
		iconCellBorder = color.NRGBA{R: 0x39, G: 0x30, B: 0x21, A: 0xFF}
		cornerBadgeTextColor = color.NRGBA{R: 0xF0, G: 0xE6, B: 0xD0, A: 0xFF}
		tooltipBg = color.NRGBA{R: 0x1A, G: 0x16, B: 0x10, A: 0xF5}
		tooltipEdge = color.NRGBA{R: 0x4A, G: 0x3E, B: 0x2A, A: 0xFF}
		tooltipText = color.NRGBA{R: 0xD8, G: 0xCD, B: 0xB4, A: 0xFF}
		modalBg = color.NRGBA{R: 0x16, G: 0x13, B: 0x0F, A: 0xFF}
		modalBorderC = color.NRGBA{R: 0x4A, G: 0x3E, B: 0x2A, A: 0xFF}
	default: // "dark"
		comboBg = color.NRGBA{R: 0x2A, G: 0x2A, B: 0x2C, A: 0xFF}
		comboHoverBg = color.NRGBA{R: 0x38, G: 0x38, B: 0x3C, A: 0xFF}
		comboBorder = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
		comboSelBg = color.NRGBA{R: 0x39, G: 0x45, B: 0x5A, A: 0xFF}
		iconCellBg = color.NRGBA{R: 0x26, G: 0x26, B: 0x28, A: 0xFF}
		iconCellHoverBg = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x40, A: 0xFF}
		iconCellBorder = color.NRGBA{R: 0x3E, G: 0x3E, B: 0x44, A: 0xFF}
		cornerBadgeTextColor = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		tooltipBg = color.NRGBA{R: 0x33, G: 0x33, B: 0x36, A: 0xF2}
		tooltipEdge = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
		tooltipText = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
		modalBg = color.NRGBA{R: 0x2C, G: 0x2C, B: 0x2E, A: 0xFF}
		modalBorderC = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
	}
}
