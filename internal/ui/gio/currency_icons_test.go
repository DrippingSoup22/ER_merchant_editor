package gio

import (
	"image"
	"image/color"
	"testing"
)

// TestCropToContentTrimsTransparentPadding covers the fix for "the rune
// icon png in the foot is still too small... there is a lot of empty space
// in the image": a synthetic 100x100 canvas with a 20x20 opaque square
// centered at (40,40)-(60,60) must crop down close to that square (plus
// the small configured margin), not stay at the full canvas size.
func TestCropToContentTrimsTransparentPadding(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 40; y < 60; y++ {
		for x := 40; x < 60; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xFF, A: 0xFF})
		}
	}

	got := cropToContent(img)
	b := got.Bounds()

	// Content bbox is (40,40)-(59,59) inclusive -> width/height 20;
	// currencyCropMarginFrac (0.12) adds ~2px on each side, then +1 for the
	// exclusive-max rectangle math -- allow a small tolerance rather than
	// pinning the exact pixel, since the point is "much smaller than the
	// original 100x100 canvas, not exactly the same".
	if b.Dx() > 30 || b.Dy() > 30 {
		t.Errorf("cropped size = %dx%d, want roughly the 20x20 content plus a small margin (<=30x30), got a crop that barely shrank the 100x100 canvas", b.Dx(), b.Dy())
	}
	if b.Dx() < 20 || b.Dy() < 20 {
		t.Errorf("cropped size = %dx%d, want at least the 20x20 content itself", b.Dx(), b.Dy())
	}
}

// TestCropToContentFullyTransparentReturnsOriginal covers the fallback for
// an all-transparent image (shouldn't happen for a real item icon, but
// must not panic or return a zero-size/garbage crop).
func TestCropToContentFullyTransparentReturnsOriginal(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 50, 50))
	got := cropToContent(img)
	if got.Bounds() != img.Bounds() {
		t.Errorf("fully-transparent image: bounds = %v, want unchanged %v", got.Bounds(), img.Bounds())
	}
}

// TestCurrencyIconCaches covers that currencyIcon only decodes/crops a
// given path once -- repeated calls must return from the cache, not
// re-read/re-decode the embedded asset every time.
func TestCurrencyIconCaches(t *testing.T) {
	currencyIconMu.Lock()
	delete(currencyIconCache, runeIconPath)
	currencyIconMu.Unlock()

	first := currencyIcon(runeIconPath)
	_, ok := currencyIconCache[runeIconPath]
	if !ok {
		t.Fatal("currencyIcon did not populate the cache")
	}
	second := currencyIcon(runeIconPath)
	if first != second {
		t.Error("currencyIcon returned a different ImageOp on a cached call")
	}
}
