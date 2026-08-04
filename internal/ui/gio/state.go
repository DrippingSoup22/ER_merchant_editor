package gio

import (
	"sync"

	"gioui.org/app"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/application"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/platform/dialogs"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
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
type FieldChange = application.FieldChange

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
type ItemChange = application.ItemChange

// RowEdit stages one row's pending edits: scalar field changes keyed by raw
// param field name, plus an optional item swap. Mirrors the Python
// pending_edits entry shape ({label, cost_type, field_changes, item_change}).
// Merchant is the canonical merchant name captured at stage time (the row's
// own Merchant field is the raw, sometimes-empty Paramdex name -- see
// enrich.go -- so it's not reusable here); normal mode displays it instead
// of the raw row id.
type RowEdit = application.RowEdit

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
	*application.Session
	Icons   *IconCache
	win     *app.Window
	dialogs dialogs.Service

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
	CategoryCombo   components.Combo
	SubCatCombo     components.Combo
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
	MerchantCombo         components.Combo
	MerchantCategoryCombo components.Combo
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
	pendingMerchant       components.Combo // merchant filter, "" = all
	pendingMerchantFilter string           // source of truth for the combo's selection across frames (see layoutPendingBody)

	// --- unexpected-error modal ---
	modal components.Modal

	// --- views / settings ---
	view             int // viewEditor, viewSettings or viewCharacters
	tabCharsBtn      widget.Clickable
	tabEditorBtn     widget.Clickable
	tabSettingsBtn   widget.Clickable
	Settings         Settings
	ThemeCombo       components.Combo
	FontCombo        components.Combo
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
	resetVanillaModal       components.Modal
	resetVanillaDiffCount   int  // computed when the button is clicked, shown in the confirm body
	resetVanillaAvailable   bool // cached by refreshResetVanillaAvailability; gates the button itself

	// --- characters view top bar: open (see character_panel.go) ---
	pathEditor   widget.Editor // typed save path, er_pvp_mod-style
	loadTypedBtn widget.Clickable

	// --- characters / merchant-unlock view (see character_panel.go) ---
	charDataPath string // save path CharList/charSaveData were read for
	charSaveData []byte // raw bytes of charDataPath (source for charunlock)
	CharList     []character.Character
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
	// bellBearingState/bellBearingChecks/PendingBellBearingEdits are
	// FlagState/flagChecks/PendingFlagEdits's counterpart for Twin Maiden
	// Husks' bell-bearing acquisition toggles (character.BellBearing)
	// -- a flag with no backing catalog.Row, so kept as its own set of
	// maps (charIndex -> bell-bearing flagID, not rowID) rather than
	// merged into the row-based ones above. Only ever populated when
	// UnlockMerchant == twinMaidenHusksMerchantName; see
	// docs/CHAR_UNLOCK.md's dated entry.
	bellBearingState  map[uint32]bool // flagID -> currently acquired (on disk), for BellBearingsForUI
	bellBearingChecks map[uint32]*widget.Bool
}

// NewState builds the editor state around a ready catalog.
func NewState(cat *catalog.Catalog) *State {
	return NewStateWithDialogs(cat, dialogs.NewNative())
}

// NewStateWithDialogs builds the editor around an injected file-dialog
// service. Tests and future platform adapters can replace native dialogs
// without changing view or application logic.
func NewStateWithDialogs(cat *catalog.Catalog, dialogService dialogs.Service) *State {
	s := &State{
		Session:           application.NewSession(cat),
		Icons:             NewIconCache(),
		dialogs:           dialogService,
		itemCells:         make(map[int64]*cellState),
		rowCells:          make(map[int64]*cellState),
		draftEdits:        make(map[int64]*RowEdit),
		formItemCells:     make(map[int64]*widget.Clickable),
		removeBtns:        make(map[int64]*widget.Clickable),
		removeAllBtns:     make(map[string]*widget.Clickable),
		removeFlagBtns:    make(map[int]*widget.Clickable),
		dropHoverRow:      noRow,
		dragFromRow:       noRow,
		SelectedChar:      -1,
		gatedCacheChar:    -2,
		charBtns:          make(map[int]*widget.Clickable),
		merchantBtns:      make(map[string]*widget.Clickable),
		flagChecks:        make(map[int64]*widget.Bool),
		bellBearingChecks: make(map[uint32]*widget.Bool),
		view:              viewCharacters,
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
