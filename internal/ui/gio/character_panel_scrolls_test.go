package gio

// Fixture-backed tests for the scroll-unlock layout (Brother Corhyn/Miriel/
// Sorceress Sellen) -- cross-checks scrollFlagSections' classification
// against the real gated-row/flag data in the fixture save (self-skips if
// save_files/ is absent, same as every other fixture test in this package).

import (
	"strings"
	"testing"
)

// TestScrollFlagSectionsMatchesFixtureSave locks in the exact named/other
// split derived from the fixture save (2026-08-01, tools/savescan.py cross-
// check, see character_panel_scrolls.go's doc comment): 8/11 of Corhyn's
// gated-row groups are scroll-matched (his own 3 questline-only unlocks --
// Great Heal/Lightning Fortification, Discus of Light, Immutable Shield --
// go to "Other Items"), all 11 of Miriel's are (she has no independent
// questline gates of her own), and 3/4 of Sellen's are (Shard Spiral is
// hers alone). A regression here means either the scroll data or the
// matching logic drifted from the real game data.
func TestScrollFlagSectionsMatchesFixtureSave(t *testing.T) {
	cases := []struct {
		merchant       string
		wantNamed      int
		wantOther      int
		wantOtherLabel string // one distinguishing "Other Items" entry, substring match
	}{
		{"Brother Corhyn", 8, 3, "Discus of Light"},
		{"Miriel", 11, 0, ""},
		{"Sorceress Sellen", 3, 1, "Shard Spiral"},
	}

	for _, tc := range cases {
		t.Run(tc.merchant, func(t *testing.T) {
			s := NewState(loadedTestCatalog(t))
			s.ensureCharList()
			s.selectCharacter(7) // level-9 slot, plenty of locked stock (same as other fixture tests)
			if len(s.CharList) == 0 {
				t.Skip("fixture has no character slots")
			}
			s.selectFlagMerchant(tc.merchant)
			if len(s.FlagRows) == 0 {
				t.Fatalf("no gated rows found for %q -- fixture data may have changed", tc.merchant)
			}

			named, other := scrollFlagSections(s.FlagRows)
			if len(named) != tc.wantNamed {
				names := make([]string, len(named))
				for i, e := range named {
					names[i] = e.name
				}
				t.Errorf("%s: %d named groups, want %d (%v)", tc.merchant, len(named), tc.wantNamed, names)
			}
			if len(other) != tc.wantOther {
				t.Errorf("%s: %d other groups, want %d", tc.merchant, len(other), tc.wantOther)
			}
			if tc.wantOtherLabel != "" {
				found := false
				for _, e := range other {
					if strings.Contains(flagGroupLabel(e.group), tc.wantOtherLabel) {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: expected an \"Other Items\" entry mentioning %q", tc.merchant, tc.wantOtherLabel)
				}
			}

			// Coverage: every gated-row group lands in exactly one section.
			total := len(named) + len(other)
			if want := len(groupFlagRows(s.FlagRows)); total != want {
				t.Errorf("%s: named+other = %d, want %d (groupFlagRows count) -- a group was dropped or double-counted", tc.merchant, total, want)
			}
		})
	}
}
