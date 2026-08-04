package icons

import (
	"bytes"
	"testing"
)

// Same display name, distinct game item: each Sorcery must retain its own
// spell-scroll art rather than reuse its Ash of War counterpart's icon.
func TestNamedSorceryIconsDifferFromAshOfWarCounterparts(t *testing.T) {
	for _, name := range []string{
		"glintblade_phalanx",
		"glintstone_pebble",
		"carian_greatsword",
	} {
		sorcery, err := FS.ReadFile("items/sorceries/" + name + ".png")
		if err != nil {
			t.Fatalf("read Sorcery %s: %v", name, err)
		}
		ash, err := FS.ReadFile("items/ashes_of_war/" + name + ".png")
		if err != nil {
			t.Fatalf("read Ash of War %s: %v", name, err)
		}
		if bytes.Equal(sorcery, ash) {
			t.Fatalf("%s Sorcery icon must not reuse the Ash of War icon", name)
		}
	}
}
