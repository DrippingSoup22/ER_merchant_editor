package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ncruces/zenity"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/charunlock"
	"er_merchant_editor/app/editor/widgets"
	"er_merchant_editor/app/shopwrite"
)

// noRow marks "no row selected / not picking".
const noRow int64 = -1

// Views the header button switches between.
const (
	viewEditor = iota
	viewSettings
	viewCharacters
)

// FieldChange is a single staged scalar field edit (raw param field name ->
// from/to). From is captured at stage time from the row's decoded value.
type FieldChange struct{ From, To int64 }

// ItemChange is a staged item swap for a row: the display bits for the
// pending-edits UI plus the equipId/equipType the write-back needs.
// ClearOverrides lists the row's display-override fields (iconId/nameMsgId/
// menuTitleMsgId/menuIconId that were non--1 at stage time) which the swap
// resets to -1 so the new item shows its own name/icon; it lives on the
// swap (not FieldChanges) so undoing the swap drops the resets with it.
//
// BaseEquipID/Level (weapon reinforcement) exist only for
// weapon-table items (EquipType == 0): BaseEquipID is the item's own +0
// equipId (== its unified item id, since weapon offset is 0 -- see
// docs/ITEM_IDS.md), Level is the staged "+N" reinforcement (0 =
// unleveled). EquipID -- the id write-back actually uses -- is always kept
// equal to BaseEquipID+Level; BuildEdits reads EquipID as-is and needs no
// knowledge of leveling at all. ToName never includes the "+N" suffix
// (DisplayName appends it) so re-leveling never double-suffixes.
type ItemChange struct {
	FromName       string
	ToName         string
	IconPath       string
	EquipID        int64
	EquipType      int64
	BaseEquipID    int64
	Level          int64
	ClearOverrides []string
}

// DisplayName is ToName with a "+N" reinforcement suffix when Level > 0 --
// the game appends this client-side too (item names are level-agnostic in
// the underlying data), so this is purely a presentation concern, computed
// fresh every time rather than baked into ToName.
func (ic *ItemChange) DisplayName() string {
	if ic.Level > 0 {
		return fmt.Sprintf("%s +%d", ic.ToName, ic.Level)
	}
	return ic.ToName
}

// IsLevelOnly reports whether this change only adjusts a weapon's
// reinforcement level (the item itself is unchanged) -- lets pending-edit
// displays say "level change" instead of the misleading "swap"/
// "replacement" wording when nothing was actually swapped.
func (ic *ItemChange) IsLevelOnly() bool { return ic.FromName == ic.ToName }

// RowEdit stages one row's pending edits: scalar field changes keyed by raw
// param field name, plus an optional item swap. Mirrors the Python
// pending_edits entry shape ({label, cost_type, field_changes, item_change}).
// Merchant is the canonical merchant name captured at stage time (the row's
// own Merchant field is the raw, sometimes-empty Paramdex name -- see
// enrich.go -- so it's not reusable here); normal mode displays it instead
// of the raw row id.
type RowEdit struct {
	Label        string
	Merchant     string
	CostType     int64
	IconPath     string // the row's own current icon at stage time (fallback display icon when ItemChange == nil)
	FieldChanges map[string]FieldChange
	ItemChange   *ItemChange
}

// draftGateEdit is one drafted gate-unlock toggle: row is a representative
// row carrying the target UnlockFlag (setRowUnlockForSelectedChar fans out
// to every row sharing it, so any one suffices), target is the drafted
// unlocked state.
type draftGateEdit struct {
	row    *catalog.Row
	target bool
}

// cellState is the retained per-cell widget state (click/hover), keyed by
// item id (catalog grid) or row id (merchant grid). Tooltips are immediate-
// mode inside IconCell and need no retained state. drag is the catalog-cell
// drag-source gesture (unused by merchant cells, which are drop targets keyed
// by the cellState pointer itself as the transfer tag).
type cellState struct {
	click widget.Clickable
	drag  widget.Draggable
}

// itemsMIME is the in-app drag-and-drop payload type: a comma-separated list of
// items.json ids. Self-contained so the drop reads it without side-channel state.
const itemsMIME = "application/x-er-items"

// rowsMIME is the merchant-grid-internal drag-and-drop payload type: a
// single merchant row_id (the dragged row itself, never a multi-select
// group -- "just to swap items", the user's own framing). A separate MIME
// from itemsMIME so a merchant cell's drop target can tell "swap with this
// other row" (rowsMIME) apart from "replace with this catalog item"
// (itemsMIME) without any payload-shape sniffing.
const rowsMIME = "application/x-er-merchant-row"

// itemFilterKey identifies one catalog-filter combination (memoization key).
// risky is part of the key because the default view excludes risky items
// (Settings.ShowRiskyItems).
type itemFilterKey struct {
	category, subCat, search string
	risky                    bool
}

// State is the whole editor's shared, retained UI state. All fields are owned
// by the UI (frame) goroutine except those guarded by mu, which the save-load
// worker goroutine also touches.
type State struct {
	Catalog *catalog.Catalog
	Icons   *IconCache
	win     *app.Window

	// --- save-switcher / load lifecycle (mu-guarded) ---
	mu           sync.Mutex
	busy         bool   // a load or apply is in progress
	busyMsg      string // what the busy state is doing ("Loading save...", "Saving...")
	loadedName   string // base name of the loaded save ("" = none)
	inlineErr    string // save-switcher inline error text
	dialogOpen   bool   // a native file dialog is open
	resetPending bool   // a save just loaded; UI state must reset next frame
	applyDone    bool   // an apply just succeeded; staging must clear next frame
	applyErr     string // pending-bar inline error (EditError text)
	modalErr     string // unexpected-error modal text ("" = hidden)

	openBtn widget.Clickable

	// --- catalog panel ---
	Search          widget.Editor
	CategoryCombo   widgets.Combo
	SubCatCombo     widgets.Combo
	subCatFilterHas bool // whether the current category has any real sub-categories (see refreshSubcategories)
	CatalogList     widget.List
	itemCells       map[int64]*cellState
	itemsCache      []*catalog.Item // memoized filter result (see filteredItems)
	itemsCacheKey   itemFilterKey

	// PickLevel (weapon reinforcement) is the shared "+N" level
	// applied to whatever weapon-table item gets picked/dragged next (see
	// stageItemSwapCore) -- one control for the whole catalog, not one per
	// cell, mirroring the single search/category/subcategory filter bar.
	// Clamped per-item at stage time (a level too high for the specific
	// item picked silently clamps to that item's own max), so this can
	// safely sit above any one item's cap.
	PickLevel   int64
	levelEditor widget.Editor
	levelSlider widget.Float

	// Catalog multi-select. SelectedItems is ORDERED — the order defines the
	// drag-and-drop replacement order. selAnchor is the shift-range pivot;
	// selAnchorSet guards its validity (item ids may legitimately be 0).
	SelectedItems []int64
	selAnchor     int64
	selAnchorSet  bool
	dragCount     int   // items in the in-flight drag payload
	dropHoverRow  int64 // merchant row under the cursor during a drag (noRow = none)

	// Strict-deselect press tracking (frame-scoped). Distinct int fields so
	// their addresses are unique event tags for pass-through Press listeners.
	pressTag              int
	catalogAreaTag        int
	merchantAreaTag       int
	editBtnAreaTag        int // press area for the "Edit (N)" button (excluded from strict-deselect)
	pendingModalTag       int
	pendingScrimPressTag  int
	itemInfoModalTag      int
	itemInfoScrimPressTag int
	rowEditModalTag       int
	rowEditScrimPressTag  int
	// dimTagHeader/dimTagMerchant/dimTagFooter are opaque-input-blocker tags
	// for the "obscure everything but the catalog" scrim shown while picking
	// a replacement item (see window.go's dimOverlay) -- distinct addresses,
	// same reasoning as the press-listener tags above.
	dimTagHeader     int
	dimTagMerchant   int
	dimTagFooter     int
	pressAnywhere    bool // a pointer press happened somewhere this frame
	catalogAreaHit   bool // ...inside the catalog grid (cells/scrollbar/gaps)
	merchantAreaHit  bool // ...inside the merchant grid
	editBtnHit       bool // ...on the "Edit (N)" button (must not clear the selection it acts on)
	pendingModalHit  bool // ...inside the pending-edits modal (including scrim; blocks global deselection)
	itemInfoModalHit bool // ...inside the item-info modal (including scrim; blocks global deselection)
	rowEditModalHit  bool // ...inside the row-edit modal (including scrim; blocks global deselection)

	// --- item-info popup (right-click a sellable cell, either grid) ---
	itemInfoOpen         bool
	itemInfoID           int64 // catalog item id currently shown; meaningless while itemInfoOpen is false
	itemInfoLevel        int   // weapon "+N" upgrade level to show stats at (0 = base); meaningless while closed
	itemInfoScrim        widget.Clickable
	itemInfoPanelBlocker widget.Clickable
	itemInfoCloseBtn     widget.Clickable
	itemInfoList         widget.List

	// --- merchant panel ---
	MerchantCombo         widgets.Combo
	MerchantCategoryCombo widgets.Combo
	merchantCategoryHas   bool // whether the current merchant has multiple categories among its rows (see fetchMerchantRows)
	merchantGamePreview   bool // false = stable editable ShopLineupParam slots; true = Elden Ring's sorted menu preview
	merchantEditModeBtn   widget.Clickable
	merchantPreviewBtn    widget.Clickable
	MerchantList          widget.List
	rowCells              map[int64]*cellState
	MerchantRows          []*catalog.Row // rows for the currently shown merchant
	merchantsPath         string         // save path the merchant combo was populated from
	merchantsShowCounts   bool           // Settings.ShowMerchantRowCounts the combo's labels were built with
	lastMerchant          string         // merchant the row cache was fetched for

	// SelectedRowIDs is the merchant grid's multi-selection (ORDERED, the same
	// ctrl-click-toggle / shift-click-range / plain-click-replace scheme as
	// the catalog's SelectedItems -- see toggleSelectRow/selectRowRange/
	// selectRowPlain). rowSelAnchor is the shift-range pivot; rowSelAnchorSet
	// guards its validity.
	SelectedRowIDs  []int64
	rowSelAnchor    int64
	rowSelAnchorSet bool
	// showRowEditor gates the row-edit modal (a floating overlay since
	// 2026-08-03, matching Pending Edits/Item Info -- previously docked
	// inline below the grid). Selecting rows (plain OR Ctrl/Shift) NEVER
	// opens it -- a selection is just a selection, usable for a drag-swap
	// without the form eating grid space. The form opens ONLY when the user
	// clicks the explicit "Edit (N)" button (editBtn), and closes when the
	// selection is cleared (clearRowSelection resets this). User request:
	// one selection must serve both swapping and multi-row editing.
	showRowEditor bool
	editBtn       widget.Clickable
	rowEditScrim  widget.Clickable

	// dragSrc is the catalog OR merchant-row cell whose Draggable is (or was
	// last) in a drag; its Dragging() is the ground truth that refreshes
	// dndActive each frame.
	dragSrc *cellState
	// dndActive reports that a compatible item drag (catalog-origin or
	// merchant-row-origin) is in flight (recomputed every frame from
	// dragSrc.drag.Dragging() at Layout top). Drives the drop-span highlight.
	dndActive bool
	// transferInit marks that the drag actually MOVED (the router sends
	// InitiateEvent to targets on the first move-while-pressed over a
	// source; a mere press never does). Gates the ghost, the drop-span
	// highlight, and the deselect-when-the-instance-ends rule, so static
	// clicks keep their plain select/deselect semantics.
	transferInit bool
	// dragFromRow is the source row id when the in-flight drag started on a
	// MERCHANT cell (rowsMIME, internal swap) rather than a catalog cell
	// (itemsMIME, replace) -- noRow when not applicable. Distinguishes the
	// two payload kinds for handleRowDrop/dropSpan, and tells window.go's
	// Layout not to clear the CATALOG selection (clearSelection) when a
	// merchant-origin drag ends, since it never touched that selection.
	dragFromRow int64

	// PickingForRows is the QUEUE of rows still waiting for a picked item
	// (empty = not picking) -- a snapshot of SelectedRowIDs taken when
	// "Change item" is clicked. consumeNextPickingRow pops one row per
	// catalog click (each pick replaces one queued row, not all -- see its
	// doc comment).
	PickingForRows []int64

	// footerStatus is an event-driven message shown in the otherwise empty
	// centre of the shared bottom bar. It is deliberately not a permanent
	// hint: controls set it when their result needs explaining.
	footerStatus          string
	footerStatusBarHeight int // measured each frame; lets modal overlays reuse the footer's exact baseline

	// PendingEdits stages edits per row id -- the REAL, already-committed
	// staging map that BuildEdits/the Pending Edits modal/the Save button
	// all read. The row-edit bar no longer writes here directly (see
	// draftEdits below).
	PendingEdits map[int64]*RowEdit

	// --- row-edit bar draft ---
	//
	// Every field/item/level/gate edit made in the docked row-edit bar
	// lands in this DRAFT first, not PendingEdits -- only the bar's Apply
	// button (applyDraft, row_edit_form.go) commits it, and Apply then
	// closes the bar itself (clearRowSelection). The X button and clicking
	// outside the bar both close it WITHOUT applying, discarding whatever
	// was drafted (item icons, cost/quantity, and gate unlocks all revert
	// to their pre-draft values). draftRowIDs is the
	// selection snapshot the draft belongs to; ensureDraft (called every
	// frame layoutMerchantPanel runs) reseeds draftEdits fresh from
	// PendingEdits the moment SelectedRowIDs no longer matches it -- that
	// one check is what implements both "switched to a different
	// selection" and "closed then reopened" as plain reseeds, no separate
	// close-handling needed. draftGateEdits stages gate-unlock drafts keyed
	// by UnlockFlag (one entry per distinct flag touched this session,
	// written by either the single-row Unlock/Lock button -- toggleDraftGate
	// -- or the multi-select "Unlock all" button -- draftUnlockAll); a flag
	// absent from the map is untouched this session.
	draftRowIDs    []int64
	draftEdits     map[int64]*RowEdit
	draftGateEdits map[int64]draftGateEdit

	// formItemCells backs the multi-select preview grid's per-row hover
	// state (formMultiItemList) -- a SEPARATE retained map from rowCells
	// (the main merchant grid's own cellState), since the two would
	// otherwise register the same widget.Clickable's input area twice in
	// one frame for a row shown in both places.
	formItemCells map[int64]*widget.Clickable

	// --- edit form (below the merchant grid) ---
	priceEditor         widget.Editor
	qtyEditor           widget.Editor
	levelFormEditor     widget.Editor
	formRowIDs          []int64 // rows the editors are seeded for (nil = none)
	formPrice           int64   // numeric value last seeded into priceEditor (meaningless if formPriceMixed)
	formPriceMixed      bool    // selected rows disagree on price -- editor shows blank/"Mixed"
	formQty             int64
	formQtyMixed        bool
	formLevel           int64
	formLevelMixed      bool
	formItemList        widget.List // bounded-height scroller for the multi-select name list (formMultiItemList)
	changeItemBtn       widget.Clickable
	cancelPickBtn       widget.Clickable
	undoSwapBtn         widget.Clickable
	gateBtn             widget.Clickable
	unlockAllFormBtn    widget.Clickable // multi-select bar's "Unlock all" -- draftUnlockAll
	applyDraftBtn       widget.Clickable // commits the draft into PendingEdits/PendingFlagEdits, then closes the bar
	closeDraftBtn       widget.Clickable // the "X": discards the draft and closes the bar
	rowEditPanelBlocker widget.Clickable // absorbs blank presses inside the bright edit window

	// --- pending-edits header controls + modal ---
	removeBtns            map[int64]*widget.Clickable
	removeAllBtns         map[string]*widget.Clickable // per merchant, keyed by group name (see groupPendingByMerchant)
	removeFlagBtns        map[int]*widget.Clickable    // per character index (see layoutPendingFlagList)
	saveBtn               widget.Clickable
	pendingBtn            widget.Clickable
	pendingOpen           bool // modal visible (UI-goroutine-owned)
	pendingList           widget.List
	pendingCloseBtn       widget.Clickable
	pendingScrim          widget.Clickable // click-to-close area behind the modal panel
	pendingPanelBlocker   widget.Clickable // absorbs blank presses inside the bright pending window
	pendingMerchant       widgets.Combo    // merchant filter, "" = all
	pendingMerchantFilter string           // source of truth for the combo's selection across frames (see layoutPendingBody)

	// --- unexpected-error modal ---
	modal widgets.Modal

	// --- views / settings ---
	view             int // viewEditor, viewSettings or viewCharacters
	tabCharsBtn      widget.Clickable
	tabEditorBtn     widget.Clickable
	tabSettingsBtn   widget.Clickable
	Settings         Settings
	ThemeCombo       widgets.Combo
	FontCombo        widgets.Combo
	autoFreeChk      widget.Bool
	autoUnlimitedChk widget.Bool
	showCountsChk    widget.Bool
	sellValueChk     widget.Bool
	riskyItemsChk    widget.Bool

	openEditorAfterDropChk widget.Bool
	gridCellSlider         widget.Float
	catalogCellSlider      widget.Float
	th                     *material.Theme

	// --- Reset to Vanilla (see staging.go's ResetToVanilla) ---
	resetVanillaBtn         widget.Clickable
	resetVanillaConfirmOpen bool
	resetVanillaModal       widgets.Modal
	resetVanillaDiffCount   int  // computed when the button is clicked, shown in the confirm body
	resetVanillaAvailable   bool // cached by refreshResetVanillaAvailability; gates the button itself

	// --- characters view top bar: open (see character_panel.go) ---
	pathEditor   widget.Editor // typed save path, er_pvp_mod-style
	loadTypedBtn widget.Clickable

	// --- characters / merchant-unlock view (see character_panel.go) ---
	charDataPath string // save path CharList/charSaveData were read for
	charSaveData []byte // raw bytes of charDataPath (source for charunlock)
	CharList     []charunlock.Character
	charBtns     map[int]*widget.Clickable
	SelectedChar int // -1 = none picked (index into the .sl2 character slots)
	charColList  widget.List

	gatedCachePath        string         // save path merchantGated* was computed for
	gatedCacheChar        int            // SelectedChar merchantGated* was computed for (-2 = never)
	merchantGatedTotal    map[string]int // merchant name -> total gated-row count, > 0 only
	merchantGatedUnlocked map[string]int // merchant name -> currently-unlocked subset of the above
	// charFlagState covers every gated row of every merchant for the selected
	// character (not just the currently open merchant's FlagRows/FlagState).
	// Enia's rows are included strictly for their read-only lock display;
	// charFlagMerchant deliberately excludes her so she cannot enter the
	// Characters unlock UI or any bulk-edit route. These maps are byproducts
	// of the same per-merchant loop that computes merchantGatedTotal/Unlocked,
	// cache-invalidated together. They feed effectiveRowUnlocked, which binds
	// the Shop Editor's purple-lock display and the merchant list's live
	// recolor to the selected character's actual (committed + staged) state.
	charFlagState    map[int64]bool   // rowID -> on-disk unlocked
	charFlagMerchant map[int64]string // rowID -> canonical merchant name
	charFlagFlag     map[int64]int64  // rowID -> UnlockFlag (collapses the
	// live count overlay per flag-group instead of per-row, see
	// displayMerchantUnlocked)
	readOnlyGateRows map[int64]bool  // Enia boss-progress flags: display only, never staged
	tmhBellCommitted map[uint32]bool // Twin Maiden Husks bell-bearing flagID
	// -> on-disk acquired; her "NPC" buttons, folded into her gated count
	merchantBtns    map[string]*widget.Clickable
	merchantColList widget.List

	UnlockMerchant        string         // merchant whose flags are shown ("" = none)
	FlagRows              []*catalog.Row // ALL gated rows for UnlockMerchant + SelectedChar
	FlagState             map[int64]bool // rowID -> currently unlocked (on disk), for FlagRows
	flagChecks            map[int64]*widget.Bool
	unlockAllBtn          widget.Clickable
	unlockAllMerchantsBtn widget.Clickable
	merchantUnlockUndo    *bulkUnlockUndo
	allMerchantsUndo      *bulkUnlockUndo
	flagColList           widget.List
	// tmhColList is Twin Maiden Husks' single flags-column scroll list --
	// one shared scrollbar for her 3-section grid (see layoutTMHFlagsGrid)
	// -- separate from flagColList (used by every other merchant's flat
	// list) so switching merchants doesn't fight over one scroll-position
	// widget.
	tmhColList widget.List
	// scrollColList is the same idea for Brother Corhyn/Miriel/Sorceress
	// Sellen's 2-section scroll-unlock grid (layoutScrollFlagsGrid) --
	// its own scroll-position widget, not shared with tmhColList/flagColList.
	scrollColList widget.List

	// PendingFlagEdits stages character-flag toggles until the shared
	// Save button (one shared footer/button for every view, see
	// startCombinedSave) commits them (mirrors PendingEdits' staging
	// model for item edits) -- charIndex -> rowID -> target released
	// value. A row absent here means "no staged change" (its checkbox
	// matches FlagState); staging back to the committed value removes the
	// entry, same rule as item-edit staging.
	PendingFlagEdits map[int]map[int64]bool

	// bellBearingState/bellBearingChecks/PendingBellBearingEdits are
	// FlagState/flagChecks/PendingFlagEdits's counterpart for Twin Maiden
	// Husks' bell-bearing acquisition toggles (app/charunlock.BellBearing)
	// -- a flag with no backing catalog.Row, so kept as its own set of
	// maps (charIndex -> bell-bearing flagID, not rowID) rather than
	// merged into the row-based ones above. Only ever populated when
	// UnlockMerchant == twinMaidenHusksMerchantName; see
	// docs/CHAR_UNLOCK.md's dated entry.
	bellBearingState        map[uint32]bool // flagID -> currently acquired (on disk), for BellBearingsForUI
	bellBearingChecks       map[uint32]*widget.Bool
	PendingBellBearingEdits map[int]map[uint32]bool
}

// NewState builds the editor state around a ready catalog.
func NewState(cat *catalog.Catalog) *State {
	s := &State{
		Catalog:                 cat,
		Icons:                   NewIconCache(),
		itemCells:               make(map[int64]*cellState),
		rowCells:                make(map[int64]*cellState),
		PendingEdits:            make(map[int64]*RowEdit),
		draftEdits:              make(map[int64]*RowEdit),
		formItemCells:           make(map[int64]*widget.Clickable),
		removeBtns:              make(map[int64]*widget.Clickable),
		removeAllBtns:           make(map[string]*widget.Clickable),
		removeFlagBtns:          make(map[int]*widget.Clickable),
		dropHoverRow:            noRow,
		dragFromRow:             noRow,
		SelectedChar:            -1,
		gatedCacheChar:          -2,
		charBtns:                make(map[int]*widget.Clickable),
		merchantBtns:            make(map[string]*widget.Clickable),
		flagChecks:              make(map[int64]*widget.Bool),
		PendingFlagEdits:        make(map[int]map[int64]bool),
		bellBearingChecks:       make(map[uint32]*widget.Bool),
		PendingBellBearingEdits: make(map[int]map[uint32]bool),
		view:                    viewCharacters,
	}
	s.Search.SingleLine = true
	s.Search.Submit = true

	// Number inputs: single-line, commit on Enter, live-clamped into their
	// valid range as the user types (see liveClampedInt). Price and level
	// fields never go negative, so "-" is excluded from their filter
	// entirely; qty keeps it since -1 (unlimited) is a valid value.
	for _, ed := range []*widget.Editor{&s.priceEditor, &s.levelFormEditor, &s.levelEditor} {
		ed.SingleLine = true
		ed.Submit = true
		ed.Filter = "0123456789"
	}
	s.qtyEditor.SingleLine = true
	s.qtyEditor.Submit = true
	s.qtyEditor.Filter = "-0123456789"
	s.levelEditor.SetText("0")
	s.pathEditor.SingleLine = true
	s.pathEditor.Submit = true

	// Item categories come from items.json, available regardless of any
	// loaded save.
	s.CategoryCombo.SetOptionsWithLabels(orderedCategoryOptions(cat.ListCategories()))
	s.SubCatCombo.SetOptionsWithLabels([]string{""}, []string{"All Subcategories"})
	s.MerchantCategoryCombo.SetOptionsWithLabels([]string{""}, []string{"All Categories"})

	// Persisted settings + theme.
	s.Settings = LoadSettings()
	s.ThemeCombo.SetOptions([]string{"Dark", "Light", "Elden Ring"})
	switch s.Settings.Theme {
	case "light":
		s.ThemeCombo.SetValue("Light")
	case "elden":
		s.ThemeCombo.SetValue("Elden Ring")
	}
	fontValues := make([]string, len(fontOptions))
	fontLabels := make([]string, len(fontOptions))
	for i, o := range fontOptions {
		fontValues[i], fontLabels[i] = o.value, o.label
	}
	s.FontCombo.SetOptionsWithLabels(fontValues, fontLabels)
	s.FontCombo.SetValue(s.Settings.Font)
	s.autoFreeChk.Value = s.Settings.AutoFreeItems
	s.autoUnlimitedChk.Value = s.Settings.AutoUnlimitedItems
	s.showCountsChk.Value = s.Settings.ShowMerchantRowCounts
	s.sellValueChk.Value = s.Settings.ShowSellValueChanges
	s.riskyItemsChk.Value = s.Settings.ShowRiskyItems
	s.openEditorAfterDropChk.Value = s.Settings.OpenEditorAfterDrop
	s.gridCellSlider.Value = float32(s.merchantCellSize()-cellSizeSnapMin) / float32(cellSizeSnapMax-cellSizeSnapMin)
	s.catalogCellSlider.Value = float32(s.catalogCellSize()-cellSizeSnapMin) / float32(cellSizeSnapMax-cellSizeSnapMin)
	s.applyTheme()
	return s
}

// SetWindow wires the app window so background work (save loads, applies,
// icon decodes) can request a redraw.
func (s *State) SetWindow(w *app.Window) {
	s.win = w
	s.Icons.Start(s.invalidate)
}

func (s *State) invalidate() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// --- load lifecycle (thread-safe snapshot accessors) ---

// Busy reports whether a save load or an edit apply is in progress.
func (s *State) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

// clearBusy resets busy after a flow that set it (e.g. startCombinedSave)
// aborts before reaching the worker that would otherwise clear it itself
// (combinedApplyWorker, loadWorker) -- dialog canceled/failed, or a
// pre-dialog validation error.
func (s *State) clearBusy() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// BusyMsg describes the in-flight operation ("" when idle).
func (s *State) BusyMsg() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy {
		return ""
	}
	return s.busyMsg
}

// ApplyErr returns the pending-dropdown inline error ("" = none).
func (s *State) ApplyErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyErr
}

// ClearApplyErr dismisses the pending-dropdown inline error.
func (s *State) ClearApplyErr() {
	s.mu.Lock()
	s.applyErr = ""
	s.mu.Unlock()
}

// LoadedName returns the base filename of the loaded save ("" = none).
func (s *State) LoadedName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadedName
}

// InlineErr returns the current save-switcher inline error text.
func (s *State) InlineErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inlineErr
}

// StartLoadSave loads a save off the UI goroutine. The heavy
// decrypt/decompress and the first row decode both run in the worker, so the
// frame loop never blocks; it requests a redraw when done. A second call while
// a load is in flight is ignored.
func (s *State) StartLoadSave(path string) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.busyMsg = "Loading save..."
	s.inlineErr = ""
	s.mu.Unlock()

	go s.loadWorker(path)
}

func (s *State) loadWorker(path string) {
	err := s.Catalog.LoadSave(path)
	if err == nil {
		// Warm the row decode here so the expensive first ShopRows() call
		// happens off the UI goroutine.
		_, err = s.Catalog.ListMerchants()
	}

	s.mu.Lock()
	s.busy = false
	if err != nil {
		s.inlineErr = friendlyLoadError(path, err)
	} else {
		s.inlineErr = ""
		s.loadedName = filepath.Base(path)
		// UI-goroutine-owned fields must not be touched from this worker;
		// flag the reset for the frame loop to apply instead.
		s.resetPending = true
	}
	s.mu.Unlock()
	s.invalidate()
}

// friendlyLoadError turns a missing-file load error into a plain "File not
// found" message instead of Go's raw *fs.PathError text; any other error
// (wrong format, corrupt data, etc.) passes through unchanged.
func friendlyLoadError(path string, err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "File not found: " + path
	}
	return err.Error()
}

// consumeReset applies the after-load UI reset on the UI goroutine (called at
// the top of every frame): a fresh save invalidates whatever was selected or
// staged against the old one. An apply-success instead clears only the staging
// and re-reads the current merchant's rows — the save's content changed but the
// user's place in the UI (merchant, selected row) is still meaningful.
func (s *State) consumeReset() {
	s.mu.Lock()
	pending := s.resetPending
	applied := s.applyDone
	s.resetPending = false
	s.applyDone = false
	s.mu.Unlock()

	if pending {
		s.clearRowSelection()
		s.lastMerchant = ""
		s.merchantsPath = ""
		s.clearSelection()
		s.resetStagingCommon()
		s.setFooterStatus("Loaded " + s.LoadedName())
		return
	}
	if applied {
		// The catalog's save path moved to the Save-As target; adopt it without
		// repopulating the merchant combo (same merchants, updated rows). The
		// apply worker pre-warmed the row decode, so this re-read is cheap.
		s.merchantsPath = s.Catalog.SavePath()
		if s.lastMerchant != "" {
			if rows, err := s.Catalog.MerchantRows(s.lastMerchant); err == nil {
				s.MerchantRows = rows
			}
		}
		s.resetStagingCommon()
		s.setFooterStatus("Saved " + s.LoadedName())
	}
}

// resetStagingCommon clears the shop-editor staging state shared by both
// consumeReset outcomes above (a fresh load invalidates ALL of it; a
// completed apply clears only the staging, since MerchantRows/lastMerchant
// are handled separately by each caller) plus the Characters view's own
// staged state and the Reset-to-Vanilla button's availability cache --
// every consumeReset outcome ends here.
func (s *State) resetStagingCommon() {
	s.PendingEdits = make(map[int64]*RowEdit)
	s.PickingForRows = nil
	s.footerStatus = ""
	s.formRowIDs = nil // reseed the form editors from the rewritten values
	s.pendingOpen = false
	s.pendingMerchantFilter = ""
	s.resetCharacterViewState()
	s.refreshResetVanillaAvailability()
}

// resetCharacterViewState clears the Characters view's state after any
// successful save/load. Staged flag edits must read as gone immediately
// -- the shared "Pending (N)" footer is visible from every view, so
// it can't wait for ensureCharList to notice a path change, which only
// happens while the Characters view itself is being laid out. Clearing
// charDataPath also makes ensureCharList redo its own full reset
// (CharList, SelectedChar, etc.) the next time that view renders, so a
// combined save started from the Shop Editor doesn't leave the Characters
// view pointed at stale, pre-save character data.
func (s *State) resetCharacterViewState() {
	s.charDataPath = ""
	s.PendingFlagEdits = make(map[int]map[int64]bool)
	s.PendingBellBearingEdits = make(map[int]map[uint32]bool)
	s.clearBulkUnlockUndo()
}

// --- Reset to Vanilla (see staging.go's ResetToVanilla; settings.go's
// button, window.go's confirm-dialog overlay) ---

// refreshResetVanillaAvailability recomputes whether the currently loaded
// save has ANY row differing from vanilla -- gates the Settings button
// itself (disabled when nothing differs from vanilla). Called from
// consumeReset right after a fresh load or a completed save/apply, both
// points where the catalog's on-disk-backed row values can actually change;
// cheap enough (a few hundred µs over ~1300 rows) to just eagerly recompute
// rather than trying to track dirtiness incrementally.
func (s *State) refreshResetVanillaAvailability() {
	if !s.Catalog.Loaded() {
		s.resetVanillaAvailable = false
		return
	}
	diffs, err := s.Catalog.VanillaDiffs()
	s.resetVanillaAvailable = err == nil && len(diffs) > 0
}

// openResetVanillaConfirm computes how many rows currently differ from
// vanilla (for the confirm dialog's body text) and opens the confirm
// dialog. A no-op with no save loaded -- the Settings button is disabled
// in that case, but this is the authoritative guard.
func (s *State) openResetVanillaConfirm() {
	if !s.Catalog.Loaded() {
		return
	}
	diffs, err := s.Catalog.VanillaDiffs()
	if err != nil {
		s.reportError("compute Reset to Vanilla diff", err)
		return
	}
	s.resetVanillaDiffCount = len(diffs)
	s.resetVanillaConfirmOpen = true
}

// doResetToVanilla closes the confirm dialog and stages the reset (see
// ResetToVanilla's own doc for exactly what that does and doesn't touch).
// Recomputing the diff here (rather than reusing openResetVanillaConfirm's
// count) keeps the two calls independent and correct even in the
// (currently impossible, single-goroutine-UI) case the catalog changed
// between the two clicks -- the cost is negligible either way.
func (s *State) doResetToVanilla() {
	s.resetVanillaConfirmOpen = false
	if _, err := s.ResetToVanilla(); err != nil {
		s.reportError("Reset to Vanilla", err)
	}
}

// --- save-as / apply flow ---

// combinedPendingCount is the shared footer's "Pending (N)" count: staged
// item edits plus every character's staged flag edits, of either kind (one
// save button, reachable from any view, saves both at once).
func (s *State) combinedPendingCount() int {
	return s.PendingCount() + s.totalPendingFlagCount() + s.totalPendingBellBearingCount()
}

// startCombinedSave resolves whatever's staged (item edits, character-flag
// edits, or both) up front on the UI goroutine, then shows ONE native
// Save-As dialog and hands the result to combinedApplyWorker -- one save
// button that always works, regardless of which kind of edit (or both) is
// staged.
func (s *State) startCombinedSave() {
	if s.Busy() || s.combinedPendingCount() == 0 {
		return
	}
	// Mark busy for the WHOLE flow, not just the write itself. The
	// itemEdits/equipParamEdits/flagTargets snapshot built below is a
	// point-in-time copy of PendingEdits/PendingFlagEdits/
	// PendingBellBearingEdits -- if busy only became true once the native
	// Save-As dialog returned a path (as before), every Busy()-gated
	// staging control (merchant grid, flag checkboxes, ...) stayed
	// interactive for the whole time the dialog sat open, so edits staged
	// during that window never entered the snapshot and were silently
	// discarded when combinedApplyWorker's write completed and
	// consumeReset cleared PendingEdits. Every early return below must
	// call s.clearBusy() (combinedApplyWorker clears it itself on
	// completion, success or failure).
	s.mu.Lock()
	s.busy = true
	s.busyMsg = "Preparing save..."
	s.applyErr = ""
	s.mu.Unlock()

	itemEdits := s.BuildEdits() // nil when there are no staged item edits

	equipParamEdits, err := s.BuildEquipParamEdits()
	if err != nil {
		s.clearBusy()
		s.reportError("resolve sellValue guards", err)
		return
	}

	var flagTargets map[int][]charunlock.FlagTarget
	if s.totalPendingFlagCount() > 0 {
		rowsByID, err := s.Catalog.RowsByID()
		if err != nil {
			s.clearBusy()
			s.reportError("resolve staged flag rows", err)
			return
		}
		flagTargets = make(map[int][]charunlock.FlagTarget, len(s.PendingFlagEdits))
		for charIndex, rowEdits := range s.PendingFlagEdits {
			batch := make([]charunlock.FlagTarget, 0, len(rowEdits))
			for rowID, released := range rowEdits {
				if r, ok := rowsByID[rowID]; ok {
					batch = append(batch, charunlock.FlagTarget{FlagID: r.UnlockFlag, Released: released, Label: fmt.Sprintf("row %d", r.RowID)})
				}
			}
			if len(batch) > 0 {
				flagTargets[charIndex] = batch
			}
		}
	}
	// Bell-bearing acquisition toggles (Twin Maiden Husks, see
	// docs/CHAR_UNLOCK.md) fold into the SAME flagTargets map/write stage
	// as row-gate toggles above -- SetReleaseBatch doesn't care about a
	// target's origin, so no separate write pipeline stage is needed.
	if s.totalPendingBellBearingCount() > 0 {
		if flagTargets == nil {
			flagTargets = make(map[int][]charunlock.FlagTarget, len(s.PendingBellBearingEdits))
		}
		for charIndex, edits := range s.PendingBellBearingEdits {
			for flagID, released := range edits {
				flagTargets[charIndex] = append(flagTargets[charIndex], charunlock.FlagTarget{
					FlagID:   int64(flagID),
					Released: released,
					Label:    fmt.Sprintf("bell bearing flag %d", flagID),
				})
			}
		}
	}

	suggested := s.SuggestedOutPath()
	if !s.setDialogOpen(true) {
		s.clearBusy()
		return
	}
	go func() {
		defer s.setDialogOpen(false)
		outPath, err := zenity.SelectFileSave(
			zenity.Title("Save edited file as"),
			zenity.Filename(suggested),
			zenity.ConfirmOverwrite(),
			zenity.FileFilters{
				{Name: "Save files", Patterns: []string{"*.dat"}, CaseFold: true},
				{Name: "All files", Patterns: []string{"*"}},
			},
		)
		if errors.Is(err, zenity.ErrCanceled) || outPath == "" {
			s.clearBusy()
			s.invalidate()
			return
		}
		if err != nil {
			s.clearBusy()
			s.reportError("save dialog", err)
			return
		}
		s.mu.Lock()
		s.busyMsg = "Saving..."
		s.mu.Unlock()
		s.combinedApplyWorker(itemEdits, flagTargets, equipParamEdits, outPath)
	}()
}

// combinedApplyWorker writes itemEdits (ShopLineupParam, via
// Catalog.ApplyEdits/app/shopwrite), flagTargets (character-slot event
// flags, via app/charunlock), and equipParamEdits (an item's own
// EquipParam* sellValue -- see BuildEquipParamEdits's doc comment for why,
// via shopwrite.ApplyWithSchema) to outPath, in whichever combination is
// non-empty. Each kind is an independently optional PIPELINE STAGE, run in
// a fixed order (flags -> sellValue, one stage per distinct EquipParam*
// table touched -> item edits) chained through numbered ".tmpN" files
// (never overwriting the currently-loaded input, same safety rule every
// individual write already enforces on its own): stage N's output becomes
// stage N+1's input, and only the LAST stage that actually runs writes
// directly to outPath. This generalizes what was a fixed 2-stage
// (flags-then-items) switch before sellValue edits existed; every
// individual write function is untouched.
//
// An EditError from ApplyEdits stays a routine inline dropdown error
// (nothing was written); any other stage's failure is unexpected/internal
// (e.g. a round-trip self-check) and goes through the same reportError
// modal path applyWorker's item-edit-only case already used.
func (s *State) combinedApplyWorker(itemEdits []shopwrite.Edit, flagTargets map[int][]charunlock.FlagTarget, equipParamEdits map[string][]shopwrite.Edit, outPath string) {
	inPath := s.Catalog.SavePath()

	type stageFunc func(from, to string) error
	var stages []stageFunc
	var buildErr error

	if len(flagTargets) > 0 {
		stages = append(stages, func(from, to string) error {
			_, err := charunlock.ApplyBatchToFile(from, to, flagTargets)
			return err
		})
	}
	for equipType := int64(0); equipType <= 4; equipType++ {
		entryName, ok := shopwrite.EquipParamEntryName(equipType)
		if !ok {
			continue
		}
		edits := equipParamEdits[entryName]
		if len(edits) == 0 {
			continue
		}
		schema, serr := shopwrite.LoadEquipParamSchema(equipType)
		if serr != nil {
			buildErr = serr
			break
		}
		en, ed, sc := entryName, edits, schema // capture per-iteration, not the loop vars
		stages = append(stages, func(from, to string) error {
			_, err := shopwrite.ApplyWithSchema(from, to, en, ed, sc)
			return err
		})
	}
	if len(itemEdits) > 0 {
		stages = append(stages, func(from, to string) error {
			if from != s.Catalog.SavePath() {
				if err := s.Catalog.LoadSave(from); err != nil {
					return err
				}
			}
			_, err := s.Catalog.ApplyEdits(itemEdits, to)
			return err
		})
	}

	err := buildErr
	if err == nil {
		var tmps []string
		cur := inPath
		for i, run := range stages {
			to := outPath
			if i < len(stages)-1 {
				to = fmt.Sprintf("%s.tmp%d", outPath, i)
				tmps = append(tmps, to)
			}
			if err = run(cur, to); err != nil {
				break
			}
			cur = to
		}
		for _, t := range tmps {
			os.Remove(t)
		}
	}
	if err == nil && s.Catalog.SavePath() != outPath {
		// The last stage that ran wasn't ApplyEdits (which already moves
		// the Catalog's notion of "loaded save" forward internally) --
		// move it forward explicitly, same as the flags-only case always
		// had to before sellValue edits existed.
		err = s.Catalog.LoadSave(outPath)
	}
	if err == nil {
		// Warm the re-decode of the freshly written save off the UI goroutine
		// (same reason as loadWorker).
		_, _ = s.Catalog.ListMerchants()
	}

	var editErr *catalog.EditError
	s.mu.Lock()
	s.busy = false
	switch {
	case err == nil:
		s.applyErr = ""
		s.loadedName = filepath.Base(s.Catalog.SavePath())
		s.applyDone = true
	case errors.As(err, &editErr):
		// Routine validation failure — inline text, nothing was written.
		s.applyErr = editErr.Error()
	default:
		s.mu.Unlock()
		s.reportError("save file", err)
		s.invalidate()
		return
	}
	s.mu.Unlock()
	s.invalidate()
}

// --- native open dialog ---

// setDialogOpen toggles the dialog guard, returning false if a dialog is
// already open (so the caller should not open a second one).
func (s *State) setDialogOpen(open bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if open && s.dialogOpen {
		return false
	}
	s.dialogOpen = open
	return true
}

// setInlineErr records a save-switcher error and requests a redraw.
func (s *State) setInlineErr(msg string) {
	s.mu.Lock()
	s.inlineErr = msg
	s.mu.Unlock()
	s.invalidate()
}

// --- shared selection-state primitives (used by both the merchant row and
// catalog item multi-select schemes below, which mirror each other's
// plain-replace/ctrl-toggle/shift-range click semantics but otherwise track
// separate fields, so only the pure id-slice logic -- not the anchor/
// invalidate/scheme-specific bookkeeping -- is actually shareable) ---

// selContains reports whether id is in sel.
func selContains(sel []int64, id int64) bool {
	for _, sid := range sel {
		if sid == id {
			return true
		}
	}
	return false
}

// selToggle adds id at the end (preserving order) or removes it.
func selToggle(sel []int64, id int64) []int64 {
	for i, sid := range sel {
		if sid == id {
			return append(sel[:i], sel[i+1:]...)
		}
	}
	return append(sel, id)
}

// --- merchant row multi-select (mirrors catalog multi-select below: same
// plain-replace / ctrl-toggle / shift-range scheme, see handleRowClick in
// merchant_panel.go) ---

// isRowSelected reports whether a row id is in the current selection.
func (s *State) isRowSelected(id int64) bool {
	return selContains(s.SelectedRowIDs, id)
}

// clearRowSelection empties the row selection and forgets the shift anchor.
func (s *State) clearRowSelection() {
	// While replacement picking is active, ordinary row-selection cleanup must
	// not close the picker. Its two deliberate exits are CancelPicking (the
	// Catalog button or a press outside the catalog), which also closes the
	// editor, and completing the queued replacement(s).
	if s.Picking() {
		return
	}
	s.SelectedRowIDs = nil
	s.rowSelAnchorSet = false
	s.PickingForRows = nil
	s.showRowEditor = false // no selection => editor closed; next selection reopens via the button
	// Do not leave an invisible draft alive until the next merchant-grid
	// layout. Closing or cancelling is an immediate rollback of everything
	// not yet applied to PendingEdits.
	s.draftRowIDs = nil
	s.draftEdits = make(map[int64]*RowEdit)
	s.draftGateEdits = nil
	s.invalidate()
}

// selectRowPlain replaces the selection with a single row and pins the
// anchor. Re-clicking the sole selected row deselects it.
func (s *State) selectRowPlain(id int64) {
	if len(s.SelectedRowIDs) == 1 && s.SelectedRowIDs[0] == id {
		s.clearRowSelection()
		return
	}
	s.SelectedRowIDs = []int64{id}
	s.rowSelAnchor, s.rowSelAnchorSet = id, true
	s.PickingForRows = nil
	s.invalidate()
}

// toggleSelectRow adds a row at the end (preserving order) or removes it,
// and moves the anchor to it.
func (s *State) toggleSelectRow(id int64) {
	s.SelectedRowIDs = selToggle(s.SelectedRowIDs, id)
	s.rowSelAnchor, s.rowSelAnchorSet = id, true
	s.PickingForRows = nil
	s.invalidate()
}

// selectRowRange replaces the selection with the contiguous run between the
// anchor and the clicked row in the current visible order, leaving the
// anchor unchanged. Falls back to a plain select if the anchor is
// missing/no longer visible.
func (s *State) selectRowRange(rows []*catalog.Row, clickedIdx int) {
	if !s.rowSelAnchorSet {
		s.selectRowPlain(rows[clickedIdx].RowID)
		return
	}
	anchorIdx := -1
	for i, r := range rows {
		if r.RowID == s.rowSelAnchor {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		s.selectRowPlain(rows[clickedIdx].RowID)
		return
	}
	lo, hi := anchorIdx, clickedIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	sel := make([]int64, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		sel = append(sel, rows[i].RowID)
	}
	s.SelectedRowIDs = sel
	// anchor unchanged
	s.PickingForRows = nil
	s.invalidate()
}

// --- catalog multi-select ---

// isSelected reports whether an item id is in the current selection.
func (s *State) isSelected(id int64) bool {
	return selContains(s.SelectedItems, id)
}

// clearSelection empties the selection and forgets the shift anchor.
func (s *State) clearSelection() {
	s.SelectedItems = nil
	s.selAnchorSet = false
	s.invalidate()
}

// selectPlain replaces the selection with a single item and pins the anchor.
// Re-clicking the sole selected item deselects it (same toggle the merchant
// grid has).
func (s *State) selectPlain(id int64) {
	if len(s.SelectedItems) == 1 && s.SelectedItems[0] == id {
		s.clearSelection()
		return
	}
	s.SelectedItems = []int64{id}
	s.selAnchor, s.selAnchorSet = id, true
	s.invalidate()
}

// dragIDCount is how many items a drag starting on id would carry.
func (s *State) dragIDCount(id int64) int {
	if s.isSelected(id) {
		return len(s.SelectedItems)
	}
	return 1
}

// toggleSelect adds an item at the end (preserving order) or removes it, and
// moves the anchor to it.
func (s *State) toggleSelect(id int64) {
	s.SelectedItems = selToggle(s.SelectedItems, id)
	s.selAnchor, s.selAnchorSet = id, true
	// invalidate() unconditionally (the pre-dedup remove branch didn't call
	// it, unlike its toggleSelectRow twin and this function's own add
	// branch -- an inconsistency, not a deliberate difference: every other
	// selection mutator here redraws immediately).
	s.invalidate()
}

// selectRange replaces the selection with the contiguous run between the anchor
// and clicked item in the current visible (filtered) order, leaving the anchor
// unchanged. Falls back to a plain select if the anchor is missing/invisible.
func (s *State) selectRange(items []*catalog.Item, clickedIdx int) {
	if !s.selAnchorSet {
		s.selectPlain(items[clickedIdx].ID)
		return
	}
	anchorIdx := -1
	for i, it := range items {
		if it.ID == s.selAnchor {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		s.selectPlain(items[clickedIdx].ID)
		return
	}
	lo, hi := anchorIdx, clickedIdx
	if lo > hi {
		lo, hi = hi, lo
	}
	sel := make([]int64, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		if items[i].EquipType == nil {
			continue // unsellable items never enter the selection
		}
		sel = append(sel, items[i].ID)
	}
	s.SelectedItems = sel
	// anchor unchanged
	s.invalidate()
}

// itemByID resolves an items.json id to its Item, hidden items INCLUDED --
// it delegates to catalog.ItemByID rather than indexing ListItems, which
// omits hidden entries. A hidden item is only hidden from the browsable
// catalog GRID; a real shop row can still reference one (e.g. Twin Maiden
// Husks' Flask of Wondrous Physick), and everything reached from such a row
// -- the item-info popup above all -- must still resolve it. Indexing
// ListItems here silently returned nil for exactly those items, so
// right-clicking them opened nothing (user-reported 2026-08-03).
func (s *State) itemByID(id int64) *catalog.Item {
	return s.Catalog.ItemByID(id)
}

// dragPayload builds the comma-separated id payload for a drag beginning on
// item id: the whole selection when the dragged item is part of it, else just
// that item. The selection is never mutated here.
func (s *State) dragPayload(id int64) string {
	ids := []int64{id}
	if s.isSelected(id) {
		ids = s.SelectedItems
	}
	parts := make([]string, len(ids))
	for i, v := range ids {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ",")
}

// rowDragPayload builds a merchant-cell drag's ordered row-id payload: the
// whole current selection (in selection order) when the dragged row is part
// of it -- so a group drag positionally swaps first-with-first, etc. -- else
// just the dragged row itself. Mirrors dragPayload for the catalog grid.
func (s *State) rowDragPayload(id int64) string {
	ids := []int64{id}
	if s.isRowSelected(id) {
		ids = s.SelectedRowIDs
	}
	parts := make([]string, len(ids))
	for i, v := range ids {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ",")
}

// rowDragIDCount is how many rows a merchant-cell drag starting on id carries
// (the selection size when id is selected, else 1) -- drives the drag ghost's
// count badge and the drop-span highlight width.
func (s *State) rowDragIDCount(id int64) int {
	if s.isRowSelected(id) {
		return len(s.SelectedRowIDs)
	}
	return 1
}

// parseItemPayload decodes a comma-separated id payload back to ids, skipping
// any unparsable token.
func parseItemPayload(payload string) []int64 {
	var out []int64
	for _, tok := range strings.Split(payload, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if v, err := strconv.ParseInt(tok, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// --- item-picking mode ---

// StartPicking enters item-picking mode for a snapshot of rowIDs (the
// selection at the moment "Change item" was clicked) -- a catalog pick
// fans out to all of them.
func (s *State) StartPicking(rowIDs []int64) {
	s.PickingForRows = append([]int64(nil), rowIDs...)
	s.setFooterStatus(s.pickingStatusMessage())
}

// CancelPicking abandons the replacement and closes the whole edit flow. A
// cancelled replacement must not reveal the row-edit window again: its draft
// has not been applied, so leaving it open would invite the user to continue
// an action they explicitly backed out of.
func (s *State) CancelPicking() {
	s.PickingForRows = nil
	s.clearRowSelection()
	s.setFooterStatus("Item replacement cancelled")
}

// Picking reports whether an item is currently being picked for one or more
// rows.
func (s *State) Picking() bool { return len(s.PickingForRows) > 0 }

func (s *State) pickingStatusMessage() string {
	n := len(s.PickingForRows)
	if n == 1 {
		return "Choose a catalog item to replace the selected stock item"
	}
	return fmt.Sprintf("Choose %d catalog items to replace the selected stock items", n)
}

// consumeNextPickingRow pops and returns the FRONT row still queued for a
// picked item (nil once the queue is empty), skipping any id no longer in
// the current merchant's row cache. Each catalog click consumes exactly one
// queued row -- picking N rows requires N picks, one pick never fans out to
// all of them; Picking() flips false once the queue empties, ending the
// dim-overlay automatically. Clicking the same item N times reproduces the
// "same item everywhere" result deliberately, one click at a time.
func (s *State) consumeNextPickingRow() *catalog.Row {
	byID := make(map[int64]*catalog.Row, len(s.MerchantRows))
	for _, r := range s.MerchantRows {
		byID[r.RowID] = r
	}
	for len(s.PickingForRows) > 0 {
		id := s.PickingForRows[0]
		s.PickingForRows = s.PickingForRows[1:]
		if r := byID[id]; r != nil {
			if s.Picking() {
				s.setFooterStatus(s.pickingStatusMessage())
			} else {
				s.setFooterStatus("Item replacement staged — review it in Pending before saving")
			}
			return r
		}
	}
	return nil
}

// --- row-edit bar draft ---

// ensureDraft reconciles the draft against the current selection every
// frame layoutMerchantPanel runs (called unconditionally, whether or not
// the bar is actually shown): the moment SelectedRowIDs no longer matches
// draftRowIDs -- a different selection, or the bar closed to none -- the
// old draft (applied or not) is discarded and a fresh one is seeded from
// PendingEdits for whatever's selected now. This single check is what
// makes "switch selection" and "close then reopen the same rows" both
// correctly drop an unapplied draft, with no separate close-handling
// needed anywhere selection-mutating code runs.
func (s *State) ensureDraft() {
	ids := append([]int64(nil), s.SelectedRowIDs...)
	if int64SliceEqual(s.draftRowIDs, ids) {
		return
	}
	s.draftRowIDs = ids
	s.reseedDraft()
}

// reseedDraft rebuilds draftEdits as a deep copy of PendingEdits for every
// row in draftRowIDs, and clears the gate draft. Deep copies (not shared
// pointers) so mutating the draft can never corrupt an already-committed
// PendingEdits entry -- applyDraft hands draft entries over to PendingEdits
// directly, then calls this again to start the next round from a clean
// copy of what was just committed.
func (s *State) reseedDraft() {
	s.draftEdits = make(map[int64]*RowEdit, len(s.draftRowIDs))
	for _, id := range s.draftRowIDs {
		if e := s.PendingEdits[id]; e != nil {
			s.draftEdits[id] = copyRowEdit(e)
		}
	}
	s.draftGateEdits = nil
}

// copyRowEdit deep-copies a RowEdit so the draft never aliases a committed
// PendingEdits entry's maps/slices.
func copyRowEdit(e *RowEdit) *RowEdit {
	cp := *e
	cp.FieldChanges = make(map[string]FieldChange, len(e.FieldChanges))
	for k, v := range e.FieldChanges {
		cp.FieldChanges[k] = v
	}
	if e.ItemChange != nil {
		ic := *e.ItemChange
		ic.ClearOverrides = append([]string(nil), e.ItemChange.ClearOverrides...)
		cp.ItemChange = &ic
	}
	return &cp
}

// inDraftSession reports whether rowID belongs to the currently open
// row-edit bar's selection (so its DRAFT, not just PendingEdits, is the
// right thing to show -- see effectiveRowEdit).
func (s *State) inDraftSession(rowID int64) bool {
	for _, id := range s.draftRowIDs {
		if id == rowID {
			return true
		}
	}
	return false
}

// effectiveRowEdit resolves what should currently be DISPLAYED for a row
// anywhere outside the row-edit bar itself (the merchant grid's icon/
// border/tooltip): the in-progress draft when the row is part of the open
// bar session (a live preview of changes not yet applied -- and reverted
// if the bar closes without Apply), else the real, already-committed
// PendingEdits entry.
func (s *State) effectiveRowEdit(rowID int64) *RowEdit {
	if s.inDraftSession(rowID) {
		return s.draftEdits[rowID]
	}
	return s.PendingEdits[rowID]
}

// setDraftGate stages a gate-unlock draft for row's UnlockFlag (overwriting
// any prior draft for that same flag this session) -- the shared write path
// for both the single-row Unlock/Lock toggle and the multi-select "Unlock
// all" button (row_edit_form.go).
func (s *State) setDraftGate(row *catalog.Row, target bool) {
	if s.draftGateEdits == nil {
		s.draftGateEdits = make(map[int64]draftGateEdit)
	}
	s.draftGateEdits[row.UnlockFlag] = draftGateEdit{row: row, target: target}
}

// applyDraft commits the current draft into the real PendingEdits (item/
// field edits) and PendingFlagEdits (staged gate toggles, if any) --
// row_edit_form.go's Apply button. The caller (row_edit_form.go) closes the
// bar right after via clearRowSelection -- see its doc comment ("the apply
// button if pressed must close the window") -- so the reseed below mostly
// just leaves draftEdits/draftGateEdits in a clean state for that close
// rather than needing to support continued in-session editing.
func (s *State) applyDraft() {
	for _, id := range s.draftRowIDs {
		if e := s.draftEdits[id]; e != nil {
			s.PendingEdits[id] = e
		} else {
			delete(s.PendingEdits, id)
		}
	}
	for _, edit := range s.draftGateEdits {
		s.setRowUnlockForSelectedChar(edit.row, edit.target)
	}
	s.reseedDraft()
	if s.combinedPendingCount() > 0 {
		s.setFooterStatus("Edits staged — review them in Pending before saving")
	} else {
		s.setFooterStatus("")
	}
	s.invalidate()
}

// --- per-cell retained state ---

func (s *State) itemCell(id int64) *cellState {
	c := s.itemCells[id]
	if c == nil {
		c = &cellState{}
		s.itemCells[id] = c
	}
	return c
}

func (s *State) rowCell(id int64) *cellState {
	c := s.rowCells[id]
	if c == nil {
		c = &cellState{}
		s.rowCells[id] = c
	}
	return c
}

// formItemCell returns the retained hover/click state for one icon in the
// row-edit bar's multi-select preview grid (formMultiItemList) -- kept
// separate from rowCell (the main merchant grid's own per-row state) since
// a row shown in both places would otherwise need its widget.Clickable
// registered twice in the same frame.
func (s *State) formItemCell(id int64) *widget.Clickable {
	c := s.formItemCells[id]
	if c == nil {
		c = &widget.Clickable{}
		s.formItemCells[id] = c
	}
	return c
}

// merchantName parses an optional numeric row-count suffix ("Name (N)")
// back to the canonical name. Some real merchant locations include ordinary
// parentheses (for example "Nomadic Merchant - Caelid (Aeonia Swamp)"), so
// only a final, all-numeric parenthesized suffix is a display count.
func merchantName(label string) string {
	i := strings.LastIndex(label, " (")
	if i < 0 || !strings.HasSuffix(label, ")") {
		return label
	}
	if _, err := strconv.Atoi(label[i+2 : len(label)-1]); err == nil {
		return label[:i]
	}
	return label
}
