package gio

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"sort"
	"strconv"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
)

// Merchant-cell border colors, in precedence order (see cellBorder). Locked
// is no longer a border color (2026-07-29) -- see components.IconCell.Locked's
// corner badge instead.
var (
	borderSelected = color.NRGBA{R: 0x5A, G: 0xA0, B: 0xFA, A: 0xFF} // #5AA0FA
	borderPending  = color.NRGBA{R: 0xF0, G: 0xB4, B: 0x50, A: 0xFF} // #F0B450
	borderWarn     = color.NRGBA{R: 0xDC, G: 0x5A, B: 0x5A, A: 0xFF} // #DC5A5A
)

// layoutMerchantPanel is the right panel: before a save loads, just a banner;
// after, a merchant selector + category filter over a 6-wide grid of that
// merchant's rows. Clicking a cell toggles row selection.
func (s *State) layoutMerchantPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	title := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return material.H6(th, "Merchant stock").Layout(gtx)
	})

	if s.Busy() || !s.Catalog.Loaded() {
		msg := "Open a save file to browse merchants."
		if s.Busy() {
			msg = s.BusyMsg()
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			title,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Body1(th, msg).Layout(gtx)
			}),
		)
	}

	s.syncMerchants()
	if s.MerchantCombo.Changed() {
		s.fetchMerchantRows()
	}
	if s.MerchantCategoryCombo.Changed() {
		s.MerchantList.Position = layout.Position{}
	}
	// Reconciled every frame regardless of whether the row-edit bar is
	// currently shown -- see state.go's ensureDraft doc comment for why
	// this single check covers both "selection changed" and "bar closed
	// then reopened on the same rows."
	s.ensureDraft()
	rows := s.visibleRows()

	children := []layout.FlexChild{
		title,
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			filterChildren := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.MerchantCombo.Layout(gtx, th, unit.Dp(240))
				}),
			}
			// Hidden entirely when this merchant's stock is already all in one
			// category -- see fetchMerchantRows. When shown, use the same
			// top-level categories and ordering as the Catalog panel rather
			// than its much narrower subcategory taxonomy.
			if s.merchantCategoryHas {
				filterChildren = append(filterChildren,
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.MerchantCategoryCombo.Layout(gtx, th, unit.Dp(160))
					}),
				)
			}
			filterChildren = append(filterChildren,
				layout.Flexed(1, flexSpacer),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.merchantViewModeButton(gtx, th, &s.merchantEditModeBtn, "Edit layout", false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.merchantViewModeButton(gtx, th, &s.merchantPreviewBtn, "Game preview", true)
				}),
			)
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, filterChildren...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedMinHeight(gtx, filterCountRowHeight, func(gtx layout.Context) layout.Dimensions {
				return s.layoutStockCountRow(gtx, th, len(rows))
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			dims := s.layoutRowGrid(gtx, th, rows)
			// Presses inside the grid keep the row-edit bar open (toggling/
			// re-targeting is handled by the cells' own click handling).
			s.pressListener(gtx, &s.merchantAreaTag, dims.Size, &s.merchantAreaHit)
			return dims
		}),
	}
	// The per-row edit form is a floating modal now (2026-08-03, see
	// window.go's layoutRowEditOverlay) -- not laid out inline here anymore,
	// so the grid no longer shrinks to make room for it.
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// merchantViewModeButton uses the header's selected-tab chrome at the regular
// button size. Keeping both buttons in the filter row makes them feel like a
// direct display choice for the selected merchant, rather than a tiny global
// control competing with the panel title.
func (s *State) merchantViewModeButton(gtx layout.Context, th *material.Theme, btn *widget.Clickable, label string, preview bool) layout.Dimensions {
	if btn.Clicked(gtx) {
		if s.merchantGamePreview != preview {
			s.merchantGamePreview = preview
			s.MerchantList.Position = layout.Position{}
		}
		s.setFooterStatus(s.merchantModeHint())
	}
	b := material.Button(th, btn, label)
	b.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}
	b.TextSize = th.TextSize
	b.CornerRadius = 3
	if s.merchantGamePreview == preview {
		b.Background = colorAccent
		b.Color = th.Palette.ContrastFg
	} else {
		b.Background = colorInputBg
		b.Color = colorFg
	}
	return b.Layout(gtx)
}

// layoutStockCountRow draws the stock count and a right-aligned Edit button
// when rows are selected. The grid-mode explanation lives in the shared
// footer status area, leaving this row focused on stock controls.
func (s *State) layoutStockCountRow(gtx layout.Context, th *material.Theme, stock int) layout.Dimensions {
	if s.editBtn.Clicked(gtx) {
		s.showRowEditor = true
		n := len(s.SelectedRowIDs)
		if n == 1 {
			s.setFooterStatus("Edit the selected item, then apply to stage the change")
		} else {
			s.setFooterStatus(fmt.Sprintf("Edit %d selected items, then apply to stage the changes", n))
		}
	}
	sideControls := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.Body2(th, fmt.Sprintf("%d items in stock", stock)).Layout),
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(s.SelectedRowIDs) == 0 || s.showRowEditor {
					return layout.Dimensions{}
				}
				// barButton = the app's standard medium action-button style (Apply/
				// Change item/etc.), so this reads clearly and matches the rest of
				// the UI rather than the tiny formButton it used to be.
				n := len(s.SelectedRowIDs)
				label := "Edit selected"
				if n > 1 {
					label = fmt.Sprintf("Edit %d items", n)
				}
				dims := barButton(th, &s.editBtn, label).Layout(gtx)
				s.pressListener(gtx, &s.editBtnAreaTag, dims.Size, &s.editBtnHit)
				return dims
			}),
		)
	}
	return sideControls(gtx)
}

func (s *State) merchantModeHint() string {
	if s.merchantGamePreview {
		return "Game preview — Elden Ring groups this stock by category and subcategory"
	}
	return "Edit layout — replaced or swapped items remain in their exact stock slots"
}

// merchantLabel builds a merchant combo entry: just the name, or (behind
// Settings' "Show merchant row counts" toggle, off by default) the name
// plus its editable row count.
func merchantLabel(m catalog.Merchant, showCounts bool) string {
	if !showCounts {
		return m.Name
	}
	return fmt.Sprintf("%s (%d)", m.Name, m.EditableRowCount)
}

// merchantHeaderLabel is the short, always-readable label in the fixed-width
// merchant filter. The full canonical name remains in the opened dropdown;
// only the repeating "Merchant" word and long location qualifiers are
// compacted here. This avoids turning a location into an ambiguous truncation.
func merchantHeaderLabel(name string) string {
	if !strings.Contains(name, " Merchant - ") {
		return name
	}
	parts := strings.SplitN(name, " Merchant - ", 2)
	location := strings.NewReplacer(
		"North Limgrave", "N. Limgrave",
		"East Limgrave", "E. Limgrave",
		"East Weeping Peninsula", "E. Weeping",
		"Liurnia of the Lakes", "Liurnia",
		"North Liurnia", "N. Liurnia",
		"Caelid (Aeonia Swamp)", "Aeonia Swamp",
		"South Caelid", "S. Caelid",
		"Academy of Raya Lucaria", "Raya Lucaria",
		"Mountaintops of the Giants", "Mountaintops",
		"Weeping Peninsula", "Weeping",
		"Ainsel River", "Ainsel",
		"Siofra River", "Siofra",
	).Replace(parts[1])
	return parts[0] + " — " + location
}

// syncMerchants (re)populates the merchant combo whenever the loaded save
// changes OR the row-count display setting is toggled (labels-only
// rebuild -- current selection is preserved by name, unlike a real path
// change which always resets to the top), then ensures the row cache
// matches the current selection.
func (s *State) syncMerchants() {
	pathChanged := s.merchantsPath != s.Catalog.SavePath()
	if pathChanged || s.merchantsShowCounts != s.Settings.ShowMerchantRowCounts {
		merchants, err := s.Catalog.ListMerchants()
		if err != nil {
			s.MerchantCombo.SetOptions(nil)
			s.merchantsPath = s.Catalog.SavePath()
			s.merchantsShowCounts = s.Settings.ShowMerchantRowCounts
			return
		}
		prevName := s.MerchantCombo.Value()
		options := make([]string, len(merchants))
		labels := make([]string, len(merchants))
		headerLabels := make([]string, len(merchants))
		selectIdx := 0
		for i, m := range merchants {
			options[i] = m.Name
			labels[i] = merchantLabel(m, s.Settings.ShowMerchantRowCounts)
			headerLabels[i] = merchantHeaderLabel(m.Name)
			if !pathChanged && m.Name == prevName {
				selectIdx = i
			}
		}
		s.MerchantCombo.SetOptionsWithDisplayLabels(options, labels, headerLabels)
		s.MerchantCombo.SetOverlayMinWidth(unit.Dp(360))
		if len(options) > 0 {
			s.MerchantCombo.SetValue(options[selectIdx])
		}
		s.merchantsPath = s.Catalog.SavePath()
		s.merchantsShowCounts = s.Settings.ShowMerchantRowCounts
		if pathChanged {
			s.lastMerchant = ""
		}
	}
	if s.MerchantCombo.Value() != s.lastMerchant {
		s.fetchMerchantRows()
	}
}

// fetchMerchantRows loads the selected merchant's rows, refreshes the
// subcategory filter from them, and clears the row selection.
func (s *State) fetchMerchantRows() {
	name := s.MerchantCombo.Value()
	s.lastMerchant = name
	s.clearRowSelection()
	// New merchant = new list; start at its top.
	s.MerchantList.Position = layout.Position{}

	if name == "" {
		s.MerchantRows = nil
	} else if rows, err := s.Catalog.MerchantRows(name); err != nil {
		s.MerchantRows = nil
	} else {
		// Material-locked rows (Enia's Remembrance hand-ins) are deliberately
		// not editable, so they aren't shown at all (user decision 2026-07-28).
		// The combo's count label matches (EditableRowCount).
		shown := rows[:0:0]
		for _, r := range rows {
			if !r.MaterialLocked {
				shown = append(shown, r)
			}
		}
		s.MerchantRows = shown
	}

	set := map[string]struct{}{}
	for _, r := range s.MerchantRows {
		if r.Category != "" {
			set[r.Category] = struct{}{}
		}
	}
	categories := make([]string, 0, len(set))
	for k := range set {
		categories = append(categories, k)
	}
	options, labels := orderedCategoryOptions(categories)
	s.MerchantCategoryCombo.SetOptionsWithLabels(options, labels)
	s.MerchantCategoryCombo.SetValue("")
	s.merchantCategoryHas = len(categories) > 1
}

// visibleRows applies the stock controls. Edit layout is deliberately kept in
// raw ShopLineupParam order so a sequence of drops never jumps around; Game
// preview is the explicit opt-in view that uses Elden Ring's category-first
// item-menu sort.
func (s *State) visibleRows() []*catalog.Row {
	category := s.MerchantCategoryCombo.Value()
	out := make([]*catalog.Row, 0, len(s.MerchantRows))
	for _, r := range s.MerchantRows {
		if category != "" && r.Category != category {
			continue
		}
		out = append(out, r)
	}
	if !s.merchantGamePreview {
		sort.Slice(out, func(i, j int) bool { return out[i].RowID < out[j].RowID })
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		aID, aOK := s.merchantMenuItemID(a)
		bID, bOK := s.merchantMenuItemID(b)
		if aRank, bRank := s.merchantMenuCategoryRank(aID, aOK), s.merchantMenuCategoryRank(bID, bOK); aRank != bRank {
			return aRank < bRank
		}
		if aOK && bOK && aID != bID {
			if s.Catalog.ItemComesBeforeInMerchantMenu(aID, bID) {
				return true
			}
			if s.Catalog.ItemComesBeforeInMerchantMenu(bID, aID) {
				return false
			}
		}
		return a.RowID < b.RowID
	})
	return out
}

// merchantMenuCategoryRank is the missing first tier in the in-game menu
// comparison. sortId/sortGroupId are local to each EquipParam table, so a
// weapon Dagger and a Good such as Rune Arc can both have group 10; comparing
// those values directly incorrectly put weapons before the Tools block.
func (s *State) merchantMenuCategoryRank(itemID int64, ok bool) int {
	if !ok {
		return unknownCategoryRank
	}
	if item := s.Catalog.ItemByID(itemID); item != nil {
		return categoryRank(item.Category)
	}
	return unknownCategoryRank
}

// merchantMenuItemID resolves the identity the game will use to sort this
// merchant row. A row-edit draft takes precedence over a pending edit through
// effectiveRowEdit, exactly as it does for the icon and tooltip.
func (s *State) merchantMenuItemID(row *catalog.Row) (int64, bool) {
	if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
		return s.Catalog.ResolveItemID(e.ItemChange.EquipID, int(e.ItemChange.EquipType))
	}
	if row.ItemID == nil {
		return 0, false
	}
	return *row.ItemID, true
}

func (s *State) layoutRowGrid(gtx layout.Context, th *material.Theme, rows []*catalog.Row) layout.Dimensions {
	// While a drag is in flight, highlight exactly the cells the drop would
	// replace (from the hovered cell onward, locked rows skipped), not every
	// eligible cell.
	span := s.dropSpan(rows)
	return layoutGrid(gtx, th, &s.MerchantList, len(rows), s.merchantCellSize(), func(gtx layout.Context, i int) layout.Dimensions {
		row := rows[i]
		cs := s.rowCell(row.RowID)
		s.handleRowClick(gtx, cs, rows, i)
		// Every merchant cell is a drop target for dragged catalog items.
		s.handleRowDrop(gtx, cs, rows, i)

		border, borderW := s.cellBorder(row), unit.Dp(0)
		if span[row.RowID] {
			border = &borderPending // would-be-replaced cell: amber wins during a drag
		}
		// A staged (or drafted-but-not-yet-applied, see effectiveRowEdit)
		// item swap shows the NEW item's icon (the row's own icon would
		// misleadingly keep showing what's being replaced).
		iconPath := row.IconPath
		if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
			iconPath = e.ItemChange.IconPath
		}
		_, qty := s.rowPriceQty(row)
		cell := components.IconCell{
			Img:         s.Icons.Get(iconPath),
			Size:        s.merchantCellSize(),
			Border:      border,
			BorderWidth: borderW,
			Locked:      s.rowLockedForDisplay(row),
			CornerBadge: qtyBadgeText(qty),
			Footer:      s.rowPriceFooter(th, row),
			Tooltip:     s.rowTooltip(row),
		}
		render := func(gtx layout.Context) layout.Dimensions {
			return cell.Layout(gtx, th, &cs.click)
		}

		// Every merchant cell is ALSO a drag source (internal swap, rowsMIME)
		// -- mirrors the catalog grid's identical press-vs-drag contract
		// (Draggable needs real movement past its slop threshold before
		// Dragging() latches, so a plain click/release still only reaches
		// cs.click above and keeps handleRowClick's selection semantics; a
		// press-and-move starts a drag). Dragging a cell that's part of the
		// current multi-selection carries the WHOLE selection (rowDragPayload)
		// for a positional group swap; dragging any other cell carries just
		// itself. The drag never MUTATES the selection (unlike the catalog
		// grid, which selects-on-drag) so it can't churn the docked edit form
		// mid-gesture -- see dragFromRow's doc comment.
		cs.drag.Type = rowsMIME
		if mime, ok := cs.drag.Update(gtx); ok {
			payload := s.rowDragPayload(row.RowID)
			cs.drag.Offer(gtx, mime, io.NopCloser(strings.NewReader(payload)))
		}
		var ghost layout.Widget
		if cs.drag.Dragging() {
			if s.dragSrc != cs {
				s.dragSrc = cs
				s.dragCount = s.rowDragIDCount(row.RowID)
				s.dragFromRow = row.RowID
			}
			if s.transferInit {
				img, count := s.Icons.Get(iconPath), s.dragCount
				ghost = func(gtx layout.Context) layout.Dimensions {
					return dragGhost(gtx, th, img, count)
				}
			}
		}
		// PassOp: Draggable registers its drag area ON TOP of the cell's click
		// area, same reasoning as the catalog grid's identical comment.
		dragPass := pointer.PassOp{}.Push(gtx.Ops)
		dims := cs.drag.Layout(gtx, render, ghost)
		dragPass.Pop()

		// Register the cell's hit area under the cellState tag so the transfer
		// router routes DataEvent to it (see handleRowDrop). PassOp: this area
		// sits on top of the cell's click area and input areas are opaque by
		// default — without pass-through the click underneath goes dead.
		pass := pointer.PassOp{}.Push(gtx.Ops)
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		event.Op(gtx.Ops, cs)
		area.Pop()
		pass.Pop()

		if rightClickTarget(gtx, cs, dims.Size) {
			if id, ok := s.rowEffectiveItemID(row); ok {
				s.openItemInfo(id, s.rowEffectiveItemLevel(row))
			}
		}
		return dims
	})
}

// rowEffectiveItemID resolves row's CURRENTLY DISPLAYED item to a catalog
// item id -- a staged/drafted swap's target item (effectiveRowEdit) if one
// exists, matching the icon/tooltip's own precedence, else the row's own
// decoded ItemID. Used by the item-info popup so right-clicking a row with
// a pending swap shows info for the NEW item, not the one being replaced.
func (s *State) rowEffectiveItemID(row *catalog.Row) (int64, bool) {
	if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
		return s.Catalog.ResolveItemID(e.ItemChange.EquipID, int(e.ItemChange.EquipType))
	}
	if row.ItemID != nil {
		return *row.ItemID, true
	}
	return 0, false
}

// rowEffectiveItemLevel is the "+N" reinforcement level of the item row
// currently displays -- a staged swap's level if one exists (matching
// rowEffectiveItemID's precedence), else the row's own decoded level. Feeds
// the item-info popup so a leveled weapon's card shows its scaled stats.
func (s *State) rowEffectiveItemLevel(row *catalog.Row) int {
	if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
		return int(e.ItemChange.Level)
	}
	return int(row.Level)
}

// handleRowClick drains one merchant cell's click events and updates the
// multi-selection per its modifiers -- ctrl/cmd-click toggles, shift-click
// range-selects (in the currently visible row order), a plain click
// replaces the selection with just this row. Mirrors handleCatalogClicks.
func (s *State) handleRowClick(gtx layout.Context, cs *cellState, rows []*catalog.Row, i int) {
	for {
		ev, ok := cs.click.Update(gtx)
		if !ok {
			break
		}
		// Selecting NEVER opens the edit form (plain or Ctrl/Shift) -- a
		// selection is usable as-is for a drag-swap; the form opens only via
		// the explicit "Edit (N)" button (see layoutMerchantPanel).
		switch {
		case ev.Modifiers.Contain(key.ModShortcut):
			s.toggleSelectRow(rows[i].RowID)
		case ev.Modifiers.Contain(key.ModShift):
			s.selectRowRange(rows, i)
		default:
			s.selectRowPlain(rows[i].RowID)
		}
	}
}

// handleRowDrop drains transfer events for one merchant cell and, on a drop
// (DataEvent), stages the payload's item swaps from this visible row onward.
// The drag-in-flight tint is NOT driven from Initiate/Cancel here — those
// arrive per-target and proved unreliable as a global signal (the tint stuck
// on); the source's Draggable.Dragging() is the ground truth instead (see
// State.Layout).
func (s *State) handleRowDrop(gtx layout.Context, cs *cellState, rows []*catalog.Row, i int) {
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: cs, Type: itemsMIME})
		if !ok {
			break
		}
		switch e := ev.(type) {
		case transfer.InitiateEvent:
			// The router sends this on the first MOVE while pressed over a
			// source — the signal that a drag instance really started.
			s.transferInit = true
		case transfer.DataEvent:
			rc := e.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			s.applyDrop(string(data), rows, i)
			// Force a clean follow-up frame so EVERY affected cell re-resolves
			// its icon/price/quantity from the freshly staged edit -- cells
			// that already laid out this frame (before this drop event was
			// delivered) would otherwise stay stale until an unrelated event
			// or the icon notifier happened to trigger the next redraw.
			s.invalidate()
		}
	}
	// A second, distinct MIME for merchant-cell-origin drags (internal row
	// swap, see rowsMIME's own doc comment) -- registered on the same target
	// tag, so one cell accepts either payload kind and dispatches to the
	// right staging function.
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: cs, Type: rowsMIME})
		if !ok {
			break
		}
		switch e := ev.(type) {
		case transfer.InitiateEvent:
			s.transferInit = true
		case transfer.DataEvent:
			rc := e.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			// The payload is the drag's ordered source row ids (one cell, or a
			// whole multi-selection). Pair them positionally with the target
			// cells from this drop cell (i) forward; excess sources with no
			// counterpart are left untouched.
			s.applyRowSwapPayload(parseItemPayload(string(data)), rows, i)
			// Deselect after a swap: the source slots now hold the swapped-in
			// items, so leaving them selected is confusing (user request).
			// clearRowSelection also invalidates, refreshing both sides at once.
			s.clearRowSelection()
		}
	}
	// Hover tracking for the drop-span highlight: the router keeps
	// mime-matching transfer targets in the hit set during a drag and
	// delivers Enter/Leave to them (verified in io/input/pointer.go's
	// deliverEnterLeaveEvents), provided the tag also has a pointer filter.
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: cs, Kinds: pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok {
			switch pe.Kind {
			case pointer.Enter:
				s.dropHoverRow = rows[i].RowID
			case pointer.Leave:
				if s.dropHoverRow == rows[i].RowID {
					s.dropHoverRow = noRow
				}
			}
		}
	}
}

// dropSpan resolves which visible rows a drop at the current hover position
// would affect. Two shapes: a merchant-row-origin drag (dragFromRow set --
// internal swap) affects dragCount cells from the hovered one forward (the
// cells the dragged single row or multi-selection would positionally swap
// into); a catalog-origin drag affects rows from the hovered one onward,
// consuming dragCount items and skipping material-locked rows (a multi-item
// fill). Empty when no drag is in flight or the cursor isn't over the grid.
func (s *State) dropSpan(rows []*catalog.Row) map[int64]bool {
	if !s.dndActive || !s.transferInit || s.dropHoverRow == noRow {
		return nil
	}
	start := -1
	for i, r := range rows {
		if r.RowID == s.dropHoverRow {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	if s.dragFromRow != noRow {
		// Internal swap: highlight the dragCount target cells from the hovered
		// cell forward (the cells the group would swap into). A single-cell
		// drag onto its own source is a no-op, so it highlights nothing.
		if s.dragCount <= 1 && s.dragFromRow == s.dropHoverRow {
			return nil
		}
		span := make(map[int64]bool, s.dragCount)
		for k := 0; k < s.dragCount && start+k < len(rows); k++ {
			span[rows[start+k].RowID] = true
		}
		return span
	}
	if s.dragCount == 0 {
		return nil
	}
	span := make(map[int64]bool, s.dragCount)
	left := s.dragCount
	for i := start; i < len(rows) && left > 0; i++ {
		if rows[i].MaterialLocked {
			continue
		}
		span[rows[i].RowID] = true
		left--
	}
	return span
}

// Warning prefixes filtered out of red-square treatment. Sourced from
// catalog's own exported constants (not hand-copied literals) so a wording
// change in rowWarnings can't silently desync this list from what it's
// actually supposed to match -- this exact class of bug shipped once
// before (see docs/EDITOR.md, cellBorder's v4 locked-state regression):
//   - the name/icon override is normal vanilla display (e.g. "Note: X" rows)
//     and staged item swaps reset the override fields automatically;
//   - the material-exchange warning describes rows that are deliberately not
//     editable, which get their own gray/dimmed treatment instead;
//   - the event-flag gate is save-independent (regulation.bin only knows a
//     row HAS a gate, not whether any given character has cleared it) --
//     the purple border already conveys the character-aware locked state
//     (see rowLockedForDisplay); once unlocked, this text shouldn't also
//     paint the row red.
var nonHazardWarningPrefixes = []string{
	catalog.WarnPrefixNameIconOverride,
	catalog.WarnPrefixMaterialExchange,
	catalog.WarnPrefixEventGated,
}

// hazardWarnings filters a row's warnings down to the ones worth a red square.
func hazardWarnings(row *catalog.Row) []string {
	var out []string
	for _, w := range row.Warnings {
		hazard := true
		for _, p := range nonHazardWarningPrefixes {
			if strings.HasPrefix(w, p) {
				hazard = false
				break
			}
		}
		if hazard {
			out = append(out, w)
		}
	}
	return out
}

// cellBorder resolves the border color by precedence: selected > pending >
// has-warnings > none. (Material-locked rows never reach the grid —
// filtered in fetchMerchantRows.) Locked-for-character is drawn as a
// separate corner badge (rowLockedForDisplay), not a border, so it can
// layer with any of these instead of being mutually exclusive.
func (s *State) cellBorder(row *catalog.Row) *color.NRGBA {
	switch {
	case s.isRowSelected(row.RowID):
		return &borderSelected
	case s.effectiveRowEdit(row.RowID) != nil:
		return &borderPending
	case len(hazardWarnings(row)) > 0:
		return &borderWarn
	default:
		return nil
	}
}

// rowLockedForDisplay reports whether a gated row should show as locked
// (corner padlock badge) in the Shop Editor grid. With no character selected,
// every gated row shows locked (v6: restored this fallback — v5 had
// dropped it, but the user found that left whatever the *previously*
// selected character had unlocked stuck looking normal after
// deselecting, which reads as a bug: "no character" must mean nothing is
// known to be unlocked). With a character selected, it's bound to THAT
// character's actual unlock state (committed on disk, a staged-but-unsaved
// toggle from the Characters view, OR an in-progress row-edit-bar gate
// DRAFT for this row's UnlockFlag -- draftEffectiveRowUnlocked,
// row_edit_form.go -- so clicking "Unlock all"/Unlock in the bar drops the
// badge immediately, and it comes back the moment the bar closes without
// Apply, 2026-07-29: was previously bar-preview-only, a known gap the user
// then actually hit: "if i click unlock all the lock icon must disappear
// from the items... it should get back if i don't click apply").
func (s *State) rowLockedForDisplay(row *catalog.Row) bool {
	if row.UnlockFlag == 0 {
		return false
	}
	if s.SelectedChar < 0 {
		return true
	}
	unlocked, known := s.draftEffectiveRowUnlocked(row)
	return !(known && unlocked)
}

// rowTooltip builds the multi-line hover text for a merchant cell. A staged
// item swap leads with the NEW item (matching the swapped icon the cell
// shows) — hovering must not resurface the item being replaced as if it were
// still current.
func (s *State) rowTooltip(row *catalog.Row) string {
	label := row.DisplayName()
	if label == "" {
		label = row.Label
	}
	if label == "" {
		label = s.lastMerchant
	}
	lines := []string{label}
	if e := s.effectiveRowEdit(row.RowID); e != nil && e.ItemChange != nil {
		staged := fmt.Sprintf("staged replacement of %s", e.ItemChange.FromName)
		if e.ItemChange.IsLevelOnly() {
			staged = "staged level change"
		}
		lines = []string{e.ItemChange.DisplayName(), staged}
	}
	// The locked state is worth surfacing (the corner badge alone doesn't
	// say why). The raw flag number and internal data-quality warnings used
	// to be appended here in Debug mode; that mode is gone (2026-08-03) and
	// they were developer bookkeeping, so they're simply not shown.
	if s.rowLockedForDisplay(row) {
		if name := s.selectedCharName(); name != "" {
			lines = append(lines, fmt.Sprintf("Locked for %s", name))
		} else {
			lines = append(lines, "Locked (gated item)")
		}
	}
	return strings.Join(lines, "\n")
}

// rowPriceQty resolves a row's DISPLAYED price/quantity for the merchant
// grid's on-cell price footer / qty badge (2026-07-29): the effective
// (staged-but-unapplied draft, or already-committed PendingEdits) value
// when one exists, else the row's own current value -- the same source
// effectiveRowEdit already supplies the icon swap from, so a pending
// price/qty edit shows up here too, not just in the row-edit bar.
func (s *State) rowPriceQty(row *catalog.Row) (price *int64, qty int64) {
	price, qty = row.Price, row.Quantity
	e := s.effectiveRowEdit(row.RowID)
	if e == nil {
		return price, qty
	}
	if fc, ok := e.FieldChanges["value"]; ok {
		v := fc.To
		price = &v
	}
	if fc, ok := e.FieldChanges["sellQuantity"]; ok {
		qty = fc.To
	}
	return price, qty
}

// qtyBadgeText renders the merchant grid's corner stock-count badge: blank
// for -1 (unlimited -- no natural stack-count analog), else the plain
// number, matching the in-game shop list showing a count for every finite-
// stock item.
func qtyBadgeText(qty int64) string {
	if qty < 0 {
		return ""
	}
	return strconv.FormatInt(qty, 10)
}

// footerCurrencyIconSize is the price footer's currency-icon size (2026-07-29,
// bumped up from reusing the row-edit bar's small fieldIconSize glyph --
// user, comparing against the game's own item card: "the icon png in the
// foot is too small... look again at how the game set it"). fieldIconSize
// itself stays untouched, still used by the row-edit bar's own Price/
// Quantity field labels.
const footerCurrencyIconSize = unit.Dp(18)

// rowPriceFooter builds the merchant grid cell's price line: a small
// currency icon (resolved from the row's own cost type, see
// field_meta.go's currencyIconPath) plus the price number, RIGHT-aligned
// and vertically centered in the strip below the icon -- 2026-07-29, user:
// "in the game this value is all shifted to the right and i think we must
// center it vertically too" (an earlier version left-hugged the strip's own
// edge). Both the icon and text scale with the cell's own size
// (components.BadgeScale) so the Shop Editor's cell-size slider actually makes
// this detail bigger, not just the item artwork. Every merchant-grid cell
// always gets a footer, even a priceless one ("-" fallback), so every cell
// in the grid shares the same total height -- a footer some cells have and
// others don't would misalign row bottoms across the grid.
func (s *State) rowPriceFooter(th *material.Theme, row *catalog.Row) layout.Widget {
	price, _ := s.rowPriceQty(row)
	scale := components.BadgeScale(s.merchantCellSize())
	content := func(gtx layout.Context) layout.Dimensions {
		if price == nil {
			lbl := material.Body2(th, "-")
			lbl.TextSize = th.TextSize * 12 / 16 * unit.Sp(scale)
			lbl.Color = colorMuted
			return lbl.Layout(gtx)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fixedIcon(gtx, currencyIcon(currencyIconPath(row.CostType)), footerCurrencyIconSize*unit.Dp(scale))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4 * scale)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, strconv.FormatInt(*price, 10))
				lbl.TextSize = th.TextSize * 12 / 16 * unit.Sp(scale)
				return lbl.Layout(gtx)
			}),
		)
	}
	return func(gtx layout.Context) layout.Dimensions {
		// Right-align + vertically center: measure the content's natural
		// size first (loose constraints, matching how content() itself
		// measures a Footer), then paint it offset within the full cell
		// width, with equal top/bottom padding -- layout.Direction can't be
		// used directly here since the incoming Max.Y is an effectively
		// unbounded sentinel (see content()'s Footer measurement pass),
		// which Direction would otherwise report back as its own Dimensions.
		pad := gtx.Dp(unit.Dp(4))
		inner := gtx
		inner.Constraints = layout.Constraints{Max: image.Pt(gtx.Constraints.Max.X-2*pad, gtx.Constraints.Max.Y-2*pad)}
		macro := op.Record(gtx.Ops)
		dims := content(inner)
		call := macro.Stop()

		x := gtx.Constraints.Max.X - dims.Size.X - pad
		if x < pad {
			x = pad
		}
		off := op.Offset(image.Pt(x, pad)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()

		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, dims.Size.Y+2*pad)}
	}
}

// materialSummary renders a row's required materials as "2x A + 1x B"
// ("a material" when a mtrlId resolves to nothing usable).
func materialSummary(row *catalog.Row) string {
	var parts []string
	for _, m := range row.Materials {
		if m.UnresolvedMtrlID != 0 || m.ItemName == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%dx %s", m.Qty, m.ItemName))
	}
	if len(parts) == 0 {
		return "a material"
	}
	return strings.Join(parts, " + ")
}
