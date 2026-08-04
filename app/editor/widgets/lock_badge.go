package widgets

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"gioui.org/op/paint"
)

// lockBadgePNG is a pre-rendered padlock badge for locked merchant rows: a
// baked one-off PNG (no in-repo generator), because live Gio clip-shapes
// looked rough at badge size. Its transparency comes from color
// classification, not flood fill -- every near-white/gray pixel goes
// transparent so the dark cell background shows through the shackle gap and
// keyhole; only the black outline and orange fill stay opaque.
//
//go:embed assets/lock_badge.png
var lockBadgePNG []byte

var lockBadgeImg = decodeLockBadge()

func decodeLockBadge() paint.ImageOp {
	img, _, err := image.Decode(bytes.NewReader(lockBadgePNG))
	if err != nil {
		panic("widgets: decode embedded assets/lock_badge.png: " + err.Error())
	}
	return paint.NewImageOp(img)
}
