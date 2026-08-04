package editor

// Settings view: theme/font, staging defaults, grid sizes, and two opt-in
// display toggles. Settings live in the OS user-config dir, never next to
// the portable exe.
//
// The old catch-all DebugMode toggle was removed 2026-08-03 (user request:
// "only useful to see if the sellValue changes correctly"). It had bundled
// five unrelated behaviors; they were split by what each was actually FOR:
//   - sellValue preview -> ShowSellValueChanges, a normal player-facing
//     toggle, since verifying a price edit really lands is a legitimate
//     everyday need, not developer bookkeeping.
//   - risky-item visibility -> ShowRiskyItems, its own default-OFF toggle:
//     it gates cut-content/online-ban-risk items, so it must stay a
//     deliberate opt-in rather than ride on an unrelated setting.
//   - raw row ids, event-flag ids, internal data-quality warnings: dropped
//     outright, developer bookkeeping with no player-facing meaning.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Settings is the persisted app configuration. Unknown/absent keys fall back
// to their zero value (i.e. normal mode, counts off, unset cell sizes).
type Settings struct {
	Theme string `json:"theme"` // "dark" (default), "light", or "elden"
	// Font selects the UI typeface: one of fontOptions/typefaceFor's
	// embedded faces (fonts.go) -- "lora" (default) or "cinzel".
	Font string `json:"font"`
	// AutoFreeItems/AutoUnlimitedItems: when on, every item swap staged from
	// this point on (picking or dragging a new item onto a row) is staged
	// with price 0 / quantity unlimited (-1) up front (see staging.go's
	// applyAutoItemDefaults). Only sets the starting point; the user can
	// still edit either value afterward.
	AutoFreeItems      bool `json:"auto_free_items"`
	AutoUnlimitedItems bool `json:"auto_unlimited_items"`
	// ShowMerchantRowCounts controls the "(N)" row-count suffix on the
	// Shop Editor's merchant combo (merchant_panel.go's merchantLabel).
	ShowMerchantRowCounts bool `json:"show_merchant_row_counts"`
	// MerchantGridCellSize is the Shop Editor stock grid's icon cell size,
	// in dp. 0 = unset (fresh install / old config) -- merchantCellSize()
	// falls back to merchantCellSizeDefault.
	MerchantGridCellSize int `json:"merchant_grid_cell_size"`
	// CatalogGridCellSize mirrors MerchantGridCellSize for the Catalog grid.
	// 0 = unset -> catalogCellSizeDefault.
	CatalogGridCellSize int `json:"catalog_grid_cell_size"`
	// OpenEditorAfterDrop: when on, dropping catalog item(s) onto merchant
	// stock cell(s) opens the edit window on the affected row(s) right away,
	// so price/quantity can be set without a separate "Edit (N)" click.
	// Drag-swapping two STOCK rows never triggers it -- that reorders what's
	// already there rather than introducing a new item needing a price.
	OpenEditorAfterDrop bool `json:"open_editor_after_drop"`
	// ShowSellValueChanges adds a per-row line to the Pending Edits modal
	// showing the item's own sellValue and what will be written for it (see
	// pending_edits.go). Off by default: it's a secondary, cross-row
	// consequence of a price edit, so it's opt-in detail rather than noise
	// on every pending row.
	ShowSellValueChanges bool `json:"show_sell_value_changes"`
	// ShowRiskyItems reveals cut-content / online-ban-risk items in the
	// Catalog grid (marked with a red border + warning tooltip). Default OFF
	// -- the user can't stage what they can't see, and that gate is
	// deliberate. Kept last in the Settings view as the least-used control.
	ShowRiskyItems bool `json:"show_risky_items"`
}

func defaultSettings() Settings { return Settings{Theme: "dark", Font: "lora"} }

// cellSizeSnapMin/Max bound BOTH grid cell-size sliders (Shop Editor +
// Catalog). cellSizeSnapStep/Band define their "sticky notch" points: every
// value in [cellSizeSnapMin, cellSizeSnapMax] stays individually reachable,
// but a notch (a multiple of cellSizeSnapStep) has a small dead zone
// (cellSizeSnapBand wide on each side) where the value pins to that exact
// notch instead of tracking the raw drag position -- see stickyCellSize.
// Both defaults below land exactly on a notch.
const (
	cellSizeSnapMin  = unit.Dp(60)
	cellSizeSnapMax  = unit.Dp(160)
	cellSizeSnapStep = unit.Dp(10)
	cellSizeSnapBand = unit.Dp(3)

	// merchantCellSizeDefault (100dp) is bumped up from the shared
	// widgets.IconCellSize baseline (80dp) -- the on-cell price/quantity
	// overlay's footer text and corner badge need more room to read.
	merchantCellSizeDefault = unit.Dp(100)
	// catalogCellSizeDefault: the catalog grid has no price/qty overlay, so
	// a bit lower than the Shop Editor's default.
	catalogCellSizeDefault = unit.Dp(90)
)

// stickyCellSize resolves a raw dp reading off the slider's drag position:
// within cellSizeSnapBand dp of a notch (a multiple of cellSizeSnapStep) it
// PINS to that exact notch (so dragging slowly through a notch visibly
// pauses), otherwise it returns raw unchanged (not rounded/quantized), so
// the full continuous range stays reachable.
func stickyCellSize(raw unit.Dp) unit.Dp {
	nearest := unit.Dp(math.Round(float64(raw)/float64(cellSizeSnapStep))) * cellSizeSnapStep
	if raw > nearest-cellSizeSnapBand && raw < nearest+cellSizeSnapBand {
		return nearest
	}
	return raw
}

// handleCellSizeSlider drains one grid cell-size slider's events and, on a
// change that clears the sticky-notch band, writes the resolved dp into *dest
// and persists. Shared by both cell-size sliders (Shop Editor + Catalog) --
// they differ only in which widget.Float / Settings field they drive. The
// sticky value is written BACK into the slider's own Value (normalized) so the
// visible thumb pauses at a notch too, not just the number it drives -- safe
// to overwrite: widget.Float recomputes Value fresh from the pointer's own
// tracked position on the next drag event, it doesn't read Value back in.
func (s *State) handleCellSizeSlider(gtx layout.Context, sl *widget.Float, dest *int) {
	if !sl.Update(gtx) {
		return
	}
	raw := cellSizeSnapMin + unit.Dp(sl.Value)*(cellSizeSnapMax-cellSizeSnapMin)
	sticky := stickyCellSize(raw)
	sl.Value = float32(sticky-cellSizeSnapMin) / float32(cellSizeSnapMax-cellSizeSnapMin)
	if int(sticky) != *dest {
		*dest = int(sticky)
		s.Settings.save()
	}
}

// merchantCellSize resolves the Shop Editor stock grid's current cell size.
func (s *State) merchantCellSize() unit.Dp {
	if s.Settings.MerchantGridCellSize <= 0 {
		return merchantCellSizeDefault
	}
	return unit.Dp(s.Settings.MerchantGridCellSize)
}

// catalogCellSize resolves the Catalog grid's current cell size.
func (s *State) catalogCellSize() unit.Dp {
	if s.Settings.CatalogGridCellSize <= 0 {
		return catalogCellSizeDefault
	}
	return unit.Dp(s.Settings.CatalogGridCellSize)
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "er_merchant_editor", "config.json"), nil
}

// LoadSettings reads the config file, falling back to defaults on any error
// (first run, unreadable file) — settings are never load-bearing.
func LoadSettings() Settings {
	s := defaultSettings()
	path, err := settingsPath()
	if err != nil {
		return s
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return defaultSettings()
	}
	if s.Theme != "light" && s.Theme != "elden" {
		s.Theme = "dark"
	}
	if !validFontValue(s.Font) {
		s.Font = "lora"
	}
	return s
}

// validFontValue reports whether v is one of fontOptions' own values --
// driven by the same list the Font combo itself is built from (fonts.go),
// so a new font added there doesn't also need a hardcoded check here.
func validFontValue(v string) bool {
	for _, o := range fontOptions {
		if o.value == v {
			return true
		}
	}
	return false
}

// save writes the settings best-effort; a failure is logged, never surfaced.
func (s Settings) save() {
	path, err := settingsPath()
	if err != nil {
		appendLog("save settings", err.Error())
		return
	}
	raw, _ := json.MarshalIndent(s, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		appendLog("save settings", err.Error())
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		appendLog("save settings", err.Error())
	}
}

// --- settings view ---

// layoutSettingsPanel is the whole settings view (shown instead of the two
// editor panels while State.view == viewSettings).
func (s *State) layoutSettingsPanel(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Theme combo: apply + persist on change.
	if s.ThemeCombo.Changed() {
		switch s.ThemeCombo.Value() {
		case "Light":
			s.Settings.Theme = "light"
		case "Elden Ring":
			s.Settings.Theme = "elden"
		default:
			s.Settings.Theme = "dark"
		}
		s.applyTheme()
		s.Settings.save()
	}
	// Font combo: apply + persist on change. Same in-place th.Face swap
	// reasoning as the theme combo -- no shaper rebuild needed.
	if s.FontCombo.Changed() {
		s.Settings.Font = s.FontCombo.Value()
		s.applyFont()
		s.Settings.save()
	}
	// Auto-free / auto-unlimited checkboxes: persist on change. Read by
	// staging.go's applyAutoItemDefaults the next time an item is swapped
	// onto a row -- no effect on edits already staged.
	if s.autoFreeChk.Update(gtx) {
		s.Settings.AutoFreeItems = s.autoFreeChk.Value
		s.Settings.save()
	}
	if s.autoUnlimitedChk.Update(gtx) {
		s.Settings.AutoUnlimitedItems = s.autoUnlimitedChk.Value
		s.Settings.save()
	}
	// Merchant row-count checkbox: persist on change. syncMerchants
	// (merchant_panel.go) notices via its own merchantsShowCounts cache
	// field and relabels the combo without reloading the file.
	if s.showCountsChk.Update(gtx) {
		s.Settings.ShowMerchantRowCounts = s.showCountsChk.Value
		s.Settings.save()
	}
	// Grid cell-size sliders: persist on every change that clears the sticky
	// notch band (see handleCellSizeSlider). The grids read
	// Settings.{Merchant,Catalog}GridCellSize directly every frame, so they
	// resize live while dragging.
	s.handleCellSizeSlider(gtx, &s.gridCellSlider, &s.Settings.MerchantGridCellSize)
	s.handleCellSizeSlider(gtx, &s.catalogCellSlider, &s.Settings.CatalogGridCellSize)
	if s.openEditorAfterDropChk.Update(gtx) {
		s.Settings.OpenEditorAfterDrop = s.openEditorAfterDropChk.Value
		s.Settings.save()
	}
	if s.sellValueChk.Update(gtx) {
		s.Settings.ShowSellValueChanges = s.sellValueChk.Value
		s.Settings.save()
	}
	// Risky-items checkbox: persist on change. The catalog's filter cache
	// keys on this, so the item list follows without an explicit reset.
	if s.riskyItemsChk.Update(gtx) {
		s.Settings.ShowRiskyItems = s.riskyItemsChk.Value
		s.Settings.save()
	}
	// Reset to Vanilla: opens the confirm dialog (window.go); the actual
	// staging only happens on confirm, see doResetToVanilla. Guarded by
	// resetVanillaAvailable too -- disabled once nothing differs from
	// vanilla, not just when no save is loaded.
	if s.resetVanillaBtn.Clicked(gtx) && s.Catalog.Loaded() && s.resetVanillaAvailable {
		s.openResetVanillaConfirm()
	}

	return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(material.H6(th, "Settings").Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			// Group: appearance, plus Reset to Vanilla right-aligned on the
			// same row (a global/rare action, not really a group of its own).
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(material.Body1(th, "Theme").Layout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.ThemeCombo.Layout(gtx, th, unit.Dp(140))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(32)}.Layout),
					layout.Rigid(material.Body1(th, "Font").Layout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.FontCombo.Layout(gtx, th, unit.Dp(180))
					}),
					layout.Flexed(1, flexSpacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !s.Catalog.Loaded() || !s.resetVanillaAvailable {
							return disabledButton(gtx, th, "Reset to Vanilla")
						}
						btn := material.Button(th, &s.resetVanillaBtn, "Reset to Vanilla")
						btn.Background = colorInputBg
						btn.Color = colorError
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(settingsGroupDivider),

			// Group: new item swap defaults.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.autoFreeChk, "New item swaps default to free").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.autoUnlimitedChk, "New item swaps default to unlimited stock").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.openEditorAfterDropChk,
					"Open the edit window after dropping catalog items onto stock").Layout(gtx)
			}),
			layout.Rigid(settingsGroupDivider),

			// Group: Shop Editor / Catalog grid display.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.showCountsChk, "Show merchant item counts").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.sellValueChk,
					"Show sell-value changes in Pending Edits").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(cellSizeSettingRow(th, "Shop Editor cell size", &s.gridCellSlider, s.merchantCellSize())),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(cellSizeSettingRow(th, "Catalog cell size", &s.catalogCellSlider, s.catalogCellSize())),
			layout.Rigid(settingsGroupDivider),

			// Group: ban-risk opt-in (least-used) -- last group.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return settingsCheckBox(th, &s.riskyItemsChk,
					"Show cut-content / online-ban-risk items in the Catalog").Layout(gtx)
			}),
		)
	})
}

// settingsCheckBox is material.CheckBox with its label forced up to Body1
// size (th.TextSize) -- material.CheckBox defaults its label to Body2 size
// (th.TextSize*14/16), a mismatch against the Theme/Font/slider labels
// beside it in this view, which all use material.Body1 directly.
func settingsCheckBox(th *material.Theme, checkbox *widget.Bool, label string) material.CheckBoxStyle {
	cb := material.CheckBox(th, checkbox, label)
	cb.TextSize = th.TextSize
	return cb
}

// cellSizeSettingRow is one "<label>  [====slider====]  Ndp" row, shared by
// the Shop Editor and Catalog cell-size settings (they differ only in label,
// backing slider, and the size shown). size is the already-resolved current
// cell size, printed as the trailing "Ndp" readout.
func cellSizeSettingRow(th *material.Theme, label string, sl *widget.Float, size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.Body1(th, label).Layout),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				sldr := material.Slider(th, sl)
				sldr.Color = colorAccent
				return sldr.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Body2(th, fmt.Sprintf("%ddp", int(size))).Layout(gtx)
			}),
		)
	}
}

// settingsGroupDivider separates two settings groups with a full-width rule
// and generous spacing, in contrast to the tight (4-10dp) spacing used
// between controls WITHIN a group -- visually coupling related options.
func settingsGroupDivider(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
		layout.Rigid(horizontalDivider),
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
	)
}
