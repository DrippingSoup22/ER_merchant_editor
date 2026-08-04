package gio

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/ui/gio/components"
)

// Palette (dark by default; applyEditorPalette swaps it — the variables are
// only read during layout on the UI goroutine). The dark values live in ONE
// place, applyEditorPalette's default case; init seeds these vars from it so
// the literals aren't duplicated here (and can't drift). NewState also always
// calls applyTheme (-> applyEditorPalette) before any frame is drawn.
var (
	colorBg         color.NRGBA // window background
	colorPanelBg    color.NRGBA // panel background
	colorFg         color.NRGBA // primary text
	colorMuted      color.NRGBA // secondary text
	colorError      color.NRGBA // error text
	colorDivider    color.NRGBA // separators
	colorInputBg    color.NRGBA // text-input background
	colorAccent     color.NRGBA // buttons
	colorAmber      color.NRGBA // pending-swap tag / picking banner
	colorWarnTxt    color.NRGBA // row warnings
	colorDisabled   color.NRGBA // disabled button bg
	colorContrastFg color.NRGBA // text on accent-colored buttons
)

func init() { applyEditorPalette("dark") }

// applyEditorPalette swaps this package's colors between dark, light, and
// elden (an Elden-Ring-flavored warm parchment/gold/bronze look).
func applyEditorPalette(theme string) {
	switch theme {
	case "light":
		// Warm off-white paper rather than the old blue-grey #F0F0F2, which
		// read as "unstyled default" beside the other two themes (user
		// feedback 2026-08-03). Text is a warm near-black, borders are a
		// visible warm grey (the old #C6C6CC vanished against the panel), and
		// the accent is a deeper slate-blue with enough contrast on white.
		colorBg = color.NRGBA{R: 0xED, G: 0xEA, B: 0xE3, A: 0xFF}
		colorPanelBg = color.NRGBA{R: 0xF8, G: 0xF6, B: 0xF1, A: 0xFF}
		colorFg = color.NRGBA{R: 0x24, G: 0x21, B: 0x1D, A: 0xFF}
		colorMuted = color.NRGBA{R: 0x6B, G: 0x64, B: 0x59, A: 0xFF}
		colorError = color.NRGBA{R: 0xA8, G: 0x2A, B: 0x22, A: 0xFF}
		colorDivider = color.NRGBA{R: 0xC3, G: 0xBA, B: 0xA9, A: 0xFF}
		colorInputBg = color.NRGBA{R: 0xFF, G: 0xFE, B: 0xFB, A: 0xFF}
		colorAccent = color.NRGBA{R: 0x2F, G: 0x5D, B: 0x8C, A: 0xFF}
		colorAmber = color.NRGBA{R: 0x8A, G: 0x5A, B: 0x08, A: 0xFF}
		colorWarnTxt = color.NRGBA{R: 0x8A, G: 0x5A, B: 0x08, A: 0xFF}
		colorDisabled = color.NRGBA{R: 0xD2, G: 0xCB, B: 0xBD, A: 0xFF}
		colorContrastFg = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	case "elden":
		// Matched to the game's own inventory screen (user reference shot,
		// 2026-08-03): a near-BLACK warm ground, not the milky brown this
		// used to be (#171310/#211B14 read as "brown UI", the game reads as
		// "black parchment"). Panels are barely lighter than the window, the
		// divider/border is a dim bronze rather than a light tan, and text is
		// the game's pale parchment gold. The accent gold is desaturated to
		// the menu's own muted tone instead of a bright honey yellow.
		colorBg = color.NRGBA{R: 0x0E, G: 0x0C, B: 0x0A, A: 0xFF}
		colorPanelBg = color.NRGBA{R: 0x16, G: 0x13, B: 0x0F, A: 0xFF}
		colorFg = color.NRGBA{R: 0xD8, G: 0xCD, B: 0xB4, A: 0xFF}
		colorMuted = color.NRGBA{R: 0x8C, G: 0x7E, B: 0x63, A: 0xFF}
		// The game prints unaffordable prices in this red -- reused for
		// errors so the palette stays in-world.
		colorError = color.NRGBA{R: 0xC0, G: 0x3A, B: 0x2E, A: 0xFF}
		colorDivider = color.NRGBA{R: 0x3A, G: 0x31, B: 0x22, A: 0xFF}
		colorInputBg = color.NRGBA{R: 0x1C, G: 0x18, B: 0x12, A: 0xFF}
		colorAccent = color.NRGBA{R: 0xB2, G: 0x93, B: 0x53, A: 0xFF}
		colorAmber = color.NRGBA{R: 0xC9, G: 0xA4, B: 0x5C, A: 0xFF}
		colorWarnTxt = color.NRGBA{R: 0xC9, G: 0xA4, B: 0x5C, A: 0xFF}
		colorDisabled = color.NRGBA{R: 0x35, G: 0x2F, B: 0x25, A: 0xFF}
		colorContrastFg = color.NRGBA{R: 0x12, G: 0x0F, B: 0x0B, A: 0xFF}
	default: // "dark"
		colorBg = color.NRGBA{R: 0x1E, G: 0x1E, B: 0x1E, A: 0xFF}
		colorPanelBg = color.NRGBA{R: 0x25, G: 0x25, B: 0x27, A: 0xFF}
		colorFg = color.NRGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF}
		colorMuted = color.NRGBA{R: 0x8A, G: 0x8A, B: 0x8A, A: 0xFF}
		colorError = color.NRGBA{R: 0xDC, G: 0x5A, B: 0x5A, A: 0xFF}
		colorDivider = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x3C, A: 0xFF}
		colorInputBg = color.NRGBA{R: 0x2A, G: 0x2A, B: 0x2C, A: 0xFF}
		colorAccent = color.NRGBA{R: 0x3A, G: 0x6E, B: 0xA5, A: 0xFF}
		colorAmber = color.NRGBA{R: 0xF0, G: 0xB4, B: 0x50, A: 0xFF}
		colorWarnTxt = color.NRGBA{R: 0xE6, G: 0xAA, B: 0x3C, A: 0xFF}
		colorDisabled = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
		colorContrastFg = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	}
}

// applyTheme applies s.Settings.Theme to both color sets and updates the
// material theme's palette IN PLACE — building a fresh material.NewTheme
// would discard the text shaper and its caches, which re-shapes every label
// and made a theme switch take seconds. UI goroutine only. The shaper's
// custom font collection (fonts.go) is attached once, the first time s.th
// is built, for the same reason -- swapping it out on every call would
// throw away its shaping cache.
func (s *State) applyTheme() {
	applyEditorPalette(s.Settings.Theme)
	components.SetPalette(s.Settings.Theme)
	if s.th == nil {
		s.th = material.NewTheme()
		s.th.Shaper = text.NewShaper(text.WithCollection(customFontCollection()))
		s.th.Face = typefaceFor(s.Settings.Font)
	}
	s.th.Palette.Bg = colorBg
	s.th.Palette.Fg = colorFg
	s.th.Palette.ContrastBg = colorAccent
	s.th.Palette.ContrastFg = colorContrastFg
}

// applyFont switches the theme's default typeface (Settings.Font already
// updated by the caller) IN PLACE, same reasoning as applyTheme: th.Face is
// read fresh by every material widget constructor (material.Body1 etc) on
// its next Layout, no shaper rebuild needed.
func (s *State) applyFont() {
	s.th.Face = typefaceFor(s.Settings.Font)
}

// Theme returns the current material theme (rebuilt on theme switches).
func (s *State) Theme() *material.Theme { return s.th }

// Layout renders one frame: a header (title + save switcher), a divider, and
// the catalog/merchant panels side by side. Resilient to empty data and small
// windows — the two grids scroll independently and nothing panics with no
// save loaded.
func (s *State) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s.consumeReset()

	// Drag-in-flight state, from the source Draggable's own gesture (set
	// during catalog layout, re-checked here every frame so the merchant
	// highlight can never stick after a drag ends). When an instance that
	// actually moved ends — drop, miss, anywhere — the selection clears
	// ("the deselection must be applied as soon as the mechanics end");
	// static clicks (no movement, no transferInit) keep toggle semantics.
	dndNow := s.dragSrc != nil && s.dragSrc.drag.Dragging()
	if s.dndActive && !dndNow {
		// A merchant-row-origin drag (rowsMIME) never touched the CATALOG
		// selection (SelectedItems) in the first place -- clearing it here
		// would wrongly wipe an unrelated catalog selection the user still
		// had active. See dragFromRow's own doc comment.
		if s.transferInit && s.dragFromRow == noRow {
			s.clearSelection()
		}
		s.transferInit = false
		s.dragSrc = nil
		s.dropHoverRow = noRow
		s.dragFromRow = noRow
	}
	s.dndActive = dndNow

	// Opaque background (Gio clears to white otherwise).
	paint.FillShape(gtx.Ops, colorBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// The two blocking modal overlays that share the top tier (error alert +
	// Reset-to-Vanilla confirm) -- kept here, before the main content, so their
	// click handling resolves in the same order as before this was extracted
	// (doResetToVanilla stages edits the footer reads this same frame). Both
	// paint via op.Defer, so they still composite on top of everything. See
	// layoutOverlays.
	s.layoutOverlays(gtx, th)

	// While picking a replacement item (2026-07-29), everything but the
	// catalog grid reads as obscured/inert -- header, merchant grid and the
	// save footer -- so the user's attention is forced onto the one thing
	// they need to act on. The Cancel affordance moves into the catalog
	// panel's own picking banner (layoutCatalogPanel) since it's the one
	// region left interactive.
	picking := s.Picking()

	dims := layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.dimOverlay(gtx, picking, &s.dimTagHeader, s.CancelPicking, func(gtx layout.Context) layout.Dimensions {
					return s.layoutHeader(gtx, th)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return horizontalDivider(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// v5: one shared save footer, identical position/size on
				// every view (previously each view drew its own -- Shop
				// Editor's item-edit Pending/Save, Characters' separate
				// Save Character/Save All) -- see layoutFooterPendingControls.
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						switch s.view {
						case viewSettings:
							return s.layoutSettingsPanel(gtx, th)
						case viewCharacters:
							return s.layoutCharactersPanel(gtx, th)
						default:
							return s.layoutPanels(gtx, th)
						}
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						statusOverlay := s.footerStatusOverlayActive()
						dims := layout.Stack{Alignment: layout.Center}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								return s.dimOverlay(gtx, picking, &s.dimTagFooter, s.CancelPicking, func(gtx layout.Context) layout.Dimensions {
									return barSurface(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutFooterPendingControlsWithStatus(gtx, th, !statusOverlay)
									})
								})
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if !picking {
									return layout.Dimensions{}
								}
								return s.layoutFooterStatus(gtx, th)
							}),
						)
						// Modal backdrops are drawn later with full-window coordinates.
						// Retain the measured footer height so their bright status copy
						// can use this exact same vertical centre, rather than estimating
						// it from the bottom of the window.
						s.footerStatusBarHeight = dims.Size.Y
						return dims
					}),
				)
			}),
		)
	})

	// The pending-edits review modal (2026-07-29: a true blocking modal --
	// full-window scrim + centered panel, like the unexpected-error dialog --
	// replacing an unscrimmed corner dropdown that let clicks reach the
	// panels underneath it) draws last so it sits on top of everything else.
	s.layoutPendingOverlay(gtx, th)
	// The row-edit form (Shop Editor's "Edit (N)" button) is the same
	// blocking-modal tier, drawn after Pending Edits (2026-08-03: converted
	// from a docked bar to a floating modal, matching Pending Edits/Item
	// Info -- see row_edit_form.go's layoutRowEditOverlay).
	s.layoutRowEditOverlay(gtx, th)
	// The item-info popup (right-click a sellable cell, either grid) is the
	// same blocking-modal tier, drawn last so it sits on top of (and isn't
	// itself covered by) the other two in the rare case they'd otherwise
	// overlap (e.g. peeking a different row's info while the row-edit form
	// is open).
	s.layoutItemInfoOverlay(gtx, th)

	// Strict deselection, press-based (registered last = topmost, but
	// pass-through so nothing below loses its events) -- see handleStrictDeselect.
	s.handleStrictDeselect(gtx)
	return dims
}

// layoutOverlays draws the two blocking modal overlays that share the top
// tier -- the unexpected-error alert and the Reset-to-Vanilla confirm dialog.
// Both paint via op.Defer (components.Backdrop) so they composite above the whole
// frame regardless of call position; Layout runs this BEFORE the main content
// so their click handling (dismiss / confirm -> doResetToVanilla, which stages
// edits the footer reads) resolves in the same order as before it was
// extracted. UI goroutine only.
func (s *State) layoutOverlays(gtx layout.Context, th *material.Theme) {
	// Unexpected-error modal overlays everything and swallows input until
	// dismissed.
	if msg := s.ModalErr(); msg != "" {
		if s.modal.OKClicked(gtx) {
			s.dismissModal()
		} else {
			s.modal.Layout(gtx, th, "Something went wrong", msg)
		}
	}

	// Reset-to-Vanilla confirm dialog -- same overlay tier as the error
	// modal above.
	if s.resetVanillaConfirmOpen {
		if s.resetVanillaModal.CancelClicked(gtx) {
			s.resetVanillaConfirmOpen = false
		} else if s.resetVanillaModal.ConfirmClicked(gtx) {
			s.doResetToVanilla()
		} else {
			body := fmt.Sprintf(
				"This will restore %d row(s) across every merchant to their original FromSoftware "+
					"values (item, price, and quantity) -- undoing changes from this and every past "+
					"session. Any character-unlock flags you've staged but not yet saved will also be "+
					"discarded; flags already saved to this file can't be reverted. Nothing is written "+
					"until you click Save.", s.resetVanillaDiffCount)
			s.resetVanillaModal.LayoutConfirm(gtx, th, "Reset to Vanilla?", body, "Reset to Vanilla")
		}
	}
}

// handleStrictDeselect runs the end-of-frame press-based strict-deselection
// pass: any press outside the catalog grid clears the item selection; any
// press outside the merchant grid deselects the row (except while picking
// an item, or while the pending/item-info/row-edit modal is up -- their own
// scrim/close button handle dismissal, and merely clicking inside any of
// them must not also blow away whatever catalog/row selection was active
// underneath). Registered last = topmost, but pass-through so nothing below
// loses its events. UI goroutine only.
func (s *State) handleStrictDeselect(gtx layout.Context) {
	s.pressListener(gtx, &s.pressTag, gtx.Constraints.Max, &s.pressAnywhere)
	// While selecting a replacement, the catalog is the one bright,
	// interactive region. Its own controls and empty space must do nothing;
	// only dimOverlay's opaque regions may cancel the flow. Do not apply the
	// normal grid-deselection policy to those catalog presses.
	if s.Picking() {
		s.pressAnywhere, s.catalogAreaHit, s.merchantAreaHit = false, false, false
		s.editBtnHit = false
		s.pendingModalHit, s.itemInfoModalHit, s.rowEditModalHit = false, false, false
		return
	}
	if s.pressAnywhere && !s.pendingModalHit && !s.itemInfoModalHit && !s.rowEditModalHit {
		if !s.catalogAreaHit {
			s.clearSelection()
		}
		if !s.merchantAreaHit && !s.editBtnHit && !s.Picking() {
			s.clearRowSelection()
		}
	}
	s.pressAnywhere, s.catalogAreaHit, s.merchantAreaHit = false, false, false
	s.editBtnHit = false
	s.pendingModalHit, s.itemInfoModalHit, s.rowEditModalHit = false, false, false
}

// pressListener registers a pass-through Press listener over an area at the
// current transform and drains its events into *hit. Distinct tags (field
// addresses) keep the streams separate.
func (s *State) pressListener(gtx layout.Context, tag event.Tag, size image.Point, hit *bool) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, isPtr := ev.(pointer.Event); isPtr {
			*hit = true
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	pass.Pop()
}

// rightClickTarget is pressListener's secondary-button counterpart: reports
// whether a RIGHT-button press landed in the area this frame (used by the
// item-info popup, catalog_panel.go/merchant_panel.go). Pass-through, same
// mechanics as pressListener, so it never blocks the cell's own click/drag
// handling underneath -- an independent filter on the same tag, delivered
// alongside whatever else that tag already listens for (proved safe by
// merchant cells already stacking a transfer target + Enter/Leave hover
// filter on one tag, see handleRowDrop). Left/other buttons are drained and
// ignored, not left to accumulate.
func rightClickTarget(gtx layout.Context, tag event.Tag, size image.Point) bool {
	fired := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok && pe.Buttons.Contain(pointer.ButtonSecondary) {
			fired = true
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	pass.Pop()
	return fired
}

// layoutHeader is the app name on the left plus the view switcher on the
// right, present in the same position on every view (Open/Save live in the
// Characters view itself — see character_panel.go/pending_edits.go).
func (s *State) layoutHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(material.H6(th, "ER Merchant Editor").Layout),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.viewTabButton(gtx, th, &s.tabCharsBtn, "Characters", viewCharacters)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.viewTabButton(gtx, th, &s.tabEditorBtn, "Shop Editor", viewEditor)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.viewTabButton(gtx, th, &s.tabSettingsBtn, "Settings", viewSettings)
		}),
	)
}

// viewTabButton renders one persistent view-switch tab: active view gets
// the accent background, any click (active or not) just sets s.view --
// no toggle-off "Back" semantics, since the tab bar is always visible.
func (s *State) viewTabButton(gtx layout.Context, th *material.Theme, btn *widget.Clickable, label string, target int) layout.Dimensions {
	if btn.Clicked(gtx) {
		s.view = target
	}
	b := material.Button(th, btn, label)
	if s.view == target {
		b.Background = colorAccent
		b.Color = th.Palette.ContrastFg
	} else {
		b.Background = colorInputBg
		b.Color = colorFg
	}
	return b.Layout(gtx)
}

// layoutPanels is the Shop Editor view: the catalog/merchant grids, side
// by side. The Save footer is shared across every view now (v5) — see
// window.go's Layout, which wraps this in the same Vertical Flex as
// layoutFooterPendingControls. The merchant grid (but not the catalog) is
// dimmed/inert while picking a replacement item — see dimOverlay.
func (s *State) layoutPanels(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutCatalogPanel(gtx, th)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return verticalDivider(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.dimOverlay(gtx, s.Picking(), &s.dimTagMerchant, s.CancelPicking, func(gtx layout.Context) layout.Dimensions {
				return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutMerchantPanel(gtx, th)
				})
			})
		}),
	)
}

// dimScrim is the "obscured" overlay's fill color -- matches
// pendingModalScrim's opacity (pending_edits.go) so both blocking overlays
// in the app read the same way.
var dimScrim = color.NRGBA{A: 0xA0}

// dimOverlay renders w normally, then — while dim is true — paints a
// translucent scrim over it and swallows all pointer input so the region
// reads (and behaves) as obscured/inert. Used to force attention onto the
// catalog grid while picking a replacement item (see layoutPanels/Layout):
// the header, merchant grid and save footer all go through this so only the
// catalog stays clickable, mirroring the "obscure everything but the
// catalog" design the user picked over a separate popup-window catalog.
func (s *State) dimOverlay(gtx layout.Context, dim bool, tag event.Tag, onPress func(), w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	call.Add(gtx.Ops)
	if dim {
		paint.FillShape(gtx.Ops, dimScrim, clip.Rect{Max: dims.Size}.Op())
		// Register (and discard) interest in every pointer kind so this
		// counts as a real input handler over the area, not just paint --
		// an input area with no PassOp is opaque by default (see
		// pressListener's own comment for the inverse, pass-through case),
		// so this blocks every press/scroll/hover reaching whatever's
		// dimmed underneath.
		pressed := false
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll})
			if !ok {
				break
			}
			if pev, ok := ev.(pointer.Event); ok && pev.Kind == pointer.Press {
				pressed = true
			}
		}
		if pressed && onPress != nil {
			onPress()
		}
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		event.Op(gtx.Ops, tag)
		area.Pop()
	}
	return dims
}

// panelSurface fills a panel background and pads its content. Only
// correct inside a Flexed(1) region: forcing Min = Max is what makes it
// grow to fill all remaining space, which is desired for a main content
// panel but would balloon a single-line bar (use barSurface for those).
func panelSurface(gtx layout.Context, w layout.Widget) layout.Dimensions {
	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, w)
}

// barSurface paints a background sized to its content's own natural
// dimensions (found in a Flex{Horizontal} filling full width) — for a
// single-line-height bar (Rigid in a Vertical Flex), which panelSurface's
// Min=Max trick would incorrectly balloon to fill all remaining vertical
// space. At least one child of w's Flex must be Flexed so the row's
// natural width comes out full-width.
func barSurface(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, w)
	call := macro.Stop()

	paint.FillShape(gtx.Ops, colorPanelBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	return dims
}

// flexSpacer is an empty Flexed child that actually claims its allotted
// space, unlike a bare `layout.Dimensions{}` return (which reports zero
// size regardless of the constraints Flex forced on it — Flex sets
// Min=Max=its share for a Flexed child, and only trusts the widget's
// returned Dimensions.Size to account for it when positioning later
// children/computing the row's total width). Used to push content to one
// end of a Flex row, or as a trailing filler so a bar's natural width
// comes out full-width for barSurface's background sizing.
func flexSpacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// filterCountRowHeight is the reserved height of the "N items"/"N rows" line
// under each panel's filter bar -- matches material.Theme's default
// FingerSize (38dp), the tallest thing that line ever has to fit (the
// Catalog panel's Weapon Level slider). Both panels force their count row to
// this height via fixedMinHeight regardless of what's actually showing, so
// (a) toggling the catalog's category filter in/out of a weapon category
// never shifts the grid start position, and (b) the two side-by-side panels
// stay visually symmetric even though only the Catalog panel ever grows a
// slider there.
const filterCountRowHeight = unit.Dp(38)

// fixedMinHeight lays out w normally, then vertically CENTERS its result
// within at least h -- used to keep a row's footprint constant (and its
// content's vertical position consistent with a taller sibling row, e.g.
// the Catalog panel's item-count line next to its Weapon Level slider)
// across frames where the content's natural height varies. A naturally
// taller-than-h result is returned untouched (no re-centering needed: the
// content already IS the frame). Simply padding below (an earlier version
// of this helper did that) left short content sitting at the top of the
// reserved band instead of centered in it.
func fixedMinHeight(gtx layout.Context, h unit.Dp, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	hpx := gtx.Dp(h)
	if dims.Size.Y >= hpx {
		call.Add(gtx.Ops)
		return dims
	}
	off := op.Offset(image.Pt(0, (hpx-dims.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: image.Pt(dims.Size.X, hpx), Baseline: dims.Baseline}
}

// boxed draws a text-input-style background + border around a widget (used for
// the search editor, whose material style has no chrome of its own).
func boxed(gtx layout.Context, th *material.Theme, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(6)).Layout(gtx, w)
	call := macro.Stop()

	paint.FillShape(gtx.Ops, colorInputBg, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	return dims
}

func horizontalDivider(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	sz := image.Pt(gtx.Constraints.Max.X, h)
	paint.FillShape(gtx.Ops, colorDivider, clip.Rect{Max: sz}.Op())
	return layout.Dimensions{Size: sz}
}

func verticalDivider(gtx layout.Context) layout.Dimensions {
	w := gtx.Dp(unit.Dp(1))
	sz := image.Pt(w, gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, colorDivider, clip.Rect{Max: sz}.Op())
	return layout.Dimensions{Size: sz}
}

// gridGap is the spacing between icon cells.
const gridGap = unit.Dp(4)

// layoutGrid lays out count cells in a vertically-scrolling list of rows,
// fitting as many columns as the available width allows (responsive — the
// grid fills its panel instead of hugging the left at a fixed column count).
// cell renders the item at a flat index. Empty data lays out nothing.
// cellSize is the caller's actual rendered cell width (components.IconCellSize
// for the catalog/pending grids' fixed size; the Shop Editor passes its own
// Settings-driven size, see merchant_panel.go's merchantCellSize -- 2026-07-29,
// user asked for an adjustable grid cell size after the on-cell price/
// quantity overlay made the fixed 80dp cells feel cramped).
func layoutGrid(gtx layout.Context, th *material.Theme, list *widget.List, count int, cellSize unit.Dp, cell func(gtx layout.Context, index int) layout.Dimensions) layout.Dimensions {
	list.Axis = layout.Vertical
	cellPx := gtx.Dp(cellSize)
	gapPx := gtx.Dp(gridGap)
	// Reserve a little width for the scrollbar so the last column never clips.
	avail := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(12))
	gridCols := (avail + gapPx) / (cellPx + gapPx)
	if gridCols < 1 {
		gridCols = 1
	}
	rows := (count + gridCols - 1) / gridCols
	return material.List(th, list).Layout(gtx, rows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
		children := make([]layout.FlexChild, 0, gridCols)
		for c := 0; c < gridCols; c++ {
			idx := rowIdx*gridCols + c
			if idx >= count {
				break
			}
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return cell(gtx, idx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		)
	})
}
