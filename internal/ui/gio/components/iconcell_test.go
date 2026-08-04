package components

// Headless dimension-assertion tests for IconCell's Footer/CornerBadge
// additions (2026-07-29) -- same technique the editor package's own
// dimension tests use (see character_panel_test.go's headlessGtx): a
// synthetic layout.Context with a 1:1 pixel/dp metric, no real GUI window
// needed.

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func headlessGtx() layout.Context {
	return layout.Context{
		Ops:         &op.Ops{},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1200, 800)},
	}
}

func tinyImageOp() paint.ImageOp {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	return paint.NewImageOp(img)
}

// TestIconCellCornerBadgeStaysSmall guards against a real shipped bug: a
// CornerBadge's text was measured under the FULL cell's Exact (Min==Max)
// constraints instead of loose ones, forcing the label -- and its backing
// rect -- to balloon to nearly the whole cell instead of a small corner
// chip. Reported by the user from a screenshot: "the rest of the cell
// beside the foot is all obscured... get back to normal if i insert -1 in
// quantity value" (CornerBadge == "" for -1, which skipped the buggy path
// entirely -- that's why it "fixed" itself).
func TestIconCellCornerBadgeStaysSmall(t *testing.T) {
	th := material.NewTheme()
	gtx := headlessGtx()

	cell := IconCell{Img: tinyImageOp(), CornerBadge: "5"}
	dims := cell.Layout(gtx, th, nil)

	want := gtx.Dp(IconCellSize)
	if dims.Size.X != want || dims.Size.Y != want {
		t.Errorf("IconCell with CornerBadge set: dims = %v, want exactly %dx%d (icon square, no Footer) -- CornerBadge must never change the cell's own size", dims.Size, want, want)
	}
}

// TestIconCellFooterGrowsHeightOnly covers the merchant grid's price
// footer: it must add exactly its own measured height below the icon
// square, without changing the cell's width or without a CornerBadge
// (independently drawn on the icon itself) interfering with that height.
func TestIconCellFooterGrowsHeightOnly(t *testing.T) {
	th := material.NewTheme()
	gtx := headlessGtx()

	const footerH = 20
	footer := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, footerH)}
	}

	cell := IconCell{Img: tinyImageOp(), CornerBadge: "1", Footer: footer}
	dims := cell.Layout(gtx, th, nil)

	iconPx := gtx.Dp(IconCellSize)
	if dims.Size.X != iconPx {
		t.Errorf("width = %d, want %d (Footer must not change cell width)", dims.Size.X, iconPx)
	}
	if want := iconPx + footerH; dims.Size.Y != want {
		t.Errorf("height = %d, want %d (icon square + exact Footer height)", dims.Size.Y, want)
	}
}

// TestBadgeScale covers the Shop Editor's cell-size slider hook: the
// default size (0, meaning "IconCellSize") and IconCellSize itself must
// both resolve to an unscaled 1.0 (today's look, unchanged for every other
// grid), and scaling must move linearly with the chosen size.
func TestBadgeScale(t *testing.T) {
	if got := BadgeScale(0); got != 1 {
		t.Errorf("BadgeScale(0) = %v, want 1", got)
	}
	if got := BadgeScale(IconCellSize); got != 1 {
		t.Errorf("BadgeScale(IconCellSize) = %v, want 1", got)
	}
	if got := BadgeScale(IconCellSize * 2); got != 2 {
		t.Errorf("BadgeScale(2x) = %v, want 2", got)
	}
}
