package editor

// Cropped currency-icon renderings for the merchant grid's price footer
// (2026-07-29): the vendored source icons (data/icons, same embedded FS
// icons.go's shared IconCache reads) carry a lot of transparent padding
// around the actual glyph -- shadow_realm_rune_5.png's rune only fills
// about half its 256x256 canvas -- so a plain Contain-fit renders it tiny
// inside the footer's small icon box no matter how big that box is. User:
// "if the problem is that there is a lot of empty space in the image and
// the rune doesn't cover most of it the result will be the same until we
// apply a 4x on the dimension but i fear that this could mess with the
// foot space." Rather than enlarging the box, each currency icon is
// cropped ONCE at first use to its own alpha bounding box (plus a small
// margin) and cached, so it arrives already "zoomed in" on the visible
// glyph. This is a SEPARATE rendering from the shared IconCache -- the
// Catalog/pending views must keep showing these exact items uncropped if
// they're ever browsed/staged as regular items, not just used here as a
// currency glyph.

import (
	"bytes"
	"image"
	_ "image/png" // registers the PNG decoder for image.Decode
	"sync"

	"gioui.org/op/paint"
	_ "golang.org/x/image/webp" // parity with icons.go: some vendored "PNGs" are WebP

	assets "er_merchant_editor"
)

// currencyCropMarginFrac pads the cropped glyph a little past its own
// content bounding box on every side, so it doesn't touch the box's edges.
const currencyCropMarginFrac = 0.12

// currencyAlphaThreshold is the RGBA().A cutoff (out of 65535) a pixel must
// clear to count as "content" rather than padding -- low enough to keep
// soft/anti-aliased glyph edges, high enough to ignore near-fully-
// transparent padding.
const currencyAlphaThreshold = 2570 // ~10/255

var (
	currencyIconMu    sync.Mutex
	currencyIconCache = map[string]paint.ImageOp{}
)

// currencyIcon returns iconPath's cropped-to-content rendering, decoding
// and cropping once per distinct path (there are only 4 in practice -- the
// rune plus the 3 named cost types, see field_meta.go) and caching the
// result. Safe to call every frame; the map lookup is the fast path.
func currencyIcon(iconPath string) paint.ImageOp {
	currencyIconMu.Lock()
	defer currencyIconMu.Unlock()
	if op, ok := currencyIconCache[iconPath]; ok {
		return op
	}
	op := decodeCroppedCurrencyIcon(iconPath)
	currencyIconCache[iconPath] = op
	return op
}

func decodeCroppedCurrencyIcon(iconPath string) paint.ImageOp {
	raw, err := assets.Icons.ReadFile("data/icons/" + iconPath)
	if err != nil {
		return paint.ImageOp{}
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return paint.ImageOp{}
	}
	return paint.NewImageOp(cropToContent(src))
}

// cropToContent returns img's tightest sub-image containing every pixel
// above currencyAlphaThreshold, expanded by currencyCropMarginFrac on each
// side. Falls back to img unchanged if it's fully transparent (shouldn't
// happen for a real item icon) or doesn't support SubImage.
func cropToContent(img image.Image) image.Image {
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	si, ok := img.(subImager)
	if !ok {
		return img
	}
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > currencyAlphaThreshold {
				found = true
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		return img
	}
	mx := int(float64(maxX-minX) * currencyCropMarginFrac)
	my := int(float64(maxY-minY) * currencyCropMarginFrac)
	rect := image.Rect(minX-mx, minY-my, maxX+mx+1, maxY+my+1).Intersect(b)
	return si.SubImage(rect)
}
