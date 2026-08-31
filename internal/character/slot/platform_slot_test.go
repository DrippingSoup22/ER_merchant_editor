package slot

import (
	"bytes"
	"crypto/md5"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "..", "save_files", name)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Skipf("fixture %s not present, skipping: %v", name, err)
	}
	return data
}

// The slot offsets are the highest-risk part of PC support: get them wrong and
// the unlock path writes flags into the middle of another region. Reading real
// character identities out of a real PC container is the check that the
// arithmetic lands where characters actually are -- garbage offsets cannot
// produce a decodable name and level.
func TestPCSlotOffsetsFindRealCharacters(t *testing.T) {
	data := fixture(t, "pc_fixture.sl2")
	if !isPC(data) {
		t.Fatal("fixture is not a PC container")
	}

	slots := Slots(data)
	if len(slots) != NumSlots {
		t.Fatalf("got %d slots, want %d", len(slots), NumSlots)
	}
	found := 0
	for i, s := range slots {
		if IsEmpty(s) {
			continue
		}
		id, err := ReadIdentity(s)
		if err != nil {
			t.Errorf("slot %d: non-empty but identity unreadable: %v", i, err)
			continue
		}
		if id.Name == "" {
			t.Errorf("slot %d: empty character name", i)
		}
		found++
		t.Logf("slot %d: %q level %d", i, id.Name, id.Level)
	}
	if found == 0 {
		t.Fatal("no readable characters in the PC fixture; slot offsets are almost certainly wrong")
	}
}

// A PC slot start must sit exactly one digest after the previous region ends,
// and the digest stored there must be the MD5 of the slot body.
func TestPCSlotDigestsMatchSlotBodies(t *testing.T) {
	data := fixture(t, "pc_fixture.sl2")
	for i := 0; i < NumSlots; i++ {
		start := slotStart(data, i)
		want := md5.Sum(data[start : start+SlotSize])
		if !bytes.Equal(data[start-DigestSize:start], want[:]) {
			t.Fatalf("slot %d digest at 0x%X does not match its body", i, start-DigestSize)
		}
	}
}

// RefreshDigest must recompute, and must touch nothing but its own sixteen bytes.
func TestPCRefreshDigestIsScoped(t *testing.T) {
	data := fixture(t, "pc_fixture.sl2")
	work := append([]byte(nil), data...)

	const target = 0
	start := slotStart(work, target)
	work[start+0x40] ^= 0xFF // perturb inside the slot body
	if err := RefreshDigest(work, target); err != nil {
		t.Fatalf("RefreshDigest: %v", err)
	}
	want := md5.Sum(work[start : start+SlotSize])
	if !bytes.Equal(work[start-DigestSize:start], want[:]) {
		t.Fatal("digest was not recomputed for the modified slot")
	}
	// Only the perturbed byte and the digest may differ from the original.
	for i := range work {
		changed := work[i] != data[i]
		allowed := i == start+0x40 || (i >= start-DigestSize && i < start)
		if changed && !allowed {
			t.Fatalf("RefreshDigest changed byte 0x%X outside slot %d's digest", i, target)
		}
	}
}

// PlayStation containers have no digests; the call must be inert.
func TestPS4RefreshDigestIsInert(t *testing.T) {
	data := fixture(t, "vanilla_fresh_character.dat")
	if isPC(data) {
		t.Fatal("fixture is not a PlayStation container")
	}
	work := append([]byte(nil), data...)
	if err := RefreshDigest(work, 0); err != nil {
		t.Fatalf("RefreshDigest: %v", err)
	}
	if !bytes.Equal(work, data) {
		t.Fatal("PlayStation save was modified by RefreshDigest")
	}
}

// The two containers put slot 0 in different places, and the gap is exactly
// the BND4 header growth plus slot 0's own digest.
func TestSlotStartPerContainer(t *testing.T) {
	pc := []byte("BND4")
	ps := []byte{0xCB, 0x01, 0x9C, 0x2C}
	if got := slotStart(ps, 0); got != HeaderSize {
		t.Fatalf("PS4 slot 0 at 0x%X, want 0x%X", got, HeaderSize)
	}
	if got := slotStart(pc, 0); got != PCHeaderSize+DigestSize {
		t.Fatalf("PC slot 0 at 0x%X, want 0x%X", got, PCHeaderSize+DigestSize)
	}
	// Slot 3 accumulates three digests ahead of it, plus its own.
	if got := slotStart(pc, 3); got != PCHeaderSize+3*(DigestSize+SlotSize)+DigestSize {
		t.Fatalf("PC slot 3 at 0x%X", got)
	}
}
