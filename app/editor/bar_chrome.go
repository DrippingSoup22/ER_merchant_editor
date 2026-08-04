package editor

// Generic, reusable rendering primitives shared across the editor's panels,
// split out of row_edit_form.go (pure code motion). Every declaration here has
// at least one caller OUTSIDE row_edit_form.go -- goldRule and groupBox in
// pending_edits.go, formButton in the catalog/character/pending panels,
// fixedIcon in merchant_panel.go, framedIcon in pending_edits.go -- which is
// exactly what makes them shared "chrome" rather than row-edit-bar specifics.

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// goldRuleHeight is the thin accent rule's thickness, under the docked bar's
// title -- an Elden-Ring-menu-styled touch (thin gold rule under a title),
// using colorAmber since that's already a themed gold/amber tone in all
// three palettes (dark/light/elden), so no new theme-conditional color was
// needed.
const goldRuleHeight = unit.Dp(2)

func goldRule(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(goldRuleHeight)
	sz := image.Pt(gtx.Constraints.Max.X, h)
	paint.FillShape(gtx.Ops, colorAmber, clip.Rect{Max: sz}.Op())
	return layout.Dimensions{Size: sz}
}

// fixedIcon paints an icon at an exact square size.
func fixedIcon(gtx layout.Context, img paint.ImageOp, size unit.Dp) layout.Dimensions {
	side := gtx.Dp(size)
	sz := image.Pt(side, side)
	gtx.Constraints = layout.Exact(sz)
	widget.Image{Src: img, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	return layout.Dimensions{Size: sz}
}

// framedIcon draws an icon inside a bordered "inventory slot" (a filled
// backing square plus a 1dp border), matching the merchant/catalog grid
// cells' look instead of a bare floating icon.
func framedIcon(gtx layout.Context, img paint.ImageOp, size unit.Dp) layout.Dimensions {
	side := gtx.Dp(size)
	sz := image.Pt(side, side)
	paint.FillShape(gtx.Ops, colorInputBg, clip.Rect{Max: sz}.Op())
	gtx.Constraints = layout.Exact(sz)
	widget.Image{Src: img, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(3)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: sz} })
	return layout.Dimensions{Size: sz}
}

// groupBox wraps w in a bordered card (no separate fill -- the docked bar
// already sits on the panel background, so a border alone is enough to read
// as a distinct section) grouping the Price/Quantity/Level fields, sized to
// its content rather than stretched to the docked bar's full width (a
// numeric field group stretched edge-to-edge on a wide panel looks odd).
func groupBox(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(10)).Layout(gtx, w)
	call := macro.Stop()

	call.Add(gtx.Ops)
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(4)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

// formButton is a compact accent button for the edit form's small actions.
func formButton(th *material.Theme, btn *widget.Clickable, label string) material.ButtonStyle {
	b := material.Button(th, btn, label)
	b.Inset = layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}
	b.TextSize = th.TextSize * 12 / 16
	b.CornerRadius = 3
	return b
}
