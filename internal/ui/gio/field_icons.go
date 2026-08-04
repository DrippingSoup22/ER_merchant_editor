package gio

// Small glyphs shown beside form field labels (Price/Quantity) in the
// docked row-edit bar. stockSackPNG is a user-provided coin-sack icon,
// background-removed the same way as widgets/lock_badge.go: the source was
// a flat-shaded, 16-color palette PNG whose "transparent" checkerboard was
// actually baked-in flat gray/white pixels ([255,255,255] and [230,230,230]
// exactly, no anti-aliasing blend since the source has no gradients) -- an
// exact-match classification (rather than the lock badge's fuzzy
// lightness/saturation threshold, which that JPEG source needed to shrug
// off compression artifacts) was enough and safer, since it can't
// misclassify the sack's own gray rope/clasp pixels as background.

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"gioui.org/op/paint"
)

//go:embed assets/stock_sack.png
var stockSackPNG []byte

var stockSackImg = decodeFieldIcon(stockSackPNG, "assets/stock_sack.png")

func decodeFieldIcon(data []byte, name string) paint.ImageOp {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		panic("editor: decode embedded " + name + ": " + err.Error())
	}
	return paint.NewImageOp(img)
}
