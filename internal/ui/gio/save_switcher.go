package gio

import (
	"errors"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/platform/dialogs"
)

// layoutOpenMessage renders the Characters view's Open bar message area
// (character_panel.go's layoutOpenBar, a Flexed(1) region ending at the
// divider): the busy message, or the inline load error in red, or
// nothing -- always a single line, right-aligned so it reads up against
// the divider. v6: split out of the old single-slot layoutStatus, which
// showed only one of {busy, error, filename} -- a failed load used to
// hide the still-correctly-loaded filename entirely. Now the filename
// lives in its own always-visible slot (layoutFilenameSlot) instead.
func (s *State) layoutOpenMessage(gtx layout.Context, th *material.Theme) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X // give text.End something to align within

	msg := s.BusyMsg()
	color := colorMuted
	if msg == "" {
		msg = s.InlineErr()
		color = colorError
	}
	lbl := material.Body1(th, msg)
	lbl.Color = color
	lbl.MaxLines = 1
	lbl.Alignment = text.End
	return lbl.Layout(gtx)
}

// layoutFilenameSlot renders the loaded save's base filename, centered
// within its slot, single line -- truly empty (no placeholder text) when
// nothing is loaded, per explicit user request. Always visible regardless
// of any concurrent busy/error state in layoutOpenMessage.
func (s *State) layoutFilenameSlot(gtx layout.Context, th *material.Theme) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	lbl := material.Body1(th, s.LoadedName())
	lbl.MaxLines = 1
	lbl.Alignment = text.Middle
	return lbl.Layout(gtx)
}

// openFileDialog shows a native open dialog on a goroutine and, on a valid
// pick, kicks off an async load. Cancelling the dialog is not an error.
func (s *State) openFileDialog() {
	if !s.setDialogOpen(true) {
		return
	}
	go func() {
		defer s.setDialogOpen(false)
		path, err := s.dialogs.OpenSave()
		if errors.Is(err, dialogs.ErrCanceled) || path == "" {
			return
		}
		if err != nil {
			s.setInlineErr(err.Error())
			return
		}
		s.StartLoadSave(path)
	}()
}
