package gio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/unit"
)

// TestTypefaceForMapsEveryFontOption: every Settings.Font value in
// fontOptions (the Font combo's own option list), plus any unrecognized/
// legacy value (including ""), must resolve to a real, non-empty typeface.
// An empty Typeface would be silently hijacked by whichever custom font
// loads first into the shaper's collection (see fonts.go's "BUG FIXED"
// note) instead of resolving to Lora, the actual baseline.
func TestTypefaceForMapsEveryFontOption(t *testing.T) {
	for _, v := range append([]string{"", "not-a-real-font"}, fontValues()...) {
		if got := typefaceFor(v); got == "" {
			t.Errorf("typefaceFor(%q) = empty typeface -- would be hijacked by the first custom font loaded", v)
		}
	}
	if got := typefaceFor(""); got != "Lora" {
		t.Errorf(`typefaceFor("") = %q, want "Lora" (the baseline for any unrecognized/legacy value)`, got)
	}
}

func fontValues() []string {
	vals := make([]string, len(fontOptions))
	for i, o := range fontOptions {
		vals[i] = o.value
	}
	return vals
}

// TestCustomFontCollectionParsesEmbeddedFonts: every embedded font TTF must
// parse without panicking and its Typeface name must match what
// typefaceFor expects to select it by.
func TestCustomFontCollectionParsesEmbeddedFonts(t *testing.T) {
	coll := customFontCollection()
	want := map[string]bool{"Cinzel": false, "Lora": false}
	if len(coll) != len(want) {
		t.Fatalf("customFontCollection() len = %d, want %d", len(coll), len(want))
	}
	for _, ff := range coll {
		tf := string(ff.Font.Typeface)
		if _, ok := want[tf]; !ok {
			t.Errorf("unexpected embedded font typeface %q", tf)
			continue
		}
		want[tf] = true
	}
	for tf, seen := range want {
		if !seen {
			t.Errorf("expected embedded font %q not found in collection", tf)
		}
	}
}

// TestMerchantCellSizeDefaultsWhenUnset covers the Shop Editor cell-size
// slider's persisted-settings fallback: a fresh install or an old config
// (MerchantGridCellSize's JSON zero value) must resolve to
// merchantCellSizeDefault, not 0dp -- a 0dp cell would collapse the whole
// grid.
func TestMerchantCellSizeDefaultsWhenUnset(t *testing.T) {
	s := &State{}
	if got := s.merchantCellSize(); got != merchantCellSizeDefault {
		t.Errorf("merchantCellSize() with unset setting = %v, want default %v", got, merchantCellSizeDefault)
	}

	s.Settings.MerchantGridCellSize = 120
	if got := s.merchantCellSize(); got != 120 {
		t.Errorf("merchantCellSize() with MerchantGridCellSize=120 = %v, want 120", got)
	}
}

// TestCatalogCellSizeDefaultsWhenUnset mirrors
// TestMerchantCellSizeDefaultsWhenUnset for the Catalog grid's own slider.
func TestCatalogCellSizeDefaultsWhenUnset(t *testing.T) {
	s := &State{}
	if got := s.catalogCellSize(); got != catalogCellSizeDefault {
		t.Errorf("catalogCellSize() with unset setting = %v, want default %v", got, catalogCellSizeDefault)
	}

	s.Settings.CatalogGridCellSize = 90
	if got := s.catalogCellSize(); got != 90 {
		t.Errorf("catalogCellSize() with CatalogGridCellSize=90 = %v, want 90", got)
	}
}

// TestStickyCellSizePinsNearNotch covers the sliders' magnetic detents: a
// raw value within cellSizeSnapBand of a notch (a multiple of
// cellSizeSnapStep) must pin to that exact notch, giving the "stuck for a
// moment" feel as the user drags through it.
func TestStickyCellSizePinsNearNotch(t *testing.T) {
	cases := []unit.Dp{100, 97.5, 102.5, 97.1, 102.9}
	for _, raw := range cases {
		if got := stickyCellSize(raw); got != 100 {
			t.Errorf("stickyCellSize(%v) = %v, want pinned to notch 100", raw, got)
		}
	}
}

// TestStickyCellSizeKeepsEveryOtherValue is the exact correction the user
// asked for after an initial version wrongly quantized the whole range:
// "i don't want to reduce the values to just the multiple of 10, i want to
// include all the value." Any value outside a notch's band must come back
// completely unchanged, not rounded to the nearest multiple of 10.
func TestStickyCellSizeKeepsEveryOtherValue(t *testing.T) {
	cases := []unit.Dp{87, 63, 95, 133, 65, 75}
	for _, raw := range cases {
		if got := stickyCellSize(raw); got != raw {
			t.Errorf("stickyCellSize(%v) = %v, want unchanged %v (not a notch -- must stay fully reachable)", raw, got, raw)
		}
	}
}

// TestStickyCellSizeBandEdges covers the exact boundary of the sticky
// band: strictly inside pins to the notch, at-or-outside the boundary
// passes the raw value through unchanged.
func TestStickyCellSizeBandEdges(t *testing.T) {
	half := cellSizeSnapBand // band is symmetric around each notch
	if got := stickyCellSize(100 + half - 0.1); got != 100 {
		t.Errorf("stickyCellSize(100+band-0.1) = %v, want pinned to 100", got)
	}
	if got := stickyCellSize(100 + half); got == 100 {
		t.Errorf("stickyCellSize(100+band) = %v, want the raw value passed through, not pinned", got)
	}
}

// TestOldDebugModeConfigStillLoads: configs written before DebugMode was
// removed (2026-08-03) still contain "debug_mode". Loading one must not
// error, and must NOT silently re-enable the risky-item opt-in that
// replaced it -- ShowRiskyItems has its own key and defaults off, so an
// old debug user doesn't get ban-risk items back without asking.
func TestOldDebugModeConfigStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	raw := `{"theme":"elden","font":"cinzel","debug_mode":true,"auto_free_items":true}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	var got Settings
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("old config failed to parse: %v", err)
	}
	if got.Theme != "elden" || got.Font != "cinzel" || !got.AutoFreeItems {
		t.Errorf("known keys lost: %+v", got)
	}
	if got.ShowRiskyItems {
		t.Error("ShowRiskyItems must stay false for an old debug_mode config " +
			"-- ban-risk items are a deliberate new opt-in")
	}
	if got.ShowSellValueChanges {
		t.Error("ShowSellValueChanges must default false")
	}
}

// TestSellValueChangeText covers the Pending Edits sell-value line's three
// real cases. Unresolvable returns "" so the caller omits the line entirely
// rather than printing a confusing "unknown". The line stays bare
// "before -> after" with no trailing parenthetical -- user request
// 2026-08-03, after the first version's explanatory clause read as noise.
func TestSellValueChangeText(t *testing.T) {
	if got := sellValueChangeText(&equipParamTarget{CurrentOK: false}); got != "" {
		t.Errorf("unresolvable = %q, want empty", got)
	}
	got := sellValueChangeText(&equipParamTarget{Current: 400, Target: 400, CurrentOK: true})
	if !strings.Contains(got, "400") || !strings.Contains(got, "unchanged") {
		t.Errorf("unchanged = %q", got)
	}
	for _, tc := range []struct{ cur, tgt int64 }{{400, 0}, {0, 400}} {
		got := sellValueChangeText(&equipParamTarget{Current: tc.cur, Target: tc.tgt, CurrentOK: true})
		if !strings.Contains(got, "->") {
			t.Errorf("%d->%d = %q, want a before/after arrow", tc.cur, tc.tgt, got)
		}
		if strings.ContainsAny(got, "()") {
			t.Errorf("%d->%d = %q, want no parenthetical", tc.cur, tc.tgt, got)
		}
	}
}
