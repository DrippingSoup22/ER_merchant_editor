// Package widgets holds the small reusable Gio widgets the editor panels
// share: a dropdown Combo and a 64dp icon-button IconCell. They keep no
// reference to the catalog or app state, so panels drive them by value.
package widgets

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

// Combo is a dropdown selector: a clickable header showing the current value
// plus an arrow glyph, and a deferred overlay (op.Defer) that draws a bordered,
// scrollable list of options over whatever is laid out below it. The overlay
// closes on a second header click, on selecting an option, or on a click
// outside (a full-window scrim). Options are plain strings; selection is
// polled via Changed().
type Combo struct {
	options      []string
	labels       []string // optional menu display text, index-aligned with options; falls back to options
	headerLabels []string // optional compact header text, index-aligned with options; falls back to menu text
	overlayMinW  unit.Dp  // 0 = header width; lets a compact field keep a readable menu
	header       widget.Clickable
	scrim        widget.Clickable
	opts         []widget.Clickable // one per option, index-aligned with options
	list         widget.List
	open         bool
	sel          int
	changed      bool
}

var (
	comboBg      = color.NRGBA{R: 0x2A, G: 0x2A, B: 0x2C, A: 0xFF}
	comboHoverBg = color.NRGBA{R: 0x38, G: 0x38, B: 0x3C, A: 0xFF}
	comboBorder  = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
	comboSelBg   = color.NRGBA{R: 0x39, G: 0x45, B: 0x5A, A: 0xFF}
)

// SetOptions replaces the option list. The current selection is preserved by
// index when still in range, otherwise reset to the first option. Reallocating
// the per-option click state on a length change is intentional: a different
// option set has no meaningful in-flight click to carry over.
func (c *Combo) SetOptions(options []string) {
	c.SetOptionsWithLabels(options, nil)
}

// SetOptionsWithLabels is SetOptions plus an optional parallel display-text
// array (e.g. so a "" option, meaning "no filter", can show "All
// Categories" while Value() still reports "" to filtering code). labels must
// be the same length as options, or it's ignored and options are shown
// as-is.
func (c *Combo) SetOptionsWithLabels(options, labels []string) {
	c.SetOptionsWithDisplayLabels(options, labels, nil)
}

// SetOptionsWithDisplayLabels sets raw values plus separate menu and header
// labels. This is useful where the selected-field width must stay compact but
// the dropdown still needs to show an unambiguous full name. Either label list
// may be nil (or the wrong length), in which case it falls back gracefully.
func (c *Combo) SetOptionsWithDisplayLabels(options, labels, headerLabels []string) {
	c.options = options
	if len(labels) == len(options) {
		c.labels = labels
	} else {
		c.labels = nil
	}
	if len(headerLabels) == len(options) {
		c.headerLabels = headerLabels
	} else {
		c.headerLabels = nil
	}
	if len(c.opts) != len(options) {
		c.opts = make([]widget.Clickable, len(options))
	}
	if c.sel < 0 || c.sel >= len(options) {
		c.sel = 0
	}
}

// SetOverlayMinWidth lets the opened option list be wider than its header
// without changing the surrounding layout. Pass 0 to use the header width.
func (c *Combo) SetOverlayMinWidth(width unit.Dp) { c.overlayMinW = width }

// Options returns the current option list.
func (c *Combo) Options() []string { return c.options }

// Value returns the currently selected option ("" if there are none).
func (c *Combo) Value() string {
	if c.sel >= 0 && c.sel < len(c.options) {
		return c.options[c.sel]
	}
	return ""
}

// label returns the display text for option i: labels[i] if set, else the
// raw option value.
func (c *Combo) label(i int) string {
	if i >= 0 && i < len(c.labels) {
		return c.labels[i]
	}
	if i >= 0 && i < len(c.options) {
		return c.options[i]
	}
	return ""
}

func (c *Combo) headerLabel(i int) string {
	if i >= 0 && i < len(c.headerLabels) {
		return c.headerLabels[i]
	}
	return c.label(i)
}

// SetValue selects the first option equal to v, or the first option if none
// match. Does not raise Changed().
func (c *Combo) SetValue(v string) {
	for i, o := range c.options {
		if o == v {
			c.sel = i
			return
		}
	}
	c.sel = 0
}

// Changed reports (and clears) whether the selection changed via user click
// since the last call.
func (c *Combo) Changed() bool {
	ch := c.changed
	c.changed = false
	return ch
}

// Layout draws the header at the given width and, when open, the overlay.
func (c *Combo) Layout(gtx layout.Context, th *material.Theme, width unit.Dp) layout.Dimensions {
	if c.header.Clicked(gtx) {
		c.open = !c.open
	}
	for i := range c.options {
		if i < len(c.opts) && c.opts[i].Clicked(gtx) {
			if c.sel != i {
				c.sel = i
				c.changed = true
			}
			c.open = false
		}
	}
	// Drain every queued scrim click (not just one) so a second click landing
	// in the same frame can't leak through to re-toggle open next frame --
	// same reasoning as widgets.Modal's identical scrim-drain loops.
	for c.scrim.Clicked(gtx) {
		c.open = false
	}

	dims := c.header.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return c.layoutHeader(gtx, th, width)
	})
	if c.open && len(c.options) > 0 {
		overlayW := dims.Size.X
		if minW := gtx.Dp(c.overlayMinW); minW > overlayW {
			overlayW = minW
		}
		c.layoutOverlay(gtx, th, overlayW, dims.Size.Y)
	}
	return dims
}

func (c *Combo) layoutHeader(gtx layout.Context, th *material.Theme, width unit.Dp) layout.Dimensions {
	w := gtx.Dp(width)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w

	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(th, c.headerLabel(c.sel))
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Body1(th, arrowGlyph(c.open)).Layout(gtx)
			}),
		)
	})
	call := macro.Stop()
	dims.Size.X = w

	bg := comboBg
	if c.header.Hovered() {
		bg = comboHoverBg
		pointer.CursorPointer.Add(gtx.Ops)
	}
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	widget.Border{Color: comboBorder, Width: unit.Dp(1), CornerRadius: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

func (c *Combo) layoutOverlay(gtx layout.Context, th *material.Theme, width, headerH int) {
	// Scrim: a large transparent click-catcher drawn first (so it sits below
	// the dropdown list). A click anywhere on it closes the dropdown.
	{
		m := op.Record(gtx.Ops)
		const span = 8000
		off := op.Offset(image.Pt(-span/2, -span/2)).Push(gtx.Ops)
		sgtx := gtx
		sgtx.Constraints = layout.Exact(image.Pt(span, span))
		c.scrim.Layout(sgtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(span, span)}
		})
		off.Pop()
		op.Defer(gtx.Ops, m.Stop())
	}

	// Dropdown list, deferred so it draws over the content below the header.
	{
		m := op.Record(gtx.Ops)
		off := op.Offset(image.Pt(0, headerH)).Push(gtx.Ops)
		lgtx := gtx
		lgtx.Constraints.Min = image.Pt(width, 0)
		lgtx.Constraints.Max = image.Pt(width, gtx.Dp(unit.Dp(300)))
		c.layoutList(lgtx, th)
		off.Pop()
		op.Defer(gtx.Ops, m.Stop())
	}
}

func (c *Combo) layoutList(gtx layout.Context, th *material.Theme) layout.Dimensions {
	c.list.Axis = layout.Vertical
	macro := op.Record(gtx.Ops)
	dims := material.List(th, &c.list).Layout(gtx, len(c.options), func(gtx layout.Context, i int) layout.Dimensions {
		return c.opts[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// layout.Stack zeroes Constraints.Min before laying out Stacked
			// children (see gioui.org/layout/stack.go), so a Stack-based fill
			// here would only ever cover the label's own natural text width,
			// not the full row -- record the row directly instead (same
			// macro/fill/replay pattern layoutHeader uses) and force the
			// fill to the row's full available width.
			fullX := gtx.Constraints.Max.X
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(th, c.label(i))
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
			call := macro.Stop()
			dims.Size.X = fullX

			// Row background: selection wins, else a hover highlight so the
			// option under the cursor is visibly live.
			var bg *color.NRGBA
			switch {
			case i == c.sel:
				bg = &comboSelBg
			case c.opts[i].Hovered():
				bg = &comboHoverBg
			}
			if c.opts[i].Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			if bg != nil {
				paint.FillShape(gtx.Ops, *bg, clip.Rect{Max: dims.Size}.Op())
			}
			call.Add(gtx.Ops)
			return dims
		})
	})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, comboBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	widget.Border{Color: comboBorder, Width: unit.Dp(1)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

func arrowGlyph(open bool) string {
	if open {
		return "▴" // ▴
	}
	return "▾" // ▾
}
