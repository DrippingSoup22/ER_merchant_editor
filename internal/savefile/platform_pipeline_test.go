package savefile

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// regionLayout describes one container's twelve regions so a test can verify
// every digest independently of the code under test.
type regionLayout struct {
	start  int
	bodies [12]int
}

func pcLayout() regionLayout {
	l := regionLayout{start: pcHeaderSize}
	for i := 0; i < 10; i++ {
		l.bodies[i] = slotSize
	}
	l.bodies[10] = userdata10Size
	l.bodies[11] = 0x240010
	return l
}

// assertPCDigests recomputes all twelve MD5s from scratch and compares them to
// the ones stored in the file. This is the check the game performs; doing it
// here from an independent layout constant is what makes it evidence rather
// than a restatement of the implementation.
func assertPCDigests(t *testing.T, data []byte, label string) {
	t.Helper()
	l := pcLayout()
	off := l.start
	for i, body := range l.bodies {
		if off+md5Size+body > len(data) {
			t.Fatalf("%s: region %d runs past end of file", label, i)
		}
		sum := md5.Sum(data[off+md5Size : off+md5Size+body])
		if !bytes.Equal(sum[:], data[off:off+md5Size]) {
			t.Fatalf("%s: region %d digest invalid at 0x%X", label, i, off)
		}
		off += md5Size + body
	}
	if off != len(data) {
		t.Fatalf("%s: layout consumed 0x%X, file is 0x%X", label, off, len(data))
	}
}

// The behaviour the application actually depends on: the container is
// identified on load, and every write stage independently re-identifies it and
// writes back in that same shape. The real save pipeline chains several stages
// through temporary files, so this runs a two-stage chain by hand and checks
// the container survives each hop, including the intermediate.
//
// Running it for both platforms is the point: the PS4 case proves the new
// detection did not change existing behaviour, and the PC case proves the
// digest refresh happens at every stage, not just the last one.
func TestMultiStagePipelinePerPlatform(t *testing.T) {
	for _, c := range []struct {
		name    string
		fixture func(*testing.T) string
		want    Platform
	}{
		{"PC", pcFixture, PlatformPC},
		{"PS4", ps4Fixture, PlatformPS4},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := c.fixture(t)
			dir := t.TempDir()

			// The loader must identify the container before anything is written.
			reg, err := LoadRegulation(in)
			if err != nil {
				t.Fatalf("LoadRegulation: %v", err)
			}
			if reg.Platform != c.want {
				t.Fatalf("detected %v, want %v", reg.Platform, c.want)
			}
			original, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}

			// Two distinct rows, so each stage is a real independent edit
			// rather than the same write twice.
			rows := decodeAllShopRows(t, in)
			var first, second int64 = -1, -1
			for id, f := range rows {
				if _, ok := f["value"]; !ok {
					continue
				}
				switch {
				case first < 0 || id < first:
					second = first
					first = id
				case second < 0 || id < second:
					second = id
				}
			}
			if first < 0 || second < 0 {
				t.Fatal("need two ShopLineupParam rows with a value field")
			}

			// Stage 1: edit the first row.
			stage1 := filepath.Join(dir, "stage1.bin")
			if _, err := Apply(in, stage1,
				"ShopLineupParam.param",
				[]Edit{{RowID: first, Fields: map[string]json.Number{"value": json.Number("555")}}}); err != nil {
				t.Fatalf("stage 1: %v", err)
			}

			// Stage 2: edit a different row, reading what stage 1 produced.
			stage2 := filepath.Join(dir, "stage2.bin")
			if _, err := Apply(stage1, stage2,
				"ShopLineupParam.param",
				[]Edit{{RowID: second, Fields: map[string]json.Number{"value": json.Number("777")}}}); err != nil {
				t.Fatalf("stage 2: %v", err)
			}

			// Every intermediate and the final output must still be the same
			// container, same size, and still openable.
			for _, step := range []struct{ label, path string }{
				{"stage1", stage1},
				{"stage2", stage2},
			} {
				got, err := os.ReadFile(step.path)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != len(original) {
					t.Fatalf("%s: size %d, want %d", step.label, len(got), len(original))
				}
				reopened, err := LoadRegulation(step.path)
				if err != nil {
					t.Fatalf("%s: reopen: %v", step.label, err)
				}
				if reopened.Platform != c.want {
					t.Fatalf("%s: reopened as %v, want %v", step.label, reopened.Platform, c.want)
				}
				if c.want == PlatformPC {
					assertPCDigests(t, got, step.label)
					if string(got[0:4]) != "BND4" {
						t.Fatalf("%s: lost BND4 magic", step.label)
					}
				} else {
					if !bytes.Equal(got[0:4], ps4Magic) {
						t.Fatalf("%s: lost PS4 magic", step.label)
					}
				}
				// Character slots and UserData10 are never touched by either
				// of these stages, on either platform.
				guard := reopened.UD11Off
				if c.want == PlatformPC {
					guard -= md5Size
				}
				if bad := firstMismatch(got[:guard], original[:guard]); bad >= 0 {
					t.Fatalf("%s: byte 0x%X outside UserData11 changed", step.label, bad)
				}
			}

			// Both stages' edits survive into the final output -- stage 2 must
			// not have discarded stage 1's work while rebuilding the region.
			final := decodeAllShopRows(t, stage2)
			if final[first]["value"] != 555 {
				t.Fatalf("stage 1 edit lost: row %d value = %d, want 555", first, final[first]["value"])
			}
			if final[second]["value"] != 777 {
				t.Fatalf("stage 2 edit missing: row %d value = %d, want 777", second, final[second]["value"])
			}
		})
	}
}

// A container must never be written back in the other platform's shape. The
// loader is the only thing standing between a PlayStation save and a PC-shaped
// output, so prove it refuses rather than silently mis-parses.
func TestCrossPlatformInputIsNotMisidentified(t *testing.T) {
	dir := t.TempDir()

	// A PS4 save with a PC header bolted on is not a PC save: the offsets
	// would be wrong and " GER" would not be where PC expects it.
	ps4, err := os.ReadFile(ps4Fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	frankenstein := append([]byte("BND4"), ps4[4:]...)
	bad := filepath.Join(dir, "mislabelled.sl2")
	if err := os.WriteFile(bad, frankenstein, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegulation(bad); err == nil {
		t.Fatal("a PS4 body behind a BND4 magic was accepted as a PC save")
	}

	// Renaming proves nothing either way: the extension is never consulted.
	renamed := filepath.Join(dir, "actually_ps4.sl2")
	if err := os.WriteFile(renamed, ps4, 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegulation(renamed)
	if err != nil {
		t.Fatalf("a PS4 save named .sl2 should still open: %v", err)
	}
	if reg.Platform != PlatformPS4 {
		t.Fatalf("detected %v from a .sl2 filename, want PS4", reg.Platform)
	}
}
