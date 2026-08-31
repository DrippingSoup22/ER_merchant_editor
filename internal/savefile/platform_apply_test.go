package savefile

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The end-to-end proof for PC: a real merchant edit applied to a real PC
// container, then reopened and checked. This is the same contract
// TestApplyBaseline asserts for PlayStation, so if it passes on both the
// platform work is doing its job.
//
// The edited rows are chosen from the fixture itself rather than hardcoded,
// because the PC fixture is an older regulation than the PS4 one and need not
// contain the same shop rows.
func TestApplyEditOnPCContainer(t *testing.T) {
	fixture := pcFixture(t)

	before := decodeAllShopRows(t, fixture)
	if len(before) == 0 {
		t.Fatal("PC fixture decoded no ShopLineupParam rows")
	}

	// Pick the lowest row ID that carries the fields we intend to change, so
	// the test is deterministic across fixtures.
	var target int64 = -1
	for id, fields := range before {
		if _, ok := fields["value"]; !ok {
			continue
		}
		if _, ok := fields["sellQuantity"]; !ok {
			continue
		}
		if target < 0 || id < target {
			target = id
		}
	}
	if target < 0 {
		t.Fatal("no ShopLineupParam row with both value and sellQuantity")
	}

	const wantValue, wantQty = 4321, 3
	edits := []Edit{{
		RowID: target,
		Fields: map[string]json.Number{
			"value":        json.Number("4321"),
			"sellQuantity": json.Number("3"),
		},
	}}

	out := filepath.Join(t.TempDir(), "pc_edited.sl2")
	summary, err := Apply(fixture, out, "ShopLineupParam.param", edits)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(summary.RowIDs) != 1 || summary.RowIDs[0] != target {
		t.Fatalf("summary touched %v, want [%d]", summary.RowIDs, target)
	}

	// 1. The edit landed, and nothing else in the table moved.
	after := decodeAllShopRows(t, out)
	if len(after) != len(before) {
		t.Fatalf("row count changed: %d -> %d", len(before), len(after))
	}
	for id, b := range before {
		a := after[id]
		for f, bv := range b {
			av := a[f]
			if id == target && f == "value" {
				if av != wantValue {
					t.Errorf("row %d value = %d, want %d", id, av, wantValue)
				}
				continue
			}
			if id == target && f == "sellQuantity" {
				if av != wantQty {
					t.Errorf("row %d sellQuantity = %d, want %d", id, av, wantQty)
				}
				continue
			}
			if av != bv {
				t.Errorf("row %d field %s changed %d -> %d, expected untouched", id, f, bv, av)
			}
		}
	}

	// 2. The container is still a valid PC save: same size, same platform,
	//    and every one of the twelve region digests still verifies. This is
	//    what the game checks on load, and the reason PC needed a write-side
	//    change at all.
	orig, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(orig) {
		t.Fatalf("output size %d, want %d", len(got), len(orig))
	}
	if string(got[0:4]) != "BND4" {
		t.Fatalf("output lost its BND4 magic: % X", got[0:4])
	}
	off := pcHeaderSize
	for i := 0; i < 12; i++ {
		body := slotSize
		switch i {
		case 10:
			body = userdata10Size
		case 11:
			body = 0x240010
		}
		sum := md5.Sum(got[off+md5Size : off+md5Size+body])
		if !bytes.Equal(sum[:], got[off:off+md5Size]) {
			t.Fatalf("region %d digest invalid after edit (offset 0x%X)", i, off)
		}
		off += md5Size + body
	}

	// 3. Character slots and UserData10 are untouched by a merchant edit.
	guard := pcHeaderSize + numSlots*(md5Size+slotSize) + (md5Size + userdata10Size)
	if bad := firstMismatch(got[:guard], orig[:guard]); bad >= 0 {
		t.Fatalf("byte 0x%X outside UserData11 changed", bad)
	}
}
