package components

// Modal is a blocking error dialog: a full-window, press-to-dismiss scrim
// over a centered message panel with an OK button.

import (
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	modalScrim   = color.NRGBA{A: 0xA0}
	modalBg      = color.NRGBA{R: 0x2C, G: 0x2C, B: 0x2E, A: 0xFF}
	modalBorderC = color.NRGBA{R: 0x55, G: 0x55, B: 0x5A, A: 0xFF}
)

// Modal holds the retained state of the dialog. OKClicked must be polled by
// the caller each frame; when it reports true the caller hides the modal.
//
// The same type doubles as a Yes/No confirm dialog via LayoutConfirm/
// CancelClicked/ConfirmClicked. Both variants dismiss when the dimmed area
// is pressed; only a press inside the bright panel leaves them open.
type Modal struct {
	ok            widget.Clickable
	cancel        widget.Clickable
	scrim         widget.Clickable // click-to-dismiss area outside the panel
	panelBlocker  widget.Clickable // swallows blank panel presses before they reach the scrim
	scrimPressTag int              // pointer-down dismissal, below the bright panel
	scrimPressed  bool
}

// dismissClicked drains both the compatibility click target and the dedicated
// pointer-down target. The latter makes dismissal immediate, without waiting
// for the button to be released.
func (m *Modal) dismissClicked(gtx layout.Context) bool {
	dismissed := m.scrimPressed
	m.scrimPressed = false
	for m.scrim.Clicked(gtx) {
		dismissed = true
	}
	return dismissed
}

// OKClicked reports whether OK or the dimmed area was pressed this frame.
func (m *Modal) OKClicked(gtx layout.Context) bool {
	ok := m.ok.Clicked(gtx)
	return m.dismissClicked(gtx) || ok
}

// CancelClicked reports whether Cancel OR a scrim click happened this frame
// -- for a confirm dialog, unlike the OK-only alert above, dismissing via
// the scrim is a valid "no, don't do it" (not swallowed/ignored). Poll this
// BEFORE ConfirmClicked each frame so a scrim tap can never also register
// as a confirm in the same frame.
func (m *Modal) CancelClicked(gtx layout.Context) bool {
	cancel := m.cancel.Clicked(gtx)
	return m.dismissClicked(gtx) || cancel
}

// ConfirmClicked reports whether the confirm button was clicked this frame.
func (m *Modal) ConfirmClicked(gtx layout.Context) bool {
	return m.ok.Clicked(gtx)
}

// Layout draws the scrim and the centered panel. Call only when the modal is
// visible; drawing is deferred so it overlays everything painted before it.
func (m *Modal) Layout(gtx layout.Context, th *material.Theme, title, body string) {
	m.layoutPanel(gtx, th, title, body, func(gtx layout.Context) layout.Dimensions {
		return layout.E.Layout(gtx, material.Button(th, &m.ok, "OK").Layout)
	})
}

// LayoutConfirm is Layout with a Cancel/confirmLabel button pair instead of
// a lone OK -- for a Yes/No confirmation rather than a must-acknowledge
// alert. Poll CancelClicked/ConfirmClicked, not OKClicked, alongside this.
func (m *Modal) LayoutConfirm(gtx layout.Context, th *material.Theme, title, body, confirmLabel string) {
	m.layoutPanel(gtx, th, title, body, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
			layout.Rigid(material.Button(th, &m.cancel, "Cancel").Layout),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(material.Button(th, &m.ok, confirmLabel).Layout),
		)
	})
}

// layoutPanel draws the shared scrim + centered bordered panel (title +
// body + whatever buttons the caller lays out), deferred so it overlays
// everything painted before it. Shared by Layout/LayoutConfirm.
func (m *Modal) layoutPanel(gtx layout.Context, th *material.Theme, title, body string, buttons layout.Widget) {
	Backdrop(gtx,
		BackdropStyle{
			Scrim:        modalScrim,
			PanelBg:      modalBg,
			BorderColor:  modalBorderC,
			BorderWidth:  unit.Dp(1),
			CornerRadius: unit.Dp(3),
			Inset:        unit.Dp(16),
		},
		&m.scrim,
		&m.panelBlocker,
		&m.scrimPressTag,
		func() { m.scrimPressed = true },
		func(gtx *layout.Context) { gtx.Constraints.Max.X = gtx.Dp(unit.Dp(460)) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.H6(th, title).Layout),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(material.Body2(th, body).Layout),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(buttons),
			)
		},
		nil,
	)
}

// BackdropStyle styles the centered panel Backdrop draws over its scrim.
type BackdropStyle struct {
	Scrim        color.NRGBA // full-window scrim fill
	PanelBg      color.NRGBA // panel background fill
	BorderColor  color.NRGBA
	BorderWidth  unit.Dp
	CornerRadius unit.Dp
	Inset        unit.Dp // uniform padding between the border and content
}

// Backdrop draws a blocking modal backdrop: a full-window scrim (fill in
// style.Scrim plus a click-swallowing area backed by scrim) with a centered
// panel whose content is measured via op.Record, background-filled, bordered,
// then replayed on top -- the whole overlay deferred via op.Defer so it paints
// above everything drawn before it.
//
// sizePanel (if non-nil) adjusts the panel's constraints -- max width, and any
// height cap -- after centering but before content is measured; content
// receives the post-inset constraints. panelBlocker is an invisible opaque
// input layer over the entire visible panel, beneath its content's buttons.
// It prevents blank panel clicks from leaking through to the click-to-dismiss
// scrim. scrimPressTag/onScrimPress optionally replace the Clickable with a
// single pointer-down handler. Keeping both layers active makes them compete
// for the first gesture, which can turn dismissal into a second-click action.
// The pointer layer sits below the panel blocker and all controls, so it can
// only fire on the dimmed area.
// afterPanel (if non-nil) runs inside the deferred region after the panel,
// with the ORIGINAL full-window gtx.
func Backdrop(gtx layout.Context, style BackdropStyle, scrim, panelBlocker *widget.Clickable, scrimPressTag event.Tag, onScrimPress func(), sizePanel func(*layout.Context), content layout.Widget, afterPanel func(layout.Context)) {
	rec := op.Record(gtx.Ops)
	if panelBlocker != nil {
		for panelBlocker.Clicked(gtx) {
		}
	}
	// Scrim: full-window fill + exactly one opaque input layer. When a press
	// callback is supplied, do not also lay out the release-based Clickable:
	// overlapping handlers can route the first gesture to the wrong target.
	paint.FillShape(gtx.Ops, style.Scrim, clip.Rect{Max: gtx.Constraints.Max}.Op())
	if scrimPressTag != nil {
		pressed := false
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: scrimPressTag,
				Kinds:  pointer.Cancel | pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Enter | pointer.Leave | pointer.Scroll,
			})
			if !ok {
				break
			}
			if pev, ok := ev.(pointer.Event); ok && pev.Kind == pointer.Press {
				pressed = true
			}
		}
		if pressed && onScrimPress != nil {
			onScrimPress()
		}
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		event.Op(gtx.Ops, scrimPressTag)
		area.Pop()
	} else if scrim != nil {
		scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}

	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if sizePanel != nil {
			sizePanel(&gtx)
		}
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(style.Inset).Layout(gtx, content)
		call := macro.Stop()

		paint.FillShape(gtx.Ops, style.PanelBg, clip.Rect{Max: dims.Size}.Op())
		widget.Border{Color: style.BorderColor, Width: style.BorderWidth, CornerRadius: style.CornerRadius}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return dims })
		if panelBlocker != nil {
			panelBlocker.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		call.Add(gtx.Ops)
		return dims
	})
	if afterPanel != nil {
		afterPanel(gtx)
	}
	op.Defer(gtx.Ops, rec.Stop())
}
