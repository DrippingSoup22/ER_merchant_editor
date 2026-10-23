package icons

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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

// These four vendored files previously contained other items' artwork.
// Pin the corrected assets so a future bulk icon refresh cannot silently
// reintroduce the same visually plausible misassignments.
func TestCorrectedItemIconAssets(t *testing.T) {
	want := map[string]string{
		"items/crafting_materials/scorpion_liver.png":         "140812db13ec4907ed22613d9af2aa133f0d4cab98d73c08d2548b2b29f7bf67",
		"items/arrows_and_bolts/piquebone_arrow_fletched.png": "4d44b0d3edf661729b8ad670140b1bdba1af454623223f04b1edb7ef28698700",
		"items/shields/serpent_crest_shield.png":              "28fb71ac318000e902a0bfc4cbc6e8a94d0eb9781a54002832c64cdd1d623897",
		"items/shields/golden_lion_shield.png":                "9af09da3d94eaeefc02a9201fbd559c96c3a5b261f47612e9fcdfc77faee08b1",
	}
	for path, expected := range want {
		data, err := FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		got := sha256.Sum256(data)
		if expected != fmt.Sprintf("%x", got) {
			t.Errorf("%s hash = %x, want %s", path, got, expected)
		}
	}
}
