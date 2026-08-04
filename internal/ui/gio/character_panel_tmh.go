package gio

// Twin Maiden Husks-specific rendering and helpers for the Characters view.
// She gets a dedicated 3-section grid (layoutTMHFlagsGrid) instead of the
// plain flat flag list every other merchant uses -- see docs/CHAR_UNLOCK.md
// and character.BellBearing. Depends only on shared helpers living in
// character_panel.go (groupFlagRows, flagGroupLabel, flagGroupCheckbox,
// layoutFlagRow) plus shared State fields.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
)

// bellBearingSortKey splits a bell bearing (or item-name-fallback) label
// into a family base and a trailing bracketed number (0 if none), e.g.
// "Nomadic Merchant's Bell Bearing [7]" -> ("Nomadic Merchant's Bell
// Bearing", 7). Used to group same-family entries together, ordered by
// number, instead of raw table/flag-ID order.
func bellBearingSortKey(name string) (base string, num int) {
	i := strings.LastIndex(name, " [")
	if i < 0 || !strings.HasSuffix(name, "]") {
		return name, 0
	}
	n, err := strconv.Atoi(name[i+2 : len(name)-1])
	if err != nil {
		return name, 0
	}
	return name[:i], n
}

// bellBearingGroupRank follows Twin Maiden Husks' actual Bell Bearing Shop
// routing, rather than treating each wandering archetype as a separate shop:
// Shop 1 specialists (including Iji), Shop 2's Gostoc/Pidia/Patches/Blackguard, Shop 3 Nomadic,
// Shop 4 Kalé plus every other wandering family, and Shop 5 DLC.
func bellBearingGroupRank(b character.BellBearing) int {
	switch {
	case strings.HasPrefix(b.Name, "Nomadic Merchant's"):
		return 3
	case b.Name == "Kalé's Bell Bearing", strings.HasPrefix(b.Name, "Isolated Merchant's"),
		strings.HasPrefix(b.Name, "Hermit Merchant's"), strings.HasPrefix(b.Name, "Imprisoned Merchant's"),
		strings.HasPrefix(b.Name, "Abandoned Merchant's"):
		return 4
	case b.Category == "dlc":
		return 5
	case b.Name == "Pidia's Bell Bearing", b.Name == "Patches' Bell Bearing", b.Name == "Gostoc's Bell Bearing",
		b.Name == "Blackguard's Bell Bearing":
		return 2
	default: // all other base-game named sellers are Shop 1
		return 1
	}
}

func bellBearingShopSortKey(b character.BellBearing) string {
	base, num := bellBearingSortKey(b.Name)
	if rank := bellBearingGroupRank(b); rank == 1 || rank == 2 {
		// The game's submenu order is not the save-flag order. Reuse the
		// shared merchant sort key so the Characters view and merchant filter
		// cannot drift apart. Miriel/Gowry use their first menu position: the
		// two spell-type entries each share their one underlying flag.
		if b.Merchant != "" {
			_, key := catalog.MerchantSortKey(b.Merchant)
			return key
		}
		return "99:" + base
	}
	if bellBearingGroupRank(b) != 4 {
		return fmt.Sprintf("%02d:%s", num, base)
	}
	switch {
	case b.Name == "Kalé's Bell Bearing":
		return "00:" + b.Name
	case strings.HasPrefix(b.Name, "Isolated Merchant's"):
		return fmt.Sprintf("01:%02d:%s", num, base)
	case strings.HasPrefix(b.Name, "Hermit Merchant's"):
		return fmt.Sprintf("02:%02d:%s", num, base)
	case strings.HasPrefix(b.Name, "Abandoned Merchant's"):
		return "03:" + b.Name
	case strings.HasPrefix(b.Name, "Imprisoned Merchant's"):
		return "04:" + b.Name
	default:
		return "99:" + b.Name
	}
}

// sortBellBearingsByFamily groups the NPC Bell Bearings by merchant family,
// then orders numbered entries within each family. A bearing's Merchant field
// is deliberately not part of this comparison: only some entries have a
// confident mapping, so using it would split and scramble one numbered family.
func sortBellBearingsByFamily(bb []character.BellBearing) {
	sort.Slice(bb, func(i, j int) bool {
		ri, rj := bellBearingGroupRank(bb[i]), bellBearingGroupRank(bb[j])
		if ri != rj {
			return ri < rj
		}
		return bellBearingShopSortKey(bb[i]) < bellBearingShopSortKey(bb[j])
	})
}

// splitTMHBearingColumns preserves the vertical families that make the
// bearing list scannable while keeping the two visual columns balanced:
// left is the named Shop 1/2 entries, Kalé, then Shop 5; right is the full
// Nomadic [1-10] run followed by Shop 4's remaining families. Kalé is the
// one Shop 4 entry deliberately kept with named merchants, making both
// columns exactly 18 entries in the current game data.
func splitTMHBearingColumns(bearings []character.BellBearing) (left, right []character.BellBearing) {
	for _, b := range bearings {
		rank := bellBearingGroupRank(b)
		if rank == 1 || rank == 2 || rank == 5 || b.Name == "Kalé's Bell Bearing" {
			left = append(left, b)
		} else {
			right = append(right, b)
		}
	}
	return left, right
}

// tmhOtherItem is one "TMH Bell Bearings"/"Other Items" entry: a gated-
// row group that either matches a BellBearing (known -- "TMH Bell
// Bearings", labeled with its name via layoutFlagRowNamed) or doesn't
// ("Other Items", item-name label via the plain layoutFlagRow).
type tmhOtherItem struct {
	group flagGroup
	bb    character.BellBearing
	known bool
}

// tmhFlagSections classifies Twin Maiden Husks' gated-row groups and
// character.BellBearingsForUI() into the 3 sections
// layoutTMHFlagsGrid renders, each pre-sorted by family+number
// (bellBearingSortKey). A pure function, factored out so
// TestLayoutTMHFlagsSectionCoversEveryEntry can assert coverage (every
// group/entry lands in exactly one section, none dropped) without a real
// layout pass.
//   - tmhBearings ("TMH Bell Bearings"): her own gated-row groups whose
//     flag matches a BellBearing entry (always Covered==true, since
//     Covered==false bell bearings never gate her own rows) --
//     bell-bearing-name labeled.
//   - otherItems ("Other Items"): her own gated-row groups with no
//     BellBearing match at all -- item-name labeled.
//   - npcBearings ("NPC Bell Bearings"): character.BellBearingsForUI()
//     in full -- every bell bearing that opens a DIFFERENT merchant's own
//     shop through her, individually-named NPCs (Patches, Corhyn, Moore,
//     ...) and the numbered wandering-merchant families (Nomadic/
//     Isolated/Hermit/Imprisoned/Abandoned) alike, not split by
//     BellBearing.Category -- these never match a row group.
func tmhFlagSections(rows []*catalog.Row) (tmhBearings, otherItems []tmhOtherItem, npcBearings []character.BellBearing) {
	byFlag := make(map[int64]character.BellBearing, len(character.BellBearings))
	for _, b := range character.BellBearings {
		byFlag[int64(b.FlagID)] = b
	}
	for _, g := range groupFlagRows(rows) {
		if bb, ok := byFlag[g.FlagID]; ok {
			tmhBearings = append(tmhBearings, tmhOtherItem{group: g, bb: bb, known: true})
		} else {
			otherItems = append(otherItems, tmhOtherItem{group: g})
		}
	}
	npcBearings = append(npcBearings, character.BellBearingsForUI()...)
	sortBellBearingsByFamily(npcBearings)

	return tmhBearings, otherItems, npcBearings
}

// addTMHSection appends a section header (skipped entirely if n == 0)
// plus its n rows, one per grid-row, to blocks -- shared by
// layoutTMHFlagsGrid's sections so their header styling stays identical.
func addTMHSection(th *material.Theme, blocks *[]layout.Widget, title string, n int, row func(i int) layout.Widget) {
	if n == 0 {
		return
	}
	if len(*blocks) > 0 {
		*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(14)}.Layout)
	}
	*blocks = append(*blocks, tmhSectionHeader(th, title))
	*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(4)}.Layout)
	for i := 0; i < n; i++ {
		*blocks = append(*blocks, row(i))
	}
}

// addTMHSectionPaired is addTMHSection's 2-per-row counterpart: appends
// a section header (skipped entirely if n == 0) plus its n rows laid out
// two per grid-row (row-major, left then right -- the last row's right
// cell is blank when n is odd).
func addTMHSectionPaired(th *material.Theme, blocks *[]layout.Widget, title string, n int, cell func(i int) layout.Widget) {
	if n == 0 {
		return
	}
	if len(*blocks) > 0 {
		*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(14)}.Layout)
	}
	*blocks = append(*blocks, tmhSectionHeader(th, title))
	*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(4)}.Layout)
	for i := 0; i < n; i += 2 {
		i := i
		*blocks = append(*blocks, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, cell(i)),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if i+1 < n {
						return cell(i + 1)(gtx)
					}
					return layout.Dimensions{}
				}),
			)
		})
	}
}

// addTMHSectionColumns lays out two independent vertical runs beneath one
// header. Unlike addTMHSectionPaired, left and right are not consecutive
// entries: a merchant family can therefore stay entirely in one column.
func addTMHSectionColumns(th *material.Theme, blocks *[]layout.Widget, title string, leftN, rightN int, leftCell, rightCell func(i int) layout.Widget) {
	if leftN == 0 && rightN == 0 {
		return
	}
	if len(*blocks) > 0 {
		*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(14)}.Layout)
	}
	*blocks = append(*blocks, tmhSectionHeader(th, title))
	*blocks = append(*blocks, layout.Spacer{Height: unit.Dp(4)}.Layout)
	n := leftN
	if rightN > n {
		n = rightN
	}
	for i := 0; i < n; i++ {
		i := i
		*blocks = append(*blocks, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if i < leftN {
						return leftCell(i)(gtx)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if i < rightN {
						return rightCell(i)(gtx)
					}
					return layout.Dimensions{}
				}),
			)
		})
	}
}

// tmhSectionHeader keeps the existing section names, but presents each in a
// small label sitting on a fine rule. This makes the flag families
// distinct without introducing new group names or large panel chrome.
func tmhSectionHeader(th *material.Theme, title string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, horizontalDivider),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tmhSectionTitleChip(gtx, th, title)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, horizontalDivider),
		)
	}
}

func tmhSectionTitleChip(gtx layout.Context, th *material.Theme, title string) layout.Dimensions {
	label := material.Body1(th, title)
	label.Color = colorFg
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, label.Layout)
	call := macro.Stop()
	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	return dims
}

// layoutTMHFlagsGrid is Twin Maiden Husks' flags-column body: ONE
// scrollable list (a single material.List, single scrollbar -- the same
// "headers + rows in one scroll region" pattern pending_edits.go's
// layoutPendingScrollList uses). Gio gotcha: it MUST be one Flexed(1)/
// material.List region with no trailing Rigid sibling -- a trailing sibling
// clipped an earlier version (see docs/CHAR_UNLOCK.md). 3 sections stacked
// top to bottom:
//  1. "TMH Bell Bearings" -- one item per row (addTMHSection): its
//     "unlocks: ..." subtext line can run long, so it keeps the full row
//     width.
//  2. "NPC Bell Bearings" -- two balanced independent columns:
//     named Shop 1/2 + Kalé + Shop 5 at left, full Nomadic then the remaining
//     Shop 4 families at right. Numbered families never interleave.
//  3. "Other Items" -- two items per row (addTMHSectionPaired).
//
// A section is omitted entirely when empty.
func (s *State) layoutTMHFlagsGrid(gtx layout.Context, th *material.Theme) layout.Dimensions {
	tmhBearings, otherItems, npcBearings := tmhFlagSections(s.FlagRows)

	var blocks []layout.Widget
	addTMHSection(th, &blocks, "TMH Bell Bearings", len(tmhBearings), func(i int) layout.Widget {
		it := tmhBearings[i]
		return func(gtx layout.Context) layout.Dimensions { return s.layoutFlagRowNamed(gtx, th, it.group, it.bb) }
	})
	left, right := splitTMHBearingColumns(npcBearings)
	addTMHSectionColumns(th, &blocks, "NPC Bell Bearings", len(left), len(right),
		func(i int) layout.Widget {
			b := left[i]
			return func(gtx layout.Context) layout.Dimensions { return s.layoutBellBearingRow(gtx, th, b) }
		},
		func(i int) layout.Widget {
			b := right[i]
			return func(gtx layout.Context) layout.Dimensions { return s.layoutBellBearingRow(gtx, th, b) }
		},
	)
	addTMHSectionPaired(th, &blocks, "Other Items", len(otherItems), func(i int) layout.Widget {
		it := otherItems[i]
		return func(gtx layout.Context) layout.Dimensions { return s.layoutFlagRow(gtx, th, it.group) }
	})

	s.tmhColList.Axis = layout.Vertical
	return material.List(th, &s.tmhColList).Layout(gtx, len(blocks), func(gtx layout.Context, i int) layout.Dimensions {
		return blocks[i](gtx)
	})
}

// layoutBellBearingRow renders one NPC Bell Bearing checkbox. The bearing's
// own name is enough once generic shops carry their real archetype names in
// the Shop Editor filter; repeating the target shop made this dense grid hard
// to scan.
func (s *State) layoutBellBearingRow(gtx layout.Context, th *material.Theme, b character.BellBearing) layout.Dimensions {
	pending := s.PendingBellBearingEdits[s.SelectedChar]
	val, staged := s.bellBearingState[b.FlagID], false
	if target, ok := pending[b.FlagID]; ok {
		val, staged = target, true
	}
	chk := s.bellBearingCheck(b.FlagID)
	chk.Value = val

	box := material.CheckBox(th, chk, b.Name)
	if staged {
		box.Color = colorAmber
	}
	return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, box.Layout)
}

// layoutFlagRowNamed is layoutFlagRow's counterpart for a group whose
// flag matches a character.BellBearing entry ("TMH Bell Bearings" in
// layoutTMHOwnStockColumn): same checkbox/staging/amber-if-staged logic,
// but the primary label is the bell bearing's name instead of the item
// names it unlocks -- those item names move to a muted subtext line
// instead (still built via the same flagGroupLabel the plain item-name
// label used to use).
func (s *State) layoutFlagRowNamed(gtx layout.Context, th *material.Theme, g flagGroup, bb character.BellBearing) layout.Dimensions {
	chk, staged := s.flagGroupCheckbox(g)

	box := material.CheckBox(th, chk, bb.Name)
	if staged {
		box.Color = colorAmber
	}
	subtext := material.Body2(th, "unlocks: "+flagGroupLabel(g))
	subtext.Color = colorMuted

	return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(box.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(28)}.Layout(gtx, subtext.Layout)
			}),
		)
	})
}
