package gio

// Pending-edits controls: a "Pending (N)" toggle + the one shared Save
// button (hoisted to window.go's Layout so it renders identically on every
// view), and a true blocking modal (see layoutPendingOverlay's doc comment)
// listing the staged item edits, GROUPED BY MERCHANT with an optional
// merchant filter, plus any staged character-flag edits. N and the Save
// click cover BOTH kinds -- see state.go's combinedPendingCount/
// startCombinedSave. An EditError from an apply keeps the modal open with
// the error inline until dismissed.

import (
	"fmt"
	"image"
	"image/color"
	"sort"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
)

// pendingModalScrim matches components.Modal's own scrim color -- same "dim
// everything behind a blocking dialog" treatment, reimplemented locally
// since this modal needs bespoke content (filter + grouped scrollable list)
// beyond what the shared components.Modal (title/body/OK) supports.
var pendingModalScrim = color.NRGBA{A: 0xA0}

// pendingIconSize is the small item-icon thumbnail shown on each pending
// row -- big enough to recognize the item at a glance, small enough that a
// long list of entries still stays scannable.
const pendingIconSize = unit.Dp(28)

// pendingRowIDs returns the staged row ids in ascending order (stable across
// frames, unlike Go map iteration).
func (s *State) pendingRowIDs() []int64 {
	ids := make([]int64, 0, len(s.PendingEdits))
	for id := range s.PendingEdits {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// removeBtn returns the retained Remove-button state for a row, creating it on
// first use.
func (s *State) removeBtn(id int64) *widget.Clickable {
	b := s.removeBtns[id]
	if b == nil {
		b = &widget.Clickable{}
		s.removeBtns[id] = b
	}
	return b
}

// removeAllBtn returns the retained "Remove all" button state for a
// merchant group (keyed by pendingMerchantKey), creating it on first use.
func (s *State) removeAllBtn(merchant string) *widget.Clickable {
	b := s.removeAllBtns[merchant]
	if b == nil {
		b = &widget.Clickable{}
		s.removeAllBtns[merchant] = b
	}
	return b
}

// closePendingModal hides the modal and clears any apply error it was
// showing -- shared by the footer toggle-off, the modal's own X button, and
// its scrim click.
func (s *State) closePendingModal() {
	s.pendingOpen = false
	s.ClearApplyErr()
}

// footerStatusMessage derives guidance from current state. Retaining the last
// action as text made the footer drift out of sync after later actions, most
// visibly when cancelling Save As left "Preparing ... to save" on screen.
func (s *State) footerStatusMessage() string {
	if msg := s.BusyMsg(); msg != "" {
		return msg
	}
	if s.Session == nil || s.Catalog == nil || !s.Catalog.Loaded() {
		if s.InlineErr() != "" {
			return "Could not load that file — check the message above"
		}
		return "Open a save file to begin"
	}
	if s.itemInfoOpen {
		return ""
	}
	if s.Picking() {
		return s.pickingStatusMessage()
	}
	count := s.combinedPendingCount()
	if s.ApplyErr() != "" {
		return "Save stopped — review the error and adjust your staged changes"
	}
	if s.pendingOpen {
		return fmt.Sprintf("Review %d staged change%s, then save a new copy", count, plural(count))
	}
	if s.showRowEditor && len(s.editingRows()) > 0 {
		return "Adjust the selected stock, then Apply to stage the changes"
	}
	if count > 0 {
		return fmt.Sprintf("%d change%s staged — review Pending, then save a new copy", count, plural(count))
	}
	if name := s.LoadedName(); name != "" {
		return "Editing " + name + " — the original stays unchanged until you save a copy"
	}
	return "Save loaded — changes are staged until you save a copy"
}

// layoutFooterPendingControls draws the shared footer bar's Save button
// (left), contextual status (centre), and "Pending (N)" toggle (right).
// The centre stays current because footerStatusMessage derives it from state.
func (s *State) layoutFooterPendingControls(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return s.layoutFooterPendingControlsWithStatus(gtx, th, true)
}

// layoutFooterPendingControlsWithStatus lets the replacement-picker scrim
// dim the footer controls while window.go redraws only the status text above
// that scrim. In every ordinary state it is identical to the public wrapper.
func (s *State) layoutFooterPendingControlsWithStatus(gtx layout.Context, th *material.Theme, showStatus bool) layout.Dimensions {
	count := s.combinedPendingCount()
	if count > 0 && s.pendingBtn.Clicked(gtx) {
		if s.pendingOpen {
			s.closePendingModal()
		} else {
			s.pendingOpen = true
		}
	}
	if count > 0 && !s.Busy() && s.saveBtn.Clicked(gtx) {
		s.startCombinedSave()
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if count == 0 {
				return disabledButton(gtx, th, "Save File...")
			}
			if s.Busy() {
				return disabledButton(gtx, th, s.BusyMsg())
			}
			label := fmt.Sprintf("Save File (%d change%s)", count, plural(count))
			return material.Button(th, &s.saveBtn, label).Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !showStatus {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}
			return s.layoutFooterStatus(gtx, th)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// No pending edits = nothing to review -- non-clickable, same
			// treatment as "Save File..." above (else it would open a
			// near-empty "No pending changes." modal for no reason).
			if count == 0 {
				return disabledButton(gtx, th, "Pending (0)")
			}
			label := fmt.Sprintf("Pending (%d)", count)
			btn := material.Button(th, &s.pendingBtn, label)
			btn.Background = colorInputBg
			btn.Color = colorFg // readable on the input bg in both themes
			if s.pendingOpen || s.ApplyErr() != "" {
				btn.Background = colorAccent
				btn.Color = th.Palette.ContrastFg
			} else {
				btn.Color = colorAmber
			}
			return btn.Layout(gtx)
		}),
	)
}

func (s *State) layoutFooterStatus(gtx layout.Context, th *material.Theme) layout.Dimensions {
	message := s.footerStatusMessage()
	if message == "" {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(th, message)
			label.Color = colorAmber
			label.MaxLines = 1
			return label.Layout(gtx)
		}),
	)
}

// layoutFooterStatusOverlay redraws only the status text over a blocking
// modal's scrim. The controls and footer surface remain underneath/dimmed;
// this is purely the readable guidance line at the bottom of the window. It
// uses the footer height measured earlier in the same frame, so its baseline
// is pixel-aligned with the ordinary (undimmed) footer status.
func (s *State) layoutFooterStatusOverlay(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if s.footerStatusMessage() == "" {
		return layout.Dimensions{}
	}
	if s.footerStatusBarHeight <= 0 {
		return layout.Dimensions{}
	}

	// Record first: layoutFooterStatus centres the text horizontally using the
	// full window width. Replaying it after the offset keeps that centring and
	// moves only its vertical position to the exact footer centre.
	rec := op.Record(gtx.Ops)
	textGtx := gtx
	textGtx.Constraints.Min.X = textGtx.Constraints.Max.X
	textDims := s.layoutFooterStatus(textGtx, th)
	call := rec.Stop()

	footerBottom := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(10))
	footerTop := footerBottom - s.footerStatusBarHeight
	textTop := footerTop + (s.footerStatusBarHeight-textDims.Size.Y)/2
	off := op.Offset(image.Pt(0, textTop)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return textDims
}

// footerStatusOverlayActive says whether a later, full-window overlay redraws
// the status line above its scrim. The ordinary footer must omit its own copy
// in these states, otherwise the dimmed original and bright overlay duplicate
// each other (and appear vertically misaligned).
func (s *State) footerStatusOverlayActive() bool {
	if s.Picking() || s.pendingOpen || s.ApplyErr() != "" {
		return true
	}
	return s.showRowEditor && len(s.editingRows()) > 0
}

// layoutPendingOverlay draws the Pending Edits review modal: a full-window
// scrim (blocks/dims everything else, like a warning dialog) plus a
// centered bordered panel. Visible when toggled open or when an apply error
// needs attention.
func (s *State) layoutPendingOverlay(gtx layout.Context, th *material.Theme) {
	if !s.pendingOpen && s.ApplyErr() == "" {
		return
	}
	if s.pendingCloseBtn.Clicked(gtx) {
		s.closePendingModal()
	}
	for s.pendingScrim.Clicked(gtx) {
		s.closePendingModal()
	}

	components.Backdrop(gtx,
		// Note: pendingModalHit is drained in afterPanel and is read later in
		// the SAME frame -- Backdrop's content()/afterPanel() callbacks run
		// synchronously during this call (op.Defer only defers the resulting
		// paint/hit-test OPS, not the Go calls that build them).
		// Same scrim alpha, 1dp border and 16dp inset as components.Modal, but a
		// wider (640dp, avail-clamped) panel, a 70%-height cap for the
		// scrollable list, the editor theme's panel bg/divider, and a 4dp
		// corner -- all preserved exactly from the pre-extraction inline code.
		components.BackdropStyle{
			Scrim:        pendingModalScrim,
			PanelBg:      colorPanelBg,
			BorderColor:  colorDivider,
			BorderWidth:  unit.Dp(1),
			CornerRadius: unit.Dp(4),
			Inset:        unit.Dp(16),
		},
		&s.pendingScrim,
		&s.pendingPanelBlocker,
		&s.pendingScrimPressTag,
		s.closePendingModal,
		func(gtx *layout.Context) {
			maxW := gtx.Dp(unit.Dp(640))
			if avail := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(48)); avail < maxW {
				maxW = avail
			}
			gtx.Constraints.Max.X = maxW
			gtx.Constraints.Max.Y = gtx.Constraints.Max.Y * 7 / 10
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return s.layoutPendingPanel(gtx, th)
		},
		// A click anywhere in the modal (including the scrim, which just closes
		// it) must not also cascade into the global outside-click deselect
		// logic (window.go) -- that would blow away the catalog/row selection
		// underneath just because the user reviewed or dismissed this dialog.
		func(gtx layout.Context) {
			s.pressListener(gtx, &s.pendingModalTag, gtx.Constraints.Max, &s.pendingModalHit)
			s.layoutFooterStatusOverlay(gtx, th)
		},
	)
}

// layoutPendingPanel is the modal's chrome: title + X close, a thin gold
// rule (same Elden-Ring-menu accent the row-edit bar uses), then the body.
func (s *State) layoutPendingPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	header := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, material.H6(th, "Pending Edits").Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formButton(th, &s.pendingCloseBtn, "X").Layout(gtx)
		}),
	)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return header }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(goldRule),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.layoutPendingBody(gtx, th) }),
	)
	return dims
}

// layoutPendingBody lays out the apply-error banner (if any), the merchant
// filter (only shown once there's more than one merchant to filter between
// -- a filter over a single merchant's changes is just clutter), and the
// scrollable grouped content. A Rigid (not Flexed) child of the panel's
// Vertical Flex on purpose: layoutPendingOverlay already caps Max.Y at 70%
// of the window, so this sizes to its OWN content up to that ceiling
// (short lists render small and clean, long lists cap and scroll) instead
// of always ballooning to fill whatever space a Flexed share would claim.
func (s *State) layoutPendingBody(gtx layout.Context, th *material.Theme) layout.Dimensions {
	ids := s.pendingRowIDs()
	for _, id := range ids {
		if s.removeBtn(id).Clicked(gtx) {
			s.RemoveRowEdits(id)
		}
	}
	ids = s.pendingRowIDs() // re-read in case a Remove pruned an entry

	for _, m := range pendingMerchantNames(s.PendingEdits, ids) {
		if s.removeAllBtn(m).Clicked(gtx) {
			s.RemoveMerchantEdits(m)
		}
	}
	ids = s.pendingRowIDs() // re-read again in case a Remove-all pruned entries

	{
		seen := map[int]bool{}
		for idx := range s.PendingFlagEdits {
			seen[idx] = true
		}
		for idx := range s.PendingBellBearingEdits {
			seen[idx] = true
		}
		for idx := range seen {
			if s.removeFlagBtn(idx).Clicked(gtx) {
				s.RemovePendingFlagsForChar(idx)
			}
		}
	}

	// Nothing left to review (item edits AND character-flag edits both
	// empty) -- auto-close rather than leave the user staring at an empty
	// "No pending changes." dialog they have to dismiss themselves.
	if s.combinedPendingCount() == 0 && s.ApplyErr() == "" {
		s.closePendingModal()
	}

	merchants := pendingMerchantNames(s.PendingEdits, ids)
	options := append([]string{""}, merchants...)
	labels := append([]string{"All merchants"}, merchants...)
	s.pendingMerchant.SetOptionsWithLabels(options, labels)
	s.pendingMerchant.SetValue(s.pendingMerchantFilter)

	children := []layout.FlexChild{}
	if msg := s.ApplyErr(); msg != "" {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, msg)
				lbl.Color = colorError
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		)
	}
	if len(merchants) > 1 {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := s.pendingMerchant.Layout(gtx, th, unit.Dp(260))
				s.pendingMerchantFilter = s.pendingMerchant.Value()
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		)
	}

	if len(ids) == 0 && s.totalPendingFlagCount() == 0 && s.totalPendingBellBearingCount() == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, "No pending changes.")
			lbl.Color = colorMuted
			return lbl.Layout(gtx)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return s.layoutPendingScrollList(gtx, th, ids)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pendingMerchantGroup is one merchant's staged item-edit rows.
type pendingMerchantGroup struct {
	merchant string
	ids      []int64
}

// pendingMerchantKey is a RowEdit's group-by-merchant key -- the single
// place the "" -> "Unknown merchant" fallback is decided, shared by
// groupPendingByMerchant and State.RemoveMerchantEdits (staging.go) so a
// group header's "Remove all" button always matches exactly the rows that
// header groups, even for the fallback bucket.
func pendingMerchantKey(e *RowEdit) string {
	if e.Merchant == "" {
		return "Unknown merchant"
	}
	return e.Merchant
}

// groupPendingByMerchant buckets staged row edits by their captured
// Merchant name (name-sorted, so the list order is stable across frames
// regardless of staging order).
func groupPendingByMerchant(edits map[int64]*RowEdit, ids []int64) []pendingMerchantGroup {
	idx := map[string]int{}
	var groups []pendingMerchantGroup
	for _, id := range ids {
		e := edits[id]
		if e == nil {
			continue
		}
		m := pendingMerchantKey(e)
		if i, ok := idx[m]; ok {
			groups[i].ids = append(groups[i].ids, id)
			continue
		}
		idx[m] = len(groups)
		groups = append(groups, pendingMerchantGroup{merchant: m, ids: []int64{id}})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].merchant < groups[j].merchant })
	return groups
}

// pendingMerchantNames is groupPendingByMerchant's merchant names alone, for
// the filter combo's option list.
func pendingMerchantNames(edits map[int64]*RowEdit, ids []int64) []string {
	groups := groupPendingByMerchant(edits, ids)
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.merchant
	}
	return names
}

// layoutPendingScrollList renders one flattened, scrollable list mixing
// merchant-group headers, their rows, dividers, and (at the end) the
// character-flag section -- material.List needs one homogeneous per-index
// builder, so each visual line/block is captured as its own closure rather
// than trying to express the grouping as nested lists.
func (s *State) layoutPendingScrollList(gtx layout.Context, th *material.Theme, ids []int64) layout.Dimensions {
	filter := s.pendingMerchantFilter
	groups := groupPendingByMerchant(s.PendingEdits, ids)

	// Debug-mode-only sellValue preview -- computed once per frame here,
	// not per row, and only while the sell-value display is on, since it's
	// the same aggregate pass BuildEquipParamEdits itself does.
	var equipTargets map[equipParamRef]*equipParamTarget
	var equipRowRefs map[int64]equipParamRef
	if s.Settings.ShowSellValueChanges {
		equipTargets, equipRowRefs, _ = s.computeEquipParamTargets()
	}

	var blocks []layout.Widget
	for _, mg := range groups {
		if filter != "" && mg.merchant != filter {
			continue
		}
		if len(blocks) > 0 {
			blocks = append(blocks,
				layout.Spacer{Height: unit.Dp(10)}.Layout,
				horizontalDivider,
				layout.Spacer{Height: unit.Dp(10)}.Layout,
			)
		}
		blocks = append(blocks, func(gtx layout.Context) layout.Dimensions {
			return s.layoutMerchantGroupHeader(gtx, th, mg)
		})
		for j, id := range mg.ids {
			blocks = append(blocks, layout.Spacer{Height: unit.Dp(6)}.Layout)
			blocks = append(blocks, func(gtx layout.Context) layout.Dimensions {
				return s.pendingRow(gtx, th, id, equipTargets, equipRowRefs)
			})
			if j < len(mg.ids)-1 {
				blocks = append(blocks, layout.Spacer{Height: unit.Dp(6)}.Layout)
			}
		}
	}
	if s.totalPendingFlagCount() > 0 || s.totalPendingBellBearingCount() > 0 {
		if len(blocks) > 0 {
			blocks = append(blocks, layout.Spacer{Height: unit.Dp(14)}.Layout)
		}
		// A bordered card (groupBox), not just a divider+heading, so the
		// character unlocks read as their own section rather than one more
		// entry mixed into the item-edit flow.
		blocks = append(blocks, func(gtx layout.Context) layout.Dimensions {
			return groupBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(material.Body1(th, "Character unlocks").Layout),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPendingFlagList(gtx, th)
					}),
				)
			})
		})
	}

	s.pendingList.Axis = layout.Vertical
	return material.List(th, &s.pendingList).Layout(gtx, len(blocks), func(gtx layout.Context, i int) layout.Dimensions {
		return blocks[i](gtx)
	})
}

// layoutMerchantGroupHeader is one merchant's section header: name plus a
// muted "(N change(s))" count, so it's obvious at a glance how many rows
// are staged for that merchant without counting them, and a right-aligned
// "Remove all" button that discards every staged edit for that merchant in
// one click.
func (s *State) layoutMerchantGroupHeader(gtx layout.Context, th *material.Theme, mg pendingMerchantGroup) layout.Dimensions {
	n := len(mg.ids)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(material.Body1(th, mg.merchant).Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(th, fmt.Sprintf("(%d change%s)", n, plural(n)))
			lbl.Color = colorMuted
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formButton(th, s.removeAllBtn(mg.merchant), "Remove all").Layout(gtx)
		}),
	)
}

// layoutPendingFlagList lists, per character with a staged flag or
// bell-bearing edit, how many are staged ("<name>: N flag(s) staged")
// plus a "Remove all" button that discards every staged edit (both kinds)
// for that character -- the only way to undo a staged unlock from this
// modal (otherwise it's re-toggling each checkbox in the Characters view).
// A per-FLAG breakdown (mirroring the item list's one-row-per-edit) isn't
// shown -- flags are already grouped per bell-bearing/quest-flag on the
// Characters side, and a raw count is enough to confirm what's staged
// before committing. Bell-bearing acquisition toggles
// (PendingBellBearingEdits) fold into the same per-character count rather
// than getting their own line, matching RemovePendingFlagsForChar
// clearing both maps together.
func (s *State) layoutPendingFlagList(gtx layout.Context, th *material.Theme) layout.Dimensions {
	seen := map[int]bool{}
	for idx, m := range s.PendingFlagEdits {
		if len(m) > 0 {
			seen[idx] = true
		}
	}
	for idx, m := range s.PendingBellBearingEdits {
		if len(m) > 0 {
			seen[idx] = true
		}
	}
	indices := make([]int, 0, len(seen))
	for idx := range seen {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	if len(indices) == 0 {
		lbl := material.Body2(th, "(none)")
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	}
	children := make([]layout.FlexChild, 0, len(indices))
	for _, idx := range indices {
		// Match the shared footer: several staged ShopLineup rows can be
		// one checkbox because they share a single UnlockFlag.
		n := s.pendingFlagCount(idx) + s.pendingBellBearingCount(idx)
		text := fmt.Sprintf("%s: %d flag%s staged", s.charName(idx), n, plural(n))
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, material.Body2(th, text).Layout),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return formButton(th, s.removeFlagBtn(idx), "Remove all").Layout(gtx)
						}),
					)
				})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pendingRow renders one staged row's entry, then indented sub-lines
// describing each staged change. Two header shapes:
//   - An actual item swap (FromName != ToName) gets pendingSwapHeader: the
//     OLD item (icon + name, muted -- it's being superseded), an arrow,
//     then the NEW item (icon + name, normal color -- it's what actually
//     ships). Each item's icon+name is stated ONCE, not twice.
//   - Everything else (no swap, or a level-only change where the item
//     itself didn't change) gets pendingSimpleHeader: one icon + one name,
//     same as a plain field-only edit.
func (s *State) pendingRow(gtx layout.Context, th *material.Theme, id int64, equipTargets map[equipParamRef]*equipParamTarget, equipRowRefs map[int64]equipParamRef) layout.Dimensions {
	e := s.PendingEdits[id]
	if e == nil {
		return layout.Dimensions{}
	}
	oldLabel := e.Label

	swapped := e.ItemChange != nil && !e.ItemChange.IsLevelOnly()
	subIndent := pendingIconSize + unit.Dp(8)

	var children []layout.FlexChild
	if swapped {
		ic := e.ItemChange
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.pendingSwapHeader(gtx, th, id, oldLabel, e.IconPath, ic.DisplayName(), ic.IconPath)
		}))
	} else {
		label, iconPath := oldLabel, e.IconPath
		if e.ItemChange != nil { // level-only: same item, show it at its new level
			label, iconPath = e.ItemChange.DisplayName(), e.ItemChange.IconPath
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.pendingSimpleHeader(gtx, th, id, label, iconPath)
		}))
	}

	if e.ItemChange != nil {
		ic := e.ItemChange
		if ic.IsLevelOnly() {
			children = append(children, layout.Rigid(indentedText(th,
				fmt.Sprintf("Level: +%d", ic.Level), subIndent)))
		}
	}
	// Field changes in a stable (name-sorted) order.
	names := make([]string, 0, len(e.FieldChanges))
	for name := range e.FieldChanges {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ch := e.FieldChanges[name]
		children = append(children, layout.Rigid(indentedText(th,
			describeChange(name, ch.From, ch.To, e.CostType), subIndent)))
	}

	// Sell-value line, behind Settings.ShowSellValueChanges (opt-in, see
	// settings.go). Comes straight from the same computeEquipParamTargets
	// pass the real write uses, so it shows exactly what will be written.
	// Phrased in player terms -- see sellValueChangeText.
	if s.Settings.ShowSellValueChanges {
		if ref, ok := equipRowRefs[id]; ok {
			if t := equipTargets[ref]; t != nil {
				if text := sellValueChangeText(t); text != "" {
					children = append(children, layout.Rigid(indentedText(th, text, subIndent)))
				}
			}
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pendingSimpleHeader is one icon + one name + a right-aligned Remove
// button -- used for field-only edits and level-only changes, where
// there's exactly one item to show (no before/after).
func (s *State) pendingSimpleHeader(gtx layout.Context, th *material.Theme, id int64, label, iconPath string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return framedIcon(gtx, s.Icons.Get(iconPath), pendingIconSize)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, material.Body2(th, label).Layout),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formButton(th, s.removeBtn(id), "Remove").Layout(gtx)
		}),
	)
}

// pendingSwapHeader is the one-line "old -> new" header for an actual item
// swap: old item (icon+name) and new item (icon+name) side by side on the
// SAME line, same text styling for both. Each item's block gets an EQUAL
// Flexed(1) share of the row, which -- since every row in the list has the
// same overall width and the same fixed-size siblings (both icons, the
// arrow, Remove) -- makes the old-item column and new-item column line up
// vertically across every swap row in the list, not just within one row.
func (s *State) pendingSwapHeader(gtx layout.Context, th *material.Theme, id int64, oldLabel, oldIcon, newLabel, newIcon string) layout.Dimensions {
	itemBlock := func(iconPath, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return framedIcon(gtx, s.Icons.Get(iconPath), pendingIconSize)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, material.Body2(th, label).Layout),
			)
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, itemBlock(oldIcon, oldLabel)),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(th, "→")
			lbl.Color = colorAmber
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, itemBlock(newIcon, newLabel)),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formButton(th, s.removeBtn(id), "Remove").Layout(gtx)
		}),
	)
}

// indentedText renders a sub-line of a pending-edit entry, indented under
// its row header by the given amount (rows with a leading icon indent
// further, so the text lines up under the LABEL rather than the icon; the
// flag-changes list has no icon and uses a smaller flat indent).
func indentedText(th *material.Theme, text string, indent unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: indent, Top: unit.Dp(1)}.Layout(gtx,
			material.Body2(th, text).Layout)
	}
}

// disabledButtonChrome renders a non-interactive, muted button: the
// caller's already-styled label (recolored to colorMuted) inside inset, on a
// colorDisabled rounded rect (4dp radius). No widget.Clickable is laid out at
// all, so it swallows nothing and reacts to nothing. Shared by disabledButton
// (footer, Body1 + 10dp vertical inset) and row_edit_form.go's
// disabledBarButton (bar actions, smaller Body2 + 6dp vertical inset), which
// differ only in that label style and inset.
func disabledButtonChrome(gtx layout.Context, inset layout.Inset, lbl material.LabelStyle) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl.Color = colorMuted
		return lbl.Layout(gtx)
	})
	call := macro.Stop()

	rr := gtx.Dp(unit.Dp(4))
	bg := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: rr, SW: rr, NE: rr, NW: rr}
	paint.FillShape(gtx.Ops, colorDisabled, bg.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// disabledButton renders a non-interactive, muted footer button.
func disabledButton(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	return disabledButtonChrome(gtx,
		layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12},
		material.Body1(th, label))
}

// plural returns "s" unless n == 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// sellValueChangeText phrases one item's sellValue bookkeeping for the
// Pending Edits modal (Settings.ShowSellValueChanges), in player terms
// rather than the raw field name.
//
// Background: a merchant row is only VISIBLE in-game when its price is >=
// the item's own sellValue (docs/MERCHANT_DATA.md), and sellValue is a
// property of the ITEM, not of the row -- one value shared by every
// merchant selling it. So pricing something below its sell value silently
// forces that value down game-wide, which is a real consequence worth
// stating explicitly instead of leaving the user to discover it.
//
// Returns "" when there's nothing meaningful to say (unresolvable item),
// so the caller simply omits the line rather than printing a confusing
// "unknown".
func sellValueChangeText(t *equipParamTarget) string {
	if !t.CurrentOK {
		return ""
	}
	if t.Current == t.Target {
		return fmt.Sprintf("Sell value: %d, unchanged", t.Current)
	}
	return fmt.Sprintf("Sell value: %d -> %d", t.Current, t.Target)
}
