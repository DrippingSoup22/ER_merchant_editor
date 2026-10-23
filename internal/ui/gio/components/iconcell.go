package components

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// IconCellSize is the fixed side length of an icon cell. Icons are cached at
// 128px, so anything up to 128dp stays sharp.
const IconCellSize = unit.Dp(80)

// BadgeScale gives the size ratio (relative to the pre-slider 80dp default)
// small on-cell detail -- CornerBadge's text, and callers' own price-footer
// text/icons -- should render at for a given cell size, so a bigger Shop
// Editor cell (see merchant_panel.go's cell-size slider) actually makes
// that detail easier to read instead of just enlarging the icon artwork.
// size == 0 means "use IconCellSize" (every other grid), which resolves to
// exactly 1.0 -- today's unscaled look.
func BadgeScale(size unit.Dp) float32 {
	if size == 0 {
		return 1
	}
	return float32(size) / float32(IconCellSize)
}

var (
	iconCellBg           = color.NRGBA{R: 0x26, G: 0x26, B: 0x28, A: 0xFF}
	iconCellHoverBg      = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x40, A: 0xFF}
	iconCellBorder       = color.NRGBA{R: 0x3E, G: 0x3E, B: 0x44, A: 0xFF} // default contour, distinct from panel bg
	editedMarkerColor    = color.NRGBA{R: 0x49, G: 0xC5, B: 0xB6, A: 0xFF} // compact edited-state accent; distinct from amber DnD
	iconDimVeil          = color.NRGBA{A: 0x99}                            // translucent black over disabled cells
	cornerBadgeTextColor = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // CornerBadge's text -- no backing chip
	tooltipBg            = color.NRGBA{R: 0x33, G: 0x33, B: 0x36, A: 0xF2}
	tooltipEdge          = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
	tooltipText          = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
	tooltipWidth         = unit.Dp(280)
)

// lockBadgeSize is the corner badge's diameter.
const lockBadgeSize = unit.Dp(22)

// IconCell describes one icon button. It is a by-value style; the caller
// supplies the retained click state so the same cell keeps its hover/press
// behaviour across frames.
//
// The tooltip is immediate-mode: it is drawn if and only if the cell is
// hovered THIS frame — it appears and disappears with the cursor, with no
// delay timer and no retained state that could leave a stale popup behind
// (which the earlier x/component TipArea did).
type IconCell struct {
	// Img is the icon to paint (an already-decoded paint.ImageOp).
	Img paint.ImageOp
	// Size overrides the cell's side length; 0 = IconCellSize (the default
	// used by the catalog/merchant grids). Lets a compact display -- e.g.
	// the row-edit bar's multi-select preview grid -- reuse the same
	// hover/tooltip/border chrome at a smaller footprint.
	Size unit.Dp
	// Border, when non-nil, draws a border in that color (BorderWidth wide).
	Border *color.NRGBA
	// BorderWidth is the Border stroke width; 0 means the 2dp state default.
	BorderWidth unit.Dp
	// Edited draws a compact teal bracket in the icon square's top-left.
	// Unlike a full contour it remains visually separate from selection,
	// warnings, and the merchant grid's amber drag-and-drop target outline.
	Edited bool
	// Disabled dims the cell; click handling stays with the caller (which
	// ignores clicks on disabled cells) so hover/tooltip still work.
	Disabled bool
	// Locked draws a small padlock badge in the icon square's top-right
	// corner. Independent of Border: a locked cell can still show a selection/
	// warning border or edited marker at the same time; the badge layers on top.
	Locked bool
	// CornerBadge, when non-empty, draws a small number in the icon square's
	// BOTTOM-right corner (the merchant grid's stock quantity, mirroring how
	// Locked occupies the top-right) -- independent of Footer, since it sits
	// on the icon artwork itself, not in the strip below it.
	CornerBadge string
	// Footer, when non-nil, renders in a strip BELOW the icon square, inside
	// the SAME click/hover region (the cell's total height grows to fit it)
	// -- e.g. the merchant grid's price line (currency icon + number,
	// mimicking the in-game shop list). Measured once via op.Record so its
	// height is whatever its own content needs; the caller is responsible
	// for its own padding. nil = no footer, the cell stays exactly Size
	// square (every other grid's existing behavior).
	Footer layout.Widget
	// Tooltip is the hover text ("" = none). May be multi-line.
	Tooltip string
}

// Layout draws the cell. click is the caller-retained per-cell state.
//
// Hovered() must be read INSIDE click.Layout (which processes this frame's
// pointer events first) — reading it before Layout renders one frame stale,
// and if nothing else triggers a redraw the highlight/tooltip only appear
// when the cursor leaves (the leave event finally forces a frame).
func (c IconCell) Layout(gtx layout.Context, th *material.Theme, click *widget.Clickable) layout.Dimensions {
	if click == nil {
		return c.content(gtx, th, false)
	}
	dims := click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return c.content(gtx, th, click.Hovered())
	})
	if c.Tooltip != "" && click.Hovered() {
		c.drawTooltip(gtx, th, dims)
	}
	return dims
}

func (c IconCell) content(gtx layout.Context, th *material.Theme, hovered bool) layout.Dimensions {
	size := c.Size
	if size == 0 {
		size = IconCellSize
	}
	side := gtx.Dp(size)
	iconSz := image.Pt(side, side)

	// Footer is measured first (if any) so the cell's total height -- and
	// therefore its click/hover area -- includes it from the start; it
	// lives inside the SAME region as the icon, not floating separately
	// below the Clickable.
	var footerCall op.CallOp
	footerH := 0
	if c.Footer != nil {
		fgtx := gtx
		fgtx.Constraints = layout.Constraints{Max: image.Pt(iconSz.X, 1e6)}
		macro := op.Record(gtx.Ops)
		dims := c.Footer(fgtx)
		footerCall = macro.Stop()
		footerH = dims.Size.Y
	}
	sz := image.Pt(iconSz.X, iconSz.Y+footerH)
	gtx.Constraints = layout.Exact(sz)

	// Background, lightened under the cursor so the hovered cell reads as
	// live; hand cursor on actionable cells. Scoped to the ICON SQUARE only
	// (not any Footer, which paints its own content) to mimic the game's
	// item-card look. The Clickable's hit region (sz, footer included) is
	// unchanged -- clicking the footer still selects the cell, only the
	// visual state-coloring is scoped to the icon.
	bg := iconCellBg
	if hovered && !c.Disabled {
		bg = iconCellHoverBg
		pointer.CursorPointer.Add(gtx.Ops)
	}
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: iconSz}.Op())

	// Icon, scaled to fit its own square and centered -- constrained to
	// iconSz specifically so a Footer below it doesn't stretch/distort it.
	func() {
		igtx := gtx
		igtx.Constraints = layout.Exact(iconSz)
		widget.Image{Src: c.Img, Fit: widget.Contain, Position: layout.Center}.Layout(igtx)
	}()

	if c.Footer != nil {
		off := op.Offset(image.Pt(0, iconSz.Y)).Push(gtx.Ops)
		footerCall.Add(gtx.Ops)
		off.Pop()
	}

	// Dim veil for disabled cells -- icon square only, same reasoning as
	// the background above.
	if c.Disabled {
		paint.FillShape(gtx.Ops, iconDimVeil, clip.Rect{Max: iconSz}.Op())
	}

	// Border: a state color (selected/pending/warn) at 2dp when set (or a
	// caller-chosen BorderWidth, e.g. the 1dp drag tint), else a subtle
	// default contour so every item sits in a visible square. Icon square
	// only, same reasoning as the background above.
	if c.Border != nil {
		bw := c.BorderWidth
		if bw == 0 {
			bw = unit.Dp(2)
		}
		widget.Border{Color: *c.Border, Width: bw}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: iconSz} })
	} else {
		widget.Border{Color: iconCellBorder, Width: unit.Dp(1)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: iconSz} })
	}
	if c.Edited {
		c.drawEditedMarker(gtx)
	}

	if c.Locked {
		c.drawLockBadge(gtx, iconSz)
	}
	if c.CornerBadge != "" {
		c.drawCornerBadge(gtx, th, iconSz)
	}

	return layout.Dimensions{Size: sz}
}

// drawEditedMarker paints two short strokes as a top-left corner bracket.
// Its small footprint keeps a grid with many staged edits calm, while the
// unique teal shape stays legible without competing with full-cell borders.
func (c IconCell) drawEditedMarker(gtx layout.Context) {
	inset := gtx.Dp(unit.Dp(4))
	length := gtx.Dp(unit.Dp(17))
	width := gtx.Dp(unit.Dp(3))
	paint.FillShape(gtx.Ops, editedMarkerColor, clip.Rect{
		Min: image.Pt(inset, inset),
		Max: image.Pt(inset+length, inset+width),
	}.Op())
	paint.FillShape(gtx.Ops, editedMarkerColor, clip.Rect{
		Min: image.Pt(inset, inset),
		Max: image.Pt(inset+width, inset+length),
	}.Op())
}

// drawLockBadge paints the pre-rendered padlock badge image (lock_badge.go)
// in the cell's top-right corner. It's a baked image rather than live Gio
// shapes for the reason lock_badge.go documents.
func (c IconCell) drawLockBadge(gtx layout.Context, cellSize image.Point) {
	d := gtx.Dp(lockBadgeSize)
	margin := gtx.Dp(unit.Dp(4))
	origin := image.Pt(cellSize.X-d-margin, margin)
	off := op.Offset(origin).Push(gtx.Ops)
	gtx.Constraints = layout.Exact(image.Pt(d, d))
	widget.Image{Src: lockBadgeImg, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	off.Pop()
}

// drawCornerBadge paints CornerBadge's plain text (no backing chip) in the
// icon square's bottom-right corner (mirrors drawLockBadge's top-right
// positioning/margin). cornerBadgeTextColor is theme-reactive (SetPalette,
// theme.go) so it stays legible without a background: white for dark/elden,
// near-black for light.
func (c IconCell) drawCornerBadge(gtx layout.Context, th *material.Theme, iconSz image.Point) {
	// Incoming gtx.Constraints is still Exact(sz) from content() (Min == Max):
	// loosen to the icon square before measuring, else material.Body2's label
	// is forced to fill nearly the whole cell instead of sizing to its text.
	gtx.Constraints = layout.Constraints{Max: iconSz}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(th, c.CornerBadge)
		// Scaled off the cell's own size (not a fixed constant) so a
		// bigger Shop Editor cell (see merchant_panel.go's cell-size
		// slider) actually makes the badge easier to read, anchored to
		// read the same as before at the pre-slider 80dp default.
		lbl.TextSize = th.TextSize * 12 / 16 * unit.Sp(BadgeScale(c.Size))
		lbl.Color = cornerBadgeTextColor
		return lbl.Layout(gtx)
	})
	call := macro.Stop()

	margin := gtx.Dp(unit.Dp(3))
	origin := image.Pt(iconSz.X-dims.Size.X-margin, iconSz.Y-dims.Size.Y-margin)
	off := op.Offset(origin).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
}

// drawTooltip renders the hover label just below the cell, deferred so it
// overlays anything painted after this cell (op.Defer preserves the current
// transform, so the label is positioned relative to this cell).
func (c IconCell) drawTooltip(gtx layout.Context, th *material.Theme, cell layout.Dimensions) {
	rec := op.Record(gtx.Ops)
	offset := op.Offset(image.Pt(0, cell.Size.Y+gtx.Dp(2)))
	offset.Add(gtx.Ops)

	gtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(tooltipWidth), 1e6)}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(th, c.Tooltip)
		lbl.Color = tooltipText
		return lbl.Layout(gtx)
	})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, tooltipBg, clip.Rect{Max: dims.Size}.Op())
	widget.Border{Color: tooltipEdge, Width: unit.Dp(1), CornerRadius: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	call.Add(gtx.Ops)

	op.Defer(gtx.Ops, rec.Stop())
}
