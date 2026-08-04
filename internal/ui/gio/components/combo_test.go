package components

// Pure unit tests for Combo's option/selection bookkeeping and the
// poll-and-clear Changed() contract. headlessGtx/tinyImageOp live in
// iconcell_test.go (same package).

import (
	"testing"

	"gioui.org/unit"
	"gioui.org/widget/material"
)

// TestComboLabelFallback covers SetOptionsWithLabels' display-text handling:
// a nil (or wrong-length) labels array falls back to the raw option values,
// and a correctly-sized labels array is used as-is.
func TestComboLabelFallback(t *testing.T) {
	var c Combo

	// nil labels -> label(i) shows the raw option.
	c.SetOptions([]string{"a", "b", "c"})
	for i, want := range []string{"a", "b", "c"} {
		if got := c.label(i); got != want {
			t.Errorf("nil labels: label(%d) = %q, want %q (raw option)", i, got, want)
		}
	}

	// Correctly-sized labels -> used verbatim.
	c.SetOptionsWithLabels([]string{"", "x"}, []string{"All", "Ex"})
	if got := c.label(0); got != "All" {
		t.Errorf("label(0) = %q, want %q", got, "All")
	}
	if got := c.label(1); got != "Ex" {
		t.Errorf("label(1) = %q, want %q", got, "Ex")
	}

	// Wrong-length labels -> ignored, fall back to the raw options.
	c.SetOptionsWithLabels([]string{"a", "b", "c"}, []string{"only-one"})
	if got := c.label(0); got != "a" {
		t.Errorf("mismatched labels: label(0) = %q, want %q (fallback to option)", got, "a")
	}
}

// TestComboSetOptionsPreservesIndexInRange covers SetOptions' documented
// index preservation: the selected INDEX survives an option replacement while
// it stays in range, and resets to the first option once it doesn't.
func TestComboSetOptionsPreservesIndexInRange(t *testing.T) {
	var c Combo
	c.SetOptions([]string{"a", "b", "c"})
	c.SetValue("c") // sel = 2

	// Same length, different content: index 2 is preserved.
	c.SetOptions([]string{"x", "y", "z"})
	if got := c.Value(); got != "z" {
		t.Errorf("after same-length replace: Value() = %q, want %q (index 2 preserved)", got, "z")
	}

	// Shorter list: index 2 is now out of range -> reset to the first option.
	c.SetOptions([]string{"p", "q"})
	if got := c.Value(); got != "p" {
		t.Errorf("after shorter replace: Value() = %q, want %q (index reset to 0)", got, "p")
	}
}

// TestComboSetValueFallsBackToZero covers SetValue: an exact match selects
// that option, a non-matching value falls back to the first option, and
// SetValue never raises Changed().
func TestComboSetValueFallsBackToZero(t *testing.T) {
	var c Combo
	c.SetOptions([]string{"a", "b", "c"})

	c.SetValue("b")
	if got := c.Value(); got != "b" {
		t.Errorf("SetValue(\"b\"): Value() = %q, want %q", got, "b")
	}

	c.SetValue("does-not-exist")
	if got := c.Value(); got != "a" {
		t.Errorf("SetValue(no match): Value() = %q, want %q (first option)", got, "a")
	}

	if c.Changed() {
		t.Error("SetValue must not raise Changed()")
	}
}

// TestComboChangedPollAndClear covers Changed()'s poll-and-clear semantics:
// a user click (driven here via the per-option Clickable and one Layout pass,
// the same path the real dropdown uses) reports true exactly once, then
// clears.
func TestComboChangedPollAndClear(t *testing.T) {
	th := material.NewTheme()
	gtx := headlessGtx()

	var c Combo
	c.SetOptions([]string{"a", "b", "c"})

	// Selecting the already-selected option must NOT flag a change.
	c.opts[0].Click()
	c.Layout(gtx, th, unit.Dp(120))
	if c.Changed() {
		t.Error("re-selecting the current option must not raise Changed()")
	}

	// Selecting a different option flags a change once.
	c.opts[2].Click()
	c.Layout(gtx, th, unit.Dp(120))
	if c.Value() != "c" {
		t.Fatalf("after clicking option 2: Value() = %q, want %q", c.Value(), "c")
	}
	if !c.Changed() {
		t.Fatal("Changed() = false after a real selection change, want true")
	}
	if c.Changed() {
		t.Error("Changed() must clear after the first poll, want false on the second")
	}
}
