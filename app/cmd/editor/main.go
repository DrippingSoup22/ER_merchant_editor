// Command editor is the Elden Ring merchant save editor: a Gio desktop GUI
// over the app/catalog read side, with in-process write-back via
// app/shopwrite. Single binary, all data embedded.
package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"er_merchant_editor/app/catalog"
	"er_merchant_editor/app/editor"
)

func main() {
	cat, err := catalog.New()
	if err != nil {
		log.Fatalf("initialize catalog: %v", err)
	}
	state := editor.NewState(cat)

	go func() {
		// Last resort: a panic anywhere in the frame loop is written to the
		// editor log before the process dies, so field reports are debuggable
		// (there is no console in a -H windowsgui build).
		defer func() {
			if r := recover(); r != nil {
				log.Fatalf("panic: %v (details logged to %s)", r, editor.LogPanic(r))
			}
		}()
		if err := run(state); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(state *editor.State) error {
	w := new(app.Window)
	w.Option(
		app.Title("ER Merchant Editor"),
		// Size sets the restored (un-maximized) size and, per its own doc
		// comment, resets the window mode to Windowed -- so Maximized must
		// come after it in this list to actually take effect at startup
		// (user: default 1280x800 didn't show the whole UI comfortably).
		app.Size(unit.Dp(1280), unit.Dp(800)),
		app.MinSize(unit.Dp(720), unit.Dp(480)),
		app.Maximized.Option(),
	)
	state.SetWindow(w)

	// Dev convenience: load a save immediately if ER_EDITOR_SAVE is set. Never
	// point this at save_files/ (read-only originals) — copy into
	// working_copies/ first.
	if path := os.Getenv("ER_EDITOR_SAVE"); path != "" {
		state.StartLoadSave(path)
	}

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, applyUIScaleOverride(e))
			// The theme lives on State (rebuilt when the settings view
			// switches palettes).
			state.Layout(gtx, state.Theme())
			e.Frame(gtx.Ops)
		}
	}
}
