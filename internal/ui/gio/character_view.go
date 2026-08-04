package gio

import (
	"fmt"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
)

// --- layout ---

// layoutCharactersPanel is the whole view shown instead of the two editor
// panels while State.view == viewCharacters: an Open-file bar and a
// 3-column character/merchant/flags drill-down. Saving staged flag edits
// happens through the shared footer -- see startCombinedSave in state.go.
func (s *State) layoutCharactersPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.ensureCharList()

	// Drain clicks before recomputing the caches they invalidate, so a
	// click's effect is reflected in this same frame's render instead of
	// lagging one frame behind.
	if s.loadTypedBtn.Clicked(gtx) {
		if p := strings.TrimSpace(s.pathEditor.Text()); p != "" {
			s.StartLoadSave(p)
		}
	}
	if s.openBtn.Clicked(gtx) {
		s.openFileDialog()
	}
	for i := range s.CharList {
		idx := s.CharList[i].Index
		if s.charBtn(idx).Clicked(gtx) {
			s.selectCharacter(idx)
			if s.SelectedChar < 0 {
				s.setFooterNotice("Character deselected — choose one to inspect merchant unlocks")
			} else {
				s.setFooterNotice("Selected " + s.selectedCharName() + " — choose a merchant to inspect its unlocks")
			}
		}
	}
	s.ensureMerchantGated()
	gatedMerchants := s.sortedGatedMerchants()
	for _, name := range gatedMerchants {
		if s.merchantUnlockBtn(name).Clicked(gtx) {
			s.selectFlagMerchant(name)
			if s.UnlockMerchant == "" {
				s.setFooterNotice("Merchant closed — choose another merchant to inspect its unlocks")
			} else {
				s.setFooterNotice("Opened " + name + " unlocks — check entries to stage changes")
			}
		}
	}
	for _, g := range groupFlagRows(s.FlagRows) {
		chk := s.flagCheck(g.FlagID)
		if chk.Update(gtx) {
			for _, r := range g.Rows {
				s.stageFlag(r.RowID, chk.Value)
			}
			if s.combinedPendingCount() == 0 {
				s.setFooterNotice("Unlock choice restored — no changes remain staged")
			} else {
				s.setFooterNotice("Unlock choice updated — Save File writes staged changes to a new copy")
			}
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range character.BellBearingsForUI() {
			chk := s.bellBearingCheck(b.FlagID)
			if chk.Update(gtx) {
				s.stageBellBearing(b.FlagID, chk.Value)
				if s.combinedPendingCount() == 0 {
					s.setFooterNotice("Bell Bearing choice restored — no changes remain staged")
				} else {
					s.setFooterNotice("Bell Bearing choice updated — Save File writes staged changes to a new copy")
				}
			}
		}
	}
	lockedHere := s.flagsColumnLockedCount()
	lockedEverywhere := s.allMerchantsLockedCount()
	if clicked := s.unlockAllBtn.Clicked(gtx); clicked && (s.merchantUnlockUndo != nil || lockedHere > 0) {
		undoing := s.merchantUnlockUndo != nil
		s.toggleMerchantUnlocks()
		if undoing {
			s.setFooterNotice("Removed this merchant's staged unlocks")
		} else {
			s.setFooterNotice(fmt.Sprintf("Staged %d unlocks for %s — save a new copy to write them", lockedHere, s.UnlockMerchant))
		}
	}
	if clicked := s.unlockAllMerchantsBtn.Clicked(gtx); clicked && (s.allMerchantsUndo != nil || lockedEverywhere > 0) {
		undoing := s.allMerchantsUndo != nil
		s.toggleEveryMerchantUnlocks()
		if undoing {
			s.setFooterNotice("Removed all staged merchant unlocks")
		} else {
			s.setFooterNotice(fmt.Sprintf("Staged %d merchant unlocks — save a new copy to write them", lockedEverywhere))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutOpenBar(gtx, th)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutCharactersColumns(gtx, th, gatedMerchants)
			})
		}),
	)
}

// effectiveFlagGroupUnlocked returns the committed state overlaid with a
// staged change. A group normally has every row staged together, but scanning
// the group keeps the bulk controls correct even if an old pending edit only
// touched one row in the shared flag group.
func (s *State) effectiveFlagGroupUnlocked(g flagGroup) bool {
	value := s.FlagState[g.Rows[0].RowID]
	for _, r := range g.Rows {
		if target, staged := s.PendingFlagEdits[s.SelectedChar][r.RowID]; staged {
			return target
		}
	}
	return value
}

func (s *State) effectiveBellBearingUnlocked(flagID uint32) bool {
	if target, staged := s.PendingBellBearingEdits[s.SelectedChar][flagID]; staged {
		return target
	}
	return s.bellBearingState[flagID]
}

// toggleMerchantUnlocks unlocks every remaining checkbox in the open
// merchant, or restores exactly the values changed by its previous click.
func (s *State) toggleMerchantUnlocks() {
	if undo := s.merchantUnlockUndo; undo != nil {
		s.restoreBulkUnlockUndo(undo)
		s.merchantUnlockUndo = nil
		s.invalidate()
		return
	}
	if s.SelectedChar < 0 || s.UnlockMerchant == "" {
		return
	}
	undo := &bulkUnlockUndo{charIndex: s.SelectedChar, flags: make(map[int64]pendingBoolSnapshot), bearings: make(map[uint32]pendingBoolSnapshot)}
	for _, g := range groupFlagRows(s.FlagRows) {
		if s.effectiveFlagGroupUnlocked(g) {
			continue
		}
		s.flagCheck(g.FlagID).Value = true
		for _, r := range g.Rows {
			undo.flags[r.RowID] = s.pendingFlagSnapshot(r.RowID)
			s.stageFlag(r.RowID, true)
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range character.BellBearingsForUI() {
			if s.effectiveBellBearingUnlocked(b.FlagID) {
				continue
			}
			s.bellBearingCheck(b.FlagID).Value = true
			undo.bearings[b.FlagID] = s.pendingBellBearingSnapshot(b.FlagID)
			s.stageBellBearing(b.FlagID, true)
		}
	}
	if len(undo.flags) != 0 || len(undo.bearings) != 0 {
		s.merchantUnlockUndo = undo
		s.invalidate()
	}
}

// toggleEveryMerchantUnlocks is the character-wide counterpart of
// toggleMerchantUnlocks. Enia's rows exist in charFlagState only for their
// read-only Shop Editor lock display, so readOnlyGateRows keeps them out of
// this write path. TMH's talk-script bearing flags are added separately
// because they have no ShopLineup row.
func (s *State) toggleEveryMerchantUnlocks() {
	if undo := s.allMerchantsUndo; undo != nil {
		s.restoreBulkUnlockUndo(undo)
		s.allMerchantsUndo = nil
		s.invalidate()
		return
	}
	if s.SelectedChar < 0 {
		return
	}
	s.ensureMerchantGated()
	undo := &bulkUnlockUndo{charIndex: s.SelectedChar, flags: make(map[int64]pendingBoolSnapshot), bearings: make(map[uint32]pendingBoolSnapshot)}
	for rowID, committed := range s.charFlagState {
		if s.readOnlyGateRows[rowID] {
			continue
		}
		_, staged := s.PendingFlagEdits[s.SelectedChar][rowID]
		if staged && s.PendingFlagEdits[s.SelectedChar][rowID] {
			continue
		}
		if !committed || staged {
			undo.flags[rowID] = s.pendingFlagSnapshot(rowID)
		}
		s.stageFlagAgainstCommitted(rowID, committed, true)
	}
	for _, b := range character.BellBearingsForUI() {
		_, staged := s.PendingBellBearingEdits[s.SelectedChar][b.FlagID]
		if staged && s.PendingBellBearingEdits[s.SelectedChar][b.FlagID] {
			continue
		}
		if !s.tmhBellCommitted[b.FlagID] || staged {
			undo.bearings[b.FlagID] = s.pendingBellBearingSnapshot(b.FlagID)
		}
		s.stageBellBearingAgainstCommitted(b.FlagID, s.tmhBellCommitted[b.FlagID], true)
	}
	if len(undo.flags) != 0 || len(undo.bearings) != 0 {
		s.allMerchantsUndo = undo
		s.invalidate()
	}
}

func (s *State) restoreBulkUnlockUndo(undo *bulkUnlockUndo) {
	if undo == nil || undo.charIndex != s.SelectedChar {
		return
	}
	for rowID, old := range undo.flags {
		if old.staged {
			if s.PendingFlagEdits[s.SelectedChar] == nil {
				s.PendingFlagEdits[s.SelectedChar] = make(map[int64]bool)
			}
			s.PendingFlagEdits[s.SelectedChar][rowID] = old.value
		} else if pending := s.PendingFlagEdits[s.SelectedChar]; pending != nil {
			delete(pending, rowID)
		}
	}
	for flagID, old := range undo.bearings {
		if old.staged {
			if s.PendingBellBearingEdits[s.SelectedChar] == nil {
				s.PendingBellBearingEdits[s.SelectedChar] = make(map[uint32]bool)
			}
			s.PendingBellBearingEdits[s.SelectedChar][flagID] = old.value
		} else if pending := s.PendingBellBearingEdits[s.SelectedChar]; pending != nil {
			delete(pending, flagID)
		}
	}
}

func (s *State) clearBulkUnlockUndo() {
	s.merchantUnlockUndo = nil
	s.allMerchantsUndo = nil
}

// layoutOpenBar is the er_pvp_mod-style top bar: a typed-path editor +
// Load (no-op if empty), a Browse... button (native dialog), and a status
// message to the right of Browse.
// pathEditorWidthNum/Den cap the typed-path Editor at 2/5 of the bar's own
// width -- left uncapped (as a Flexed child) it swallows all remaining
// space after the buttons.
const pathEditorWidthNum, pathEditorWidthDen = 2, 5

// filenameSlotWidth is the fixed width of the loaded-filename slot at
// the end of the Open bar (layoutFilenameSlot, save_switcher.go).
const filenameSlotWidth = unit.Dp(220)

// dividerBarHeight caps verticalDivider's height when used inside a
// single-line bar (a Rigid child of a Vertical Flex effectively has an
// unbounded Max.Y -- verticalDivider trusts Max.Y completely, so without
// this it would paint a divider far taller than the bar itself).
const dividerBarHeight = unit.Dp(28)

// layoutPathEditor renders the typed-path Editor capped to
// pathEditorWidthNum/Den of barWidth, and FORCED to fill that width (not
// just capped to it) -- Min.X = Max.X, since a Rigid Flex child is free to
// report any width up to Max, and an Editor with short/empty content
// otherwise shrinks to fit its placeholder text instead. A method rather
// than an inline closure so character_panel_test.go can assert its rendered
// width directly (a real width bug once slipped through as an inline closure).
func (s *State) layoutPathEditor(gtx layout.Context, th *material.Theme, barWidth int) layout.Dimensions {
	gtx.Constraints.Max.X = barWidth * pathEditorWidthNum / pathEditorWidthDen
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	ed := material.Editor(th, &s.pathEditor, "Save file path...")
	return boxed(gtx, th, ed.Layout)
}

func (s *State) layoutOpenBar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	barWidth := gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPathEditor(gtx, th, barWidth)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(th, &s.loadTypedBtn, "Load").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(th, &s.openBtn, "Browse...").Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutOpenMessage(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = gtx.Dp(dividerBarHeight)
			return verticalDivider(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(filenameSlotWidth)
			return s.layoutFilenameSlot(gtx, th)
		}),
	)
}

// sortedGatedMerchants orders using catalog.MerchantSortKey (TMH first,
// then named NPCs, then the 5 wandering-merchant shop families, then DLC)
// so this column matches the Shop Editor filter dropdown's ordering.
func (s *State) sortedGatedMerchants() []string {
	names := make([]string, 0, len(s.merchantGatedTotal))
	for name := range s.merchantGatedTotal {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		gi, ni := catalog.MerchantSortKey(names[i])
		gj, nj := catalog.MerchantSortKey(names[j])
		if gi != gj {
			return gi < gj
		}
		return ni < nj
	})
	return names
}

func (s *State) layoutCharactersColumns(gtx layout.Context, th *material.Theme, gatedMerchants []string) layout.Dimensions {
	if !s.Catalog.Loaded() {
		lbl := material.Body1(th, "Open a save file above to get started.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if len(s.CharList) == 0 {
		lbl := material.Body1(th, "No characters found in this save.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(charColWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutCharColumn(gtx, th)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return verticalDivider(gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(merchantColWidth)
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutMerchantColumn(gtx, th, gatedMerchants, s.displayMerchantUnlocked())
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return verticalDivider(gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutFlagsColumn(gtx, th)
		}),
	)
}

func (s *State) layoutCharColumn(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.charColList.Axis = layout.Vertical
	return material.List(th, &s.charColList).Layout(gtx, len(s.CharList), func(gtx layout.Context, i int) layout.Dimensions {
		return s.layoutCharRow(gtx, th, s.CharList[i])
	})
}

func (s *State) layoutCharRow(gtx layout.Context, th *material.Theme, ch character.Character) layout.Dimensions {
	label := fmt.Sprintf("%s\nlvl %d", ch.Name, ch.Level)
	if n := s.pendingFlagCount(ch.Index) + s.pendingBellBearingCount(ch.Index); n > 0 {
		label = fmt.Sprintf("%s\nlvl %d (%d pending)", ch.Name, ch.Level, n)
	}
	btn := formButton(th, s.charBtn(ch.Index), label)
	if s.SelectedChar == ch.Index {
		btn.Background = colorAccent
		btn.Color = th.Palette.ContrastFg
	} else {
		btn.Background = colorInputBg
		btn.Color = colorFg
	}
	return layout.Inset{Bottom: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
}

func (s *State) layoutMerchantColumn(gtx layout.Context, th *material.Theme, gatedMerchants []string, unlockedDisplay map[string]int) layout.Dimensions {
	if s.SelectedChar < 0 {
		lbl := material.Body2(th, "Pick a character.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if len(gatedMerchants) == 0 {
		lbl := material.Body2(th, "No merchant has gated stock for this character.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	s.merchantColList.Axis = layout.Vertical
	return material.List(th, &s.merchantColList).Layout(gtx, len(gatedMerchants), func(gtx layout.Context, i int) layout.Dimensions {
		return s.layoutMerchantRow(gtx, th, gatedMerchants[i], unlockedDisplay)
	})
}

// layoutMerchantRow's unlocked count comes from unlockedDisplay (staged
// edits overlaid on top of the on-disk count, see displayMerchantUnlocked)
// so the row recolors live as the user checks/unchecks flags, not just
// after a save.
func (s *State) layoutMerchantRow(gtx layout.Context, th *material.Theme, name string, unlockedDisplay map[string]int) layout.Dimensions {
	total, unlocked := s.merchantGatedTotal[name], unlockedDisplay[name]
	label := fmt.Sprintf("%s\n%d/%d gates unlocked", name, unlocked, total)
	btn := formButton(th, s.merchantUnlockBtn(name), label)
	switch {
	case s.UnlockMerchant == name:
		btn.Background = colorAccent
		btn.Color = th.Palette.ContrastFg
	case unlocked < total:
		btn.Background = colorInputBg
		btn.Color = colorWarnTxt
	default:
		btn.Background = colorInputBg
		btn.Color = colorMuted
	}
	return layout.Inset{Bottom: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
}

func (s *State) layoutFlagsColumn(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if s.SelectedChar < 0 {
		lbl := material.Body2(th, "Pick a character, then a merchant, to see its flags.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	if s.UnlockMerchant == "" {
		lbl := material.Body2(th, "Pick a merchant to see its flags.")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}

	lockedCount := s.flagsColumnLockedCount()
	allLockedCount := s.allMerchantsLockedCount()
	merchantUndo := s.merchantUnlockUndo != nil
	allUndo := s.allMerchantsUndo != nil

	header := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.H6(th, s.UnlockMerchant).Layout),
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := fmt.Sprintf("Unlock all merchants (%d)", allLockedCount)
				if allUndo {
					label = "Undo all-merchant unlock"
				}
				return actionButton(gtx, th, &s.unlockAllMerchantsBtn, label, allUndo || allLockedCount > 0)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := fmt.Sprintf("Unlock this merchant (%d)", lockedCount)
				if merchantUndo {
					label = "Undo merchant unlock"
				}
				return actionButton(gtx, th, &s.unlockAllBtn, label, merchantUndo || lockedCount > 0)
			}),
		)
	})

	body := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		if s.UnlockMerchant == twinMaidenHusksMerchantName {
			return s.layoutTMHFlagsGrid(gtx, th)
		}
		if scrollFlagsMerchants[s.UnlockMerchant] {
			return s.layoutScrollFlagsGrid(gtx, th)
		}
		groups := groupFlagRows(s.FlagRows)
		s.flagColList.Axis = layout.Vertical
		return material.List(th, &s.flagColList).Layout(gtx, len(groups), func(gtx layout.Context, i int) layout.Dimensions {
			return s.layoutFlagRow(gtx, th, groups[i])
		})
	})

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		header,
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		body,
	)
}

// allMerchantsLockedCount is the total number of still-locked checkbox
// groups across the currently selected character. It intentionally uses the
// same pending-aware merchant counts as the left-hand merchant list so the
// button label stays truthful immediately after any toggle.
func (s *State) allMerchantsLockedCount() int {
	if s.SelectedChar < 0 {
		return 0
	}
	unlocked := s.displayMerchantUnlocked()
	n := 0
	for name, total := range s.merchantGatedTotal {
		n += total - unlocked[name]
	}
	return n
}

// flagsColumnLockedCount is the header's "Unlock this merchant (N)" count --
// her own gated-row groups plus, for Twin Maiden Husks specifically, every
// still-unacquired bell bearing (character.BellBearingsForUI), since that
// button covers both staging mechanisms (see the unlockAllBtn handler in
// layoutCharactersPanel).
func (s *State) flagsColumnLockedCount() int {
	n := 0
	for _, g := range groupFlagRows(s.FlagRows) {
		if !s.effectiveFlagGroupUnlocked(g) {
			n++
		}
	}
	if s.UnlockMerchant == twinMaidenHusksMerchantName {
		for _, b := range character.BellBearingsForUI() {
			if !s.effectiveBellBearingUnlocked(b.FlagID) {
				n++
			}
		}
	}
	return n
}

// flagGroupCheckbox resolves a flag group's retained checkbox and re-seeds
// its .Value from FlagState overlaid with any staged PendingFlagEdits (staged
// wins), returning whether a staged edit was found so the caller can amber-
// tint the label. Shared by layoutFlagRow and layoutFlagRowNamed, whose only
// real difference is which label/subtext they draw around this same checkbox.
// Overwriting .Value here is safe: this runs during layout, AFTER
// layoutCharactersPanel's own chk.Update(gtx)/stageFlag pass already consumed
// this frame's click and updated PendingFlagEdits, so recomputing from it
// reflects that same click, not a stale prior one. Necessary because a toggle
// staged from OUTSIDE this column (the Shop Editor row-edit bar's Unlock/Lock
// button via setRowUnlockForSelectedChar) only updates PendingFlagEdits, so a
// column left expanded across a view switch would otherwise show a stale
// checkbox.
func (s *State) flagGroupCheckbox(g flagGroup) (chk *widget.Bool, staged bool) {
	val := s.FlagState[g.Rows[0].RowID]
	pending := s.PendingFlagEdits[s.SelectedChar]
	for _, r := range g.Rows {
		if target, ok := pending[r.RowID]; ok {
			val, staged = target, true
			break
		}
	}
	chk = s.flagCheck(g.FlagID)
	chk.Value = val
	return chk, staged
}

// layoutFlagRow draws one flag-group checkbox, re-deriving its checked state
// via flagGroupCheckbox (see there for why re-deriving every render, rather
// than trusting the last-seeded .Value, is both necessary and safe).
func (s *State) layoutFlagRow(gtx layout.Context, th *material.Theme, g flagGroup) layout.Dimensions {
	chk, staged := s.flagGroupCheckbox(g)

	label := flagGroupLabel(g)
	box := material.CheckBox(th, chk, label)
	if staged {
		box.Color = colorAmber
	}
	return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, box.Layout)
}
