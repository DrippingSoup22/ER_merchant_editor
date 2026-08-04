package components

// Tests for Modal's click-polling semantics, especially the documented
// scrim-swallow guarantee: CancelClicked (polled BEFORE ConfirmClicked)
// treats a scrim tap as a "no", while ConfirmClicked -- which reads only the
// confirm button -- never sees that scrim tap, so one scrim click can't
// register as both cancel and confirm in the same frame. headlessGtx lives in
// iconcell_test.go (same package); widget.Clickable.Click() queues a
// synthetic click that the next Clicked()/Update() drains, so no real window
// or pointer-event injection is needed.

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// TestModalScrimClickIsCancelNotConfirm is the core ordering guarantee: a
// lone scrim click, polled in the documented order (Cancel then Confirm),
// must read as cancel and never as confirm.
func TestModalScrimClickIsCancelNotConfirm(t *testing.T) {
	var m Modal
	gtx := headlessGtx()

	m.scrim.Click()

	// Documented poll order: Cancel BEFORE Confirm.
	cancel := m.CancelClicked(gtx)
	confirm := m.ConfirmClicked(gtx)

	if !cancel {
		t.Error("CancelClicked = false after a scrim click, want true (scrim dismiss == cancel)")
	}
	if confirm {
		t.Error("ConfirmClicked = true after a scrim click, want false (a scrim tap must never confirm)")
	}
}

// TestModalConfirmClickNotSwallowedByCancel is the reverse guarantee: polling
// CancelClicked first (which drains scrim clicks) must NOT eat a real confirm
// (OK button) click -- confirm still reports true.
func TestModalConfirmClickNotSwallowedByCancel(t *testing.T) {
	var m Modal
	gtx := headlessGtx()

	m.ok.Click()

	cancel := m.CancelClicked(gtx)
	confirm := m.ConfirmClicked(gtx)

	if cancel {
		t.Error("CancelClicked = true after a confirm click, want false (no scrim/cancel happened)")
	}
	if !confirm {
		t.Error("ConfirmClicked = false after clicking the confirm button, want true")
	}
}

// TestModalCancelButtonClick covers the explicit Cancel button (as opposed to
// the scrim) also reading as a cancel.
func TestModalCancelButtonClick(t *testing.T) {
	var m Modal
	gtx := headlessGtx()

	m.cancel.Click()

	if !m.CancelClicked(gtx) {
		t.Error("CancelClicked = false after clicking the Cancel button, want true")
	}
	if m.ConfirmClicked(gtx) {
		t.Error("ConfirmClicked = true after a Cancel-button click, want false")
	}
}

// TestModalOKClickedDismissesOnScrim covers the general overlay rule for an
// OK-only alert: the dimmed region dismisses it just like the OK button.
func TestModalOKClickedDismissesOnScrim(t *testing.T) {
	var m Modal
	gtx := headlessGtx()

	// A scrim click on an OK-only alert dismisses it.
	m.scrim.Click()
	if !m.OKClicked(gtx) {
		t.Error("OKClicked = false after a scrim click, want true (dimmed area dismisses the alert)")
	}

	// A real OK-button click acknowledges.
	m.ok.Click()
	if !m.OKClicked(gtx) {
		t.Error("OKClicked = false after clicking OK, want true")
	}
}

func TestModalScrimPressDismisses(t *testing.T) {
	var m Modal
	gtx := headlessGtx()
	m.scrimPressed = true
	if !m.CancelClicked(gtx) {
		t.Error("CancelClicked = false after scrim pointer-down, want true")
	}
	if m.scrimPressed {
		t.Error("scrimPressed was not consumed")
	}
}

func TestBackdropFirstPressDismissesOnlyOutsidePanel(t *testing.T) {
	const (
		windowW = 400
		windowH = 300
		panelW  = 100
		panelH  = 80
	)

	run := func(t *testing.T, pos f32.Point, wantDismissals int) {
		t.Helper()
		var (
			r            input.Router
			ops          op.Ops
			scrim        widget.Clickable
			panelBlocker widget.Clickable
			pressTag     int
			dismissals   int
		)
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(windowW, windowH)),
		}
		frame := func() {
			gtx.Reset()
			Backdrop(gtx, BackdropStyle{Inset: unit.Dp(10)},
				&scrim, &panelBlocker, &pressTag, func() { dismissals++ }, nil,
				func(layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(panelW, panelH)}
				}, nil)
			r.Frame(&ops)
		}

		frame() // register the backdrop's input areas
		r.Queue(pointer.Event{
			Kind:      pointer.Press,
			Source:    pointer.Mouse,
			Buttons:   pointer.ButtonPrimary,
			Position:  pos,
			PointerID: 1,
		})
		frame() // the first pointer-down must decide dismissal
		if dismissals != wantDismissals {
			t.Errorf("dismissals after first press at %v = %d, want %d", pos, dismissals, wantDismissals)
		}
	}

	t.Run("dimmed area", func(t *testing.T) {
		run(t, f32.Pt(10, 10), 1)
	})
	t.Run("bright panel", func(t *testing.T) {
		run(t, f32.Pt(windowW/2, windowH/2), 0)
	})
}
