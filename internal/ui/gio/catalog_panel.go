package gio

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
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

// layoutCatalogPanel is the left panel: every item in items.json, filterable
// by search text + category/subcategory, in a 6-wide icon grid. Picking mode
// (M4) will enable/disable cells; for now the disabled path only fires when a
// picking session is active, which it never is yet.
func (s *State) layoutCatalogPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Cascade: a category change repopulates the subcategory combo (with a
	// leading "" = all) and clears the current subcategory.
	if s.CategoryCombo.Changed() {
		s.refreshSubcategories()
	}
	if v, ok := liveClampedInt(gtx, &s.levelEditor, 0, pickLevelMax); ok {
		s.PickLevel = v
	}
	if s.levelSlider.Update(gtx) {
		lvl := clampPickLevel(int64(math.Round(float64(s.levelSlider.Value) * pickLevelMax)))
		if lvl != s.PickLevel {
			s.PickLevel = lvl
			s.levelEditor.SetText(strconv.FormatInt(lvl, 10))
		}
	}
	if !s.levelSlider.Dragging() {
		s.levelSlider.Value = float32(s.PickLevel) / pickLevelMax
	}

	// A combo's "" option already means "all", which the catalog treats as
	// "any" — no translation needed.
	items := s.filteredItems()
	picking := s.Picking()

	if picking && s.cancelPickBtn.Clicked(gtx) {
		s.CancelPicking()
	}

	// Keep the Catalog heading stable while replacing items. The actionable
	// picker explanation belongs in the shared status area at the bottom of
	// the window, rather than overwriting this panel's identity.
	titleRow := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.H6(th, "Catalog").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !picking {
					return layout.Dimensions{}
				}
				return barButton(th, &s.cancelPickBtn, "Cancel replacement").Layout(gtx)
			}),
		)
	})

	children := []layout.FlexChild{titleRow}
	children = append(children,
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			filterChildren := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(180))
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					ed := material.Editor(th, &s.Search, "Search items...")
					return boxed(gtx, th, ed.Layout)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.CategoryCombo.Layout(gtx, th, unit.Dp(160))
				}),
			}
			// Hidden entirely (not just showing "All Subcategories" as the
			// only option) when the current category has no real
			// sub-categories to filter by -- see refreshSubcategories.
			if s.subCatFilterHas {
				filterChildren = append(filterChildren,
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.SubCatCombo.Layout(gtx, th, unit.Dp(160))
					}),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, filterChildren...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutItemCountRow(gtx, th, len(items))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			dims := s.layoutItemGrid(gtx, th, items, picking)
			// The grid region (cells, gaps, scrollbar) is the only place a
			// press does NOT clear the catalog selection.
			s.pressListener(gtx, &s.catalogAreaTag, dims.Size, &s.catalogAreaHit)
			return dims
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pickLevelMax is the slider's fixed range (the global max across every
// weapon-table item, 25 for standard/Smithing-Stone weapons). A lower-cap
// item (somber, max +10) just clamps down silently at stage time
// (stageItemSwapCore) -- the shared control doesn't need to track whichever
// item might get picked next.
const pickLevelMax = 25

func clampPickLevel(v int64) int64 {
	if v < 0 {
		return 0
	}
	if v > pickLevelMax {
		return pickLevelMax
	}
	return v
}

// pickLevelFor is the "+N" a catalog item would be picked at right now: the
// shared PickLevel clamped to the item's own real max (0 for a non-weapon or a
// weapon that can't be reinforced). Mirrors the grid tooltip's own logic, so
// the item-info popup shows stats at exactly the level a pick would apply.
func (s *State) pickLevelFor(itemID int64) int {
	maxLvl, ok := s.Catalog.MaxUpgradeLevel(itemID)
	if !ok || maxLvl <= 0 {
		return 0
	}
	lvl := clampPickLevel(s.PickLevel)
	if lvl > int64(maxLvl) {
		lvl = int64(maxLvl)
	}
	return int(lvl)
}

// weaponLevelCategoryVisible reports whether the given CategoryCombo value
// can contain weapon-table items -- "" (All Categories) or one of the three
// weapon-table categories. Used to hide the Weapon Level control entirely
// when it would be useless (e.g. filtered to Talismans).
func weaponLevelCategoryVisible(cat string) bool {
	switch cat {
	case "", "melee_armaments", "ranged_and_catalysts", "shields":
		return true
	default:
		return false
	}
}

// layoutItemCountRow is the "N items" line below the filter bar. When the
// current category filter can show weapon-table items, it also carries the
// shared Weapon Level control (slider + exact-value box), right-aligned --
// this is a prospective setting applied to whatever gets picked/dragged
// next (see PickLevel), not a filter, so it doesn't belong in the filter
// row itself.
func (s *State) layoutItemCountRow(gtx layout.Context, th *material.Theme, count int) layout.Dimensions {
	return fixedMinHeight(gtx, filterCountRowHeight, func(gtx layout.Context) layout.Dimensions {
		return s.layoutItemCountRowContent(gtx, th, count)
	})
}

func (s *State) layoutItemCountRowContent(gtx layout.Context, th *material.Theme, count int) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body2(th, fmt.Sprintf("%d items", count)).Layout(gtx)
		}),
	}
	if weaponLevelCategoryVisible(s.CategoryCombo.Value()) {
		children = append(children,
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Body2(th, "Weapon Level").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(140))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				sl := material.Slider(th, &s.levelSlider)
				sl.Color = colorAccent
				return sl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(40))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				ed := material.Editor(th, &s.levelEditor, "")
				return boxed(gtx, th, ed.Layout)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// filteredItems returns the catalog filtered by the current controls,
// re-running the 2784-item scan only when a filter input actually changed
// (ListItems used to run every frame). items.json is fixed for the process
// lifetime, so the cache needs no other invalidation.
func (s *State) filteredItems() []*catalog.Item {
	key := itemFilterKey{
		category: s.CategoryCombo.Value(),
		subCat:   s.SubCatCombo.Value(),
		search:   s.Search.Text(),
		risky:    s.Settings.ShowRiskyItems,
	}
	if s.itemsCache == nil || key != s.itemsCacheKey {
		items := s.Catalog.ListItems(key.category, key.subCat, key.search, nil)
		if !key.risky {
			// Default: no risky items (cut content / online-ban risk) —
			// the user can't stage what they can't see. Opt in via
			// Settings.ShowRiskyItems.
			safe := items[:0:0]
			for _, it := range items {
				if !it.Risky {
					safe = append(safe, it)
				}
			}
			items = safe
		}
		if key.category == "" {
			// Catalog.ListItems already orders items correctly WITHIN each
			// category (sub-category grouping + real sortId, see
			// internal/catalog); for "All Categories" this stable pass groups by
			// categoryOrder's own rank on top, preserving that per-category
			// order rather than interleaving categories arbitrarily.
			sort.SliceStable(items, func(i, j int) bool {
				return categoryRank(items[i].Category) < categoryRank(items[j].Category)
			})
		}
		s.itemsCache = items
		s.itemsCacheKey = key
		// A new filter result is a new list — jump back to its top instead of
		// keeping a scroll offset into the old one.
		s.CatalogList.Position = layout.Position{}
	}
	return s.itemsCache
}

func (s *State) refreshSubcategories() {
	var subs []string
	if cat := s.CategoryCombo.Value(); cat != "" {
		subs = s.Catalog.ListSubcategories(cat)
	}
	options := append([]string{""}, subs...)
	labels := append([]string{"All Subcategories"}, subs...)
	s.SubCatCombo.SetOptionsWithLabels(options, labels)
	s.SubCatCombo.SetValue("")
	s.subCatFilterHas = len(subs) > 0
}

// categoryOrder is the Shop-Editor-relevant display order for item
// categories, per user request (2026-08-01). Spirit Ashes ("ashes") sits
// right after Tools: every goods category shares EquipParamGoods and the
// game orders them by sortId, whose ranges separate the categories -- tools
// ~100-11k, spirit ashes ~50k, crafting ~100k, ... -- so the game's own
// goods menu is tools then spirit ashes (verified 2026-08-03, user-flagged).
// "gestures" is deliberately absent -- gestures stay hidden entirely (never
// real merchant stock, see docs/ITEM_IDS.md) and fall through to
// orderedCategoryOptions' "unlisted sort after the known ones" fallback.
var categoryOrder = []string{
	"tools", "ashes", "crafting_materials", "bolstering_materials", "key_items",
	"sorceries", "incantations", "ashes_of_war",
	"melee_armaments", "ranged_and_catalysts", "arrows_and_bolts", "shields",
	"head", "chest", "arms", "legs",
	"talismans", "info",
}

// categoryRankIndex maps a raw category id to its position in categoryOrder,
// built once. Shared by orderedCategoryOptions (filter dropdown order) and
// filteredItems (All Categories item order) so both read the exact same
// ranking -- see categoryRank.
var categoryRankIndex = func() map[string]int {
	m := make(map[string]int, len(categoryOrder))
	for i, c := range categoryOrder {
		m[c] = i
	}
	return m
}()

// unknownCategoryRank is categoryRank's fallback for any category id not in
// categoryOrder (e.g. "gestures", or a category added by a future items.json
// update) -- sorts after every known category, never disappears.
const unknownCategoryRank = 1 << 30

func categoryRank(cat string) int {
	if r, ok := categoryRankIndex[cat]; ok {
		return r
	}
	return unknownCategoryRank
}

// categoryLabels maps a raw items.json category id (snake_case) to the
// friendly name shown in the Catalog's category filter.
var categoryLabels = map[string]string{
	"melee_armaments":      "Melee Weapons",
	"ranged_and_catalysts": "Ranged Weapons & Catalysts",
	"shields":              "Shields",
	"head":                 "Head Armor",
	"chest":                "Chest Armor",
	"arms":                 "Arm Armor",
	"legs":                 "Leg Armor",
	"talismans":            "Talismans",
	"ashes_of_war":         "Ashes of War",
	"sorceries":            "Sorceries",
	"incantations":         "Incantations",
	"arrows_and_bolts":     "Arrows & Bolts",
	"tools":                "Tools & Consumables",
	"crafting_materials":   "Crafting Materials",
	"bolstering_materials": "Bolstering Materials",
	"ashes":                "Spirit Ashes",
	"gestures":             "Gestures",
	"key_items":            "Key Items",
	"info":                 "Info Items",
}

// orderedCategoryOptions returns the CategoryCombo's options ("" first, then
// cats in categoryOrder -- any category not in that table, e.g. after a
// future items.json update, sorts alphabetically after the known ones so it
// can't silently disappear) and their matching display labels ("All
// Categories", then categoryLabels[cat] or the raw id as a fallback).
func orderedCategoryOptions(cats []string) (options, labels []string) {
	rank := make(map[string]int, len(categoryOrder))
	for i, c := range categoryOrder {
		rank[c] = i
	}
	sorted := append([]string(nil), cats...)
	sort.Slice(sorted, func(i, j int) bool {
		ri, iKnown := rank[sorted[i]]
		rj, jKnown := rank[sorted[j]]
		switch {
		case iKnown && jKnown:
			return ri < rj
		case iKnown != jKnown:
			return iKnown
		default:
			return sorted[i] < sorted[j]
		}
	})

	options = append([]string{""}, sorted...)
	labels = make([]string, len(options))
	labels[0] = "All Categories"
	for i, c := range sorted {
		if l, ok := categoryLabels[c]; ok {
			labels[i+1] = l
		} else {
			labels[i+1] = c
		}
	}
	return options, labels
}

func (s *State) layoutItemGrid(gtx layout.Context, th *material.Theme, items []*catalog.Item, picking bool) layout.Dimensions {
	return layoutGrid(gtx, th, &s.CatalogList, len(items), s.catalogCellSize(), func(gtx layout.Context, i int) layout.Dimensions {
		it := items[i]
		sellable := it.EquipType != nil
		disabled := picking && !sellable
		// Normal mode: just the item name -- the disabled dim veil already
		// shows a cell can't be picked, no need to explain the technical why.
		// A weapon-table item's tooltip shows the "+N" it would be picked at
		// right now (PickLevel clamped to this item's own max) -- recomputed
		// fresh every frame, so it tracks the slider/box live while hovered,
		// exactly as if the cursor re-entered after every change.
		tooltip := it.Name
		if maxLvl, ok := s.Catalog.MaxUpgradeLevel(it.ID); ok && maxLvl > 0 {
			if lvl := clampPickLevel(s.PickLevel); lvl > 0 {
				if lvl > int64(maxLvl) {
					lvl = int64(maxLvl)
				}
				tooltip = fmt.Sprintf("%s +%d", it.Name, lvl)
			}
		}
		if disabled {
			tooltip = it.Name + " (not sellable, no equip-slot mapping)"
		}
		cs := s.itemCell(it.ID)
		s.handleCatalogClicks(gtx, cs, items, i, picking, sellable)

		// Selected > risky (cut content / online-ban risk -- only reachable
		// with Settings.ShowRiskyItems on at all, but still worth flagging
		// red there).
		var border *color.NRGBA
		switch {
		case s.isSelected(it.ID):
			border = &borderSelected
		case it.Risky:
			border = &borderWarn
		}
		riskyTooltip := tooltip
		if it.Risky {
			riskyTooltip += " [!] cut content / online-ban risk"
		}
		cell := components.IconCell{
			Img:      s.Icons.Get(it.IconPath),
			Size:     s.catalogCellSize(),
			Disabled: disabled,
			Tooltip:  riskyTooltip,
			Border:   border,
		}
		render := func(gtx layout.Context) layout.Dimensions {
			return cell.Layout(gtx, th, &cs.click)
		}
		// Every sellable cell is always a drag source (2026-07-28 v2 — was
		// gated on isSelected, requiring a separate prior click-release
		// before a press-and-move could drag at all; that extra step was
		// the "fast double click does nothing" complaint, since a drag
		// started before the first click's release had registered had
		// nowhere to attach to). A plain click (press+release, no
		// movement) still only reaches the underlying Clickable and keeps
		// its existing toggle/multi-select semantics — Draggable requires
		// real movement past its slop threshold before Dragging() latches,
		// so static clicks are unaffected.
		if sellable {
			cs.drag.Type = itemsMIME
			if mime, ok := cs.drag.Update(gtx); ok {
				payload := s.dragPayload(it.ID)
				cs.drag.Offer(gtx, mime, io.NopCloser(strings.NewReader(payload)))
			}
			var ghost layout.Widget
			if cs.drag.Dragging() {
				if s.dragSrc != cs {
					s.dragSrc = cs
					s.dragCount = s.dragIDCount(it.ID)
				}
				// Ghost only once the drag actually moved (transferInit) —
				// a mere press shows nothing yet. Dragging an unselected
				// item selects just that item first, both so its border
				// highlights immediately (was: no highlight until a
				// separate prior click) and so the group-drag count stays
				// correct.
				if s.transferInit {
					if !s.isSelected(it.ID) {
						s.selectPlain(it.ID)
						s.dragCount = 1
					}
					img, count := s.Icons.Get(it.IconPath), s.dragCount
					ghost = func(gtx layout.Context) layout.Dimensions {
						return dragGhost(gtx, th, img, count)
					}
				}
			}
			// PassOp: Draggable registers its drag area ON TOP of the cell's
			// click area, and input areas are opaque by default — without
			// pass-through the Clickable underneath never sees a press and
			// selection clicks go dead (drags still worked, which is how this
			// was found).
			pass := pointer.PassOp{}.Push(gtx.Ops)
			dims := cs.drag.Layout(gtx, render, ghost)
			pass.Pop()
			if rightClickTarget(gtx, cs, dims.Size) {
				s.openItemInfo(it.ID, s.pickLevelFor(it.ID))
			}
			return dims
		}
		dims := render(gtx)
		if rightClickTarget(gtx, cs, dims.Size) {
			s.openItemInfo(it.ID, s.pickLevelFor(it.ID))
		}
		return dims
	})
}

// handleCatalogClicks drains a catalog cell's click events. While picking, any
// click on a sellable item stages the swap for the picking row (selection
// untouched). Otherwise a click updates the multi-selection per its modifiers.
// Unsellable cells are never selectable.
func (s *State) handleCatalogClicks(gtx layout.Context, cs *cellState, items []*catalog.Item, i int, picking, sellable bool) {
	for {
		ev, ok := cs.click.Update(gtx)
		if !ok {
			break
		}
		if picking {
			if sellable {
				if row := s.consumeNextPickingRow(); row != nil {
					s.stageItemSwapCore(s.draftEdits, row, items[i])
				}
			}
			continue
		}
		if !sellable {
			continue
		}
		id := items[i].ID
		switch {
		case ev.Modifiers.Contain(key.ModShortcut):
			s.toggleSelect(id)
		case ev.Modifiers.Contain(key.ModShift):
			s.selectRange(items, i)
		default:
			s.selectPlain(id)
		}
	}
}

// dragGhost is the little "something in hand" drawn under the cursor during a
// drag: the dragged item's icon in a bordered chip, with an "xN" badge when
// the payload carries more than one item. Draggable defers it over everything
// and moves it with the pointer.
func dragGhost(gtx layout.Context, th *material.Theme, img paint.ImageOp, count int) layout.Dimensions {
	side := gtx.Dp(unit.Dp(44))
	sz := image.Pt(side, side)
	gtx.Constraints = layout.Exact(sz)

	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: sz}.Op())
	widget.Image{Src: img, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
	widget.Border{Color: borderSelected, Width: unit.Dp(1), CornerRadius: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: sz} })

	if count > 1 {
		// Count badge, bottom-right of the chip.
		rec := op.Record(gtx.Ops)
		lbl := material.Body2(th, fmt.Sprintf("x%d", count))
		lbl.Color = th.Palette.ContrastFg
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, lbl.Layout)
		call := macro.Stop()
		paint.FillShape(gtx.Ops, colorAccent, clip.Rect{Max: dims.Size}.Op())
		call.Add(gtx.Ops)
		badge := rec.Stop()

		off := op.Offset(image.Pt(side*2/3, side*2/3))
		off.Add(gtx.Ops)
		badge.Add(gtx.Ops)
	}
	return layout.Dimensions{Size: sz}
}
