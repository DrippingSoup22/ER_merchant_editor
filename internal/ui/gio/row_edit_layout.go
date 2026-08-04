package gio

// Rendering/layout pieces for the per-row edit bar, split out of
// row_edit_form.go (pure code motion, no logic change): the leaf widgets that
// DRAW the bar's header, field rows, multi-select preview grid, gate line and
// action buttons, plus the bar-specific button/pill chrome (barButton,
// disabledBarButton, actionButton, countPill) that has no caller outside the
// bar. The event handling and value resolution these draw from live in
// row_edit_form.go.

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
)

// countPill renders a small rounded badge (e.g. "14 items selected") beside
// the bar's title, with its own background so it reads as a distinct badge
// rather than part of the merchant name.
func countPill(gtx layout.Context, th *material.Theme, text string) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(th, text)
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	})
	call := macro.Stop()

	rr := dims.Size.Y / 2
	bg := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: rr, SW: rr, NE: rr, NW: rr}
	paint.FillShape(gtx.Ops, colorInputBg, bg.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// formHeader dispatches to the single- or multi-row header layout -- a
// single selected row keeps its full icon/name/tag treatment; N>1 rows
// collapse to a scrollable icon preview grid. Neither renders action buttons
// (those live in formActionsRow): both are pure item-identity displays.
func (s *State) formHeader(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	if len(rows) == 1 {
		return s.formHeaderSingle(gtx, th, rows[0])
	}
	return s.formMultiItemList(gtx, th, rows)
}

// formHeaderSingle shows the item identity (icon + name/tag), vertically
// centered. Reads s.draftEdits (the bar's own working copy), not
// PendingEdits.
func (s *State) formHeaderSingle(gtx layout.Context, th *material.Theme, row *catalog.Row) layout.Dimensions {
	entry := s.draftEdits[row.RowID]
	display := rowDisplayName(row)
	iconPath := row.IconPath
	if entry != nil && entry.ItemChange != nil {
		display = entry.ItemChange.DisplayName()
		iconPath = entry.ItemChange.IconPath
	}
	pending := entry != nil && entry.ItemChange != nil

	nameCol := []layout.FlexChild{
		layout.Rigid(material.Body1(th, display).Layout),
	}
	if pending {
		tag := "(pending swap)"
		if entry.ItemChange.IsLevelOnly() {
			tag = "(pending level change)"
		}
		nameCol = append(nameCol,
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, tag)
				lbl.Color = colorAmber
				return lbl.Layout(gtx)
			}),
		)
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return framedIcon(gtx, s.Icons.Get(iconPath), headerIconSize)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, nameCol...)
		}),
	)
}

// formActionsRow is the ALWAYS-present bottom action area of the
// Price/Quantity/Level card, for both a single row and a multi-selection:
// Undo swap(s), the gate action, Change item(s), Apply -- in exactly that
// left-to-right order, single or multi alike (X stays in the header,
// layoutRowEditPanel).
//
// Every button is ALWAYS rendered, never omitted (actionButton), even when
// it can't currently do anything -- an unavailable action greys out rather
// than disappearing, so the row's layout never shifts. A greyed-out button
// renders via disabledBarButton -- no widget.Clickable is laid out at all
// that frame, so it truly can't be clicked, not just visually muted.
//
// Laid out via wrapButtons, NOT a plain horizontal Flex: a Flex of Rigids
// squeezes its trailing children when they overrun the available width, and
// with the form now a fixed-width modal (2026-08-03) the labels really do
// overrun -- "Apply" rendered as an unreadable sliver, worse the wider the
// gate label got ("Unlock all (All Characters)"). Wrapping to a second line
// keeps every button at its natural size and leaves room for more buttons
// later (user intends to add one).
func (s *State) formActionsRow(th *material.Theme, rows []*catalog.Row) []layout.FlexChild {
	anyPending := false
	for _, row := range rows {
		if e := s.draftEdits[row.RowID]; e != nil && e.ItemChange != nil {
			anyPending = true
			break
		}
	}
	gateLabel, gateEnabled, gateBtn := s.gateActionSpec(rows)
	changeLabel := "Change item"
	if len(rows) > 1 {
		changeLabel = "Change items"
	}
	hasDraft := len(s.draftEdits) > 0 || len(s.draftGateEdits) > 0

	buttons := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return actionButton(gtx, th, &s.undoSwapBtn, "Undo swaps", anyPending)
		},
		func(gtx layout.Context) layout.Dimensions {
			return actionButton(gtx, th, gateBtn, gateLabel, gateEnabled)
		},
		func(gtx layout.Context) layout.Dimensions {
			return actionButton(gtx, th, &s.changeItemBtn, changeLabel, !s.Picking())
		},
		func(gtx layout.Context) layout.Dimensions {
			return actionButton(gtx, th, &s.applyDraftBtn, "Apply", hasDraft)
		},
	}
	return []layout.FlexChild{
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return wrapButtons(gtx, actionGapDp, buttons)
		}),
	}
}

// actionGapDp is the gap wrapButtons leaves between action buttons, both
// horizontally and between wrapped lines.
const actionGapDp = unit.Dp(8)

// wrapButtons lays widgets left-to-right, wrapping to a new line whenever
// the next one wouldn't fit in the available width, right-aligning each
// line (matching the fields above, which push their editors to the same
// right edge). Each child is measured at its NATURAL size -- constraints
// are relaxed to Min=0/Max=available before measuring -- so nothing is ever
// squeezed the way a Flex of Rigids squeezes its overflow.
//
// Gio has no built-in flow/wrap layout; this is the same measure-then-place
// (op.Record + op.Offset) approach formMultiItemList already hand-rolls for
// its icon grid.
func wrapButtons(gtx layout.Context, gap unit.Dp, children []layout.Widget) layout.Dimensions {
	if len(children) == 0 {
		return layout.Dimensions{}
	}
	avail := gtx.Constraints.Max.X
	gapPx := gtx.Dp(gap)

	// Measure every child once at its natural size.
	calls := make([]op.CallOp, len(children))
	sizes := make([]image.Point, len(children))
	cgtx := gtx
	cgtx.Constraints.Min = image.Point{}
	for i, w := range children {
		macro := op.Record(gtx.Ops)
		dims := w(cgtx)
		calls[i] = macro.Stop()
		sizes[i] = dims.Size
	}

	// Group into lines that fit within avail (always at least one child per
	// line, so an over-wide child overflows rather than looping forever).
	type line struct{ start, end, width, height int }
	var lines []line
	cur := line{start: 0}
	for i := range children {
		w := sizes[i].X
		next := w
		if i > cur.start {
			next += gapPx
		}
		if i > cur.start && cur.width+next > avail {
			cur.end = i
			lines = append(lines, cur)
			cur = line{start: i, width: w, height: sizes[i].Y}
			continue
		}
		cur.width += next
		if sizes[i].Y > cur.height {
			cur.height = sizes[i].Y
		}
	}
	cur.end = len(children)
	lines = append(lines, cur)

	// Place: each line right-aligned, children vertically centered in it.
	y := 0
	total := image.Point{X: avail}
	for li, ln := range lines {
		if li > 0 {
			y += gapPx
		}
		x := avail - ln.width
		if x < 0 {
			x = 0
		}
		for i := ln.start; i < ln.end; i++ {
			if i > ln.start {
				x += gapPx
			}
			offY := y + (ln.height-sizes[i].Y)/2
			stack := op.Offset(image.Pt(x, offY)).Push(gtx.Ops)
			calls[i].Add(gtx.Ops)
			stack.Pop()
			x += sizes[i].X
		}
		y += ln.height
	}
	total.Y = y
	return layout.Dimensions{Size: total}
}

// actionButton renders btn as a normal barButton when enabled, or a
// greyed-out, non-interactive disabledBarButton at the SAME size when not
// -- so a currently-unavailable action reads as unavailable without ever
// disappearing or shifting formActionsRow's layout.
func actionButton(gtx layout.Context, th *material.Theme, btn *widget.Clickable, label string, enabled bool) layout.Dimensions {
	if enabled {
		return barButton(th, btn, label).Layout(gtx)
	}
	return disabledBarButton(gtx, th, label)
}

// disabledBarButton is pending_edits.go's disabledButtonChrome treatment (a
// filled rounded rect + muted text, no widget.Clickable laid out at all --
// "swallows nothing and reacts to nothing"), sized to match barButton
// exactly (same 6dp/12dp inset + 14/16 text scale) so toggling a button
// between enabled/disabled never changes formActionsRow's height or width.
func disabledBarButton(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	lbl := material.Body2(th, label)
	lbl.TextSize = th.TextSize * 14 / 16
	return disabledButtonChrome(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}, lbl)
}

// formMultiItemIconSize is the icon size used in the multi-select preview
// grid -- kept equal to headerIconSize so a single icon row matches a
// single-row selection's own icon exactly (part of the height-matching
// behavior below).
const formMultiItemIconSize = headerIconSize

// formMultiItemListHeight caps the multi-select preview grid's height so a
// LARGE selection scrolls instead of pushing the price/quantity fields (and
// the stock grid above it) further down. A ceiling, not a floor:
// formMultiItemList sizes to the selection's actual content height up to
// this cap, so a small selection (the common case) never claims more
// vertical space than its icons need.
const formMultiItemListHeight = unit.Dp(140)

// formMultiItemList renders every selected row as a small icon in a
// wrapping grid; each item's name (plus a pending-swap/level tag) moves to
// the hover tooltip instead of an inline label.
//
// Sized to the selection's OWN content height (gridRows * cellPx, plus one
// gap between rows but never a trailing one after the last), capped at
// formMultiItemListHeight -- not a fixed height regardless of content. A
// single-row selection (the common case: all icons fitting on one line)
// then comes out exactly formMultiItemIconSize tall, matching
// formHeaderSingle's own icon exactly, so switching between a 1-item and a
// several-item selection doesn't visibly change the bar's height.
func (s *State) formMultiItemList(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	s.formItemList.Axis = layout.Vertical

	cellPx := gtx.Dp(formMultiItemIconSize)
	gapPx := gtx.Dp(unit.Dp(4))
	avail := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(12)) // room for the scrollbar
	cols := (avail + gapPx) / (cellPx + gapPx)
	if cols < 1 {
		cols = 1
	}
	gridRows := (len(rows) + cols - 1) / cols

	height := gridRows * cellPx
	if gridRows > 1 {
		height += (gridRows - 1) * gapPx
	}
	if max := gtx.Dp(formMultiItemListHeight); height > max {
		height = max
	}
	gtx.Constraints.Max.Y = height
	gtx.Constraints.Min.Y = height

	return material.List(th, &s.formItemList).Layout(gtx, gridRows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
		children := make([]layout.FlexChild, 0, cols*2)
		for c := 0; c < cols; c++ {
			idx := rowIdx*cols + c
			if idx >= len(rows) {
				break
			}
			row := rows[idx]
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.formMultiItemIcon(gtx, th, row)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			)
		}
		rowChildren := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			}),
		}
		if rowIdx < gridRows-1 {
			rowChildren = append(rowChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
	})
}

// formMultiItemIcon is one cell of the multi-select preview grid: the
// row's current-or-drafted item icon, with the full name (plus a pending-
// swap/level tag) as its hover tooltip.
func (s *State) formMultiItemIcon(gtx layout.Context, th *material.Theme, row *catalog.Row) layout.Dimensions {
	display := rowDisplayName(row)
	iconPath := row.IconPath
	if e := s.draftEdits[row.RowID]; e != nil && e.ItemChange != nil {
		display = e.ItemChange.DisplayName()
		iconPath = e.ItemChange.IconPath
		if e.ItemChange.IsLevelOnly() {
			display += " (pending level change)"
		} else {
			display += " (pending swap)"
		}
	}
	cell := components.IconCell{
		Img:     s.Icons.Get(iconPath),
		Size:    formMultiItemIconSize,
		Tooltip: display,
	}
	return cell.Layout(gtx, th, s.formItemCell(row.RowID))
}

func (s *State) formPriceRow(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	hint := ""
	if s.formPriceMixed {
		hint = "Mixed"
	}
	return labeledEditor(gtx, th, batchPriceIcon(rows), batchPriceLabel(rows), &s.priceEditor, hint)
}

func (s *State) formQtyRow(gtx layout.Context, th *material.Theme) layout.Dimensions {
	hint := ""
	if s.formQtyMixed {
		hint = "Mixed"
	}
	return labeledEditor(gtx, th, stockIcon, "Quantity (-1 = unlimited, max 255)", &s.qtyEditor, hint)
}

func (s *State) formLevelRow(gtx layout.Context, th *material.Theme, maxLvl int) layout.Dimensions {
	hint := ""
	if s.formLevelMixed {
		hint = "Mixed"
	}
	return labeledEditor(gtx, th, nil, fmt.Sprintf("Weapon level (max +%d)", maxLvl), &s.levelFormEditor, hint)
}

// fieldIconSize is the side length of the small glyph shown before a
// field's label (the Price field's real currency icon, stockIcon), lining
// the labels up visually with what they represent instead of relying on
// text alone.
const fieldIconSize = unit.Dp(14)

// stockIcon draws a small coin-sack glyph standing in for a merchant's
// stock/quantity, shown before the Quantity field's label.
func stockIcon(gtx layout.Context) layout.Dimensions {
	return fixedIcon(gtx, stockSackImg, fieldIconSize)
}

// labeledEditor lays out an optional leading icon (nil = none), a label,
// and a fixed-width boxed number editor pushed to the row's right edge by a
// Flexed spacer (so every interactive control in the bar lines up on the
// right rather than wherever its label ends). hint is the editor's
// placeholder text, shown only while its own text is empty -- "" for the
// normal single-value case, "Mixed" when a multi-selection's rows disagree
// on this field (see syncFormEditors).
func labeledEditor(gtx layout.Context, th *material.Theme, icon layout.Widget, label string, ed *widget.Editor, hint string) layout.Dimensions {
	children := []layout.FlexChild{}
	if icon != nil {
		children = append(children,
			layout.Rigid(icon),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		)
	}
	children = append(children,
		layout.Rigid(material.Body2(th, label).Layout),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return boxed(gtx, th, material.Editor(th, ed, hint).Layout)
		}),
	)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// headerIconSize is the docked bar's item-icon thumbnail size -- sized to
// read well with framedIcon's inventory-slot border.
const headerIconSize = unit.Dp(40)

// barButton is a single consistent medium-sized button style for every
// action in the row-edit bar (Apply/X, Change item, Undo swap(s),
// Unlock/Lock, Unlock all).
func barButton(th *material.Theme, btn *widget.Clickable, label string) material.ButtonStyle {
	b := material.Button(th, btn, label)
	b.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}
	b.TextSize = th.TextSize * 14 / 16
	b.CornerRadius = 4
	return b
}
