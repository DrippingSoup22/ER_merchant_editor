package savefile

import (
	"bytes"
	"crypto/md5"
	"os"
	"path/filepath"
	"testing"
)

// pcFixture is a real PC container. save_files/ is gitignored, so CI skips
// every test that needs it, exactly as the PS4 fixture tests already do.
func pcFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "save_files", "pc_fixture.sl2")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("PC fixture save not present, skipping: %v", err)
	}
	return p
}

func ps4Fixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("PS4 fixture save not present, skipping: %v", err)
	}
	return p
}

// Detection must key on container magic alone. A file extension is a
// convention and a decryption attempt is a guess; either one accepted here
// could write a save back in the wrong container shape.
func TestClassifyContainerUsesMagicOnly(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Platform
		ok   bool
	}{
		{"pc BND4", []byte("BND4rest of file"), PlatformPC, true},
		{"ps4 magic", append([]byte{0xCB, 0x01, 0x9C, 0x2C}, "rest"...), PlatformPS4, true},
		{"steam encrypted pc save", []byte{0x9F, 0x21, 0x0C, 0x77, 0x00}, 0, false},
		{"empty", nil, 0, false},
		{"too short", []byte{'B', 'N'}, 0, false},
		{"lowercase magic", []byte("bnd4rest"), 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := classifyContainer(c.data)
			if c.ok && err != nil {
				t.Fatalf("classifyContainer() error = %v, want %s", err, c.want)
			}
			if !c.ok {
				if err == nil {
					t.Fatalf("classifyContainer() = %v, want an error", got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("classifyContainer() = %v, want %v", got, c.want)
			}
		})
	}
}

// The two containers put UserData11 in different places. These are the
// measured offsets from ER_research/save_info/save-architecture.md, verified
// against real files.
func TestUserData11BoundsPerPlatform(t *testing.T) {
	const pcSize, ps4Size = 0x1BA03D0, 0x1BA0080
	if off, end := userData11Bounds(PlatformPC, pcSize); off != 0x19603C0 || end != pcSize {
		t.Fatalf("PC bounds = 0x%X..0x%X, want 0x19603C0..0x%X", off, end, pcSize)
	}
	if off, end := userData11Bounds(PlatformPS4, ps4Size); off != 0x1960070 || end != ps4Size {
		t.Fatalf("PS4 bounds = 0x%X..0x%X, want 0x1960070..0x%X", off, end, ps4Size)
	}
	// The two differ by exactly the container overhead: a 0x300 BND4 header
	// instead of 0x70, plus a 16-byte digest for each of the twelve regions
	// (ten slots, UserData10, and UserData11's own). That is also the file
	// size difference, 0x1BA03D0 - 0x1BA0080 = 0x350.
	pcOff, _ := userData11Bounds(PlatformPC, pcSize)
	psOff, _ := userData11Bounds(PlatformPS4, ps4Size)
	wantDelta := (pcHeaderSize - ps4HeaderSize) + 12*md5Size
	if delta := pcOff - psOff; delta != wantDelta {
		t.Fatalf("container overhead = 0x%X, want 0x%X", delta, wantDelta)
	}
	if pcSize-ps4Size != wantDelta {
		t.Fatalf("file size difference 0x%X does not match container overhead 0x%X", pcSize-ps4Size, wantDelta)
	}
}

// Opening the PC fixture must land on " GER" and report the right platform.
// If the offset arithmetic were wrong this would be reading the middle of a
// character slot instead.
func TestLoadRegulationPC(t *testing.T) {
	reg, err := LoadRegulation(pcFixture(t))
	if err != nil {
		t.Fatalf("LoadRegulation: %v", err)
	}
	if reg.Platform != PlatformPC {
		t.Fatalf("Platform = %v, want PC", reg.Platform)
	}
	if reg.UD11Off != 0x19603C0 {
		t.Fatalf("UD11Off = 0x%X, want 0x19603C0", reg.UD11Off)
	}
	if got := string(reg.FileBytes[reg.UD11Off : reg.UD11Off+4]); got != " GER" {
		t.Fatalf("UserData11 magic = %q, want \" GER\"", got)
	}
	if len(reg.BND4) == 0 || string(reg.BND4[0:4]) != "BND4" {
		t.Fatal("regulation did not decompress to a BND4 archive")
	}
}

// Every region of a PC container is prefixed with MD5(body). Confirming that
// on the untouched fixture is what makes the write-side refresh meaningful.
func TestPCFixtureDigestsAreValid(t *testing.T) {
	data, err := os.ReadFile(pcFixture(t))
	if err != nil {
		t.Fatal(err)
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
		sum := md5.Sum(data[off+md5Size : off+md5Size+body])
		if !bytes.Equal(sum[:], data[off:off+md5Size]) {
			t.Fatalf("region %d digest mismatch at 0x%X", i, off)
		}
		off += md5Size + body
	}
	if off != len(data) {
		t.Fatalf("declared layout consumed 0x%X bytes, file is 0x%X", off, len(data))
	}
}

// The write path's real contract, on both platforms.
//
// It is deliberately NOT "byte-identical output". The reserved capacity after
// the live DCX stream is not zero in a real save -- it holds the original
// PKCS#7 padding and then several hundred KB of stale data left by a longer
// earlier regulation -- and encryptRegulation rewrites that whole region from
// zero. Only the live stream is meaningful, so the contract is:
//
//   - nothing outside UserData11's IV+ciphertext changes, except
//   - on PC, the region's MD5 digest, which must be refreshed, and
//   - the regulation must reload and decompress to the identical BND4.
func TestApplyNoEditsPreservesEverythingThatMatters(t *testing.T) {
	for _, c := range []struct {
		name    string
		fixture func(*testing.T) string
	}{
		{"PC", pcFixture},
		{"PS4", ps4Fixture},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := c.fixture(t)
			reg, err := LoadRegulation(in)
			if err != nil {
				t.Fatalf("LoadRegulation: %v", err)
			}
			blob, capacity, err := buildRegBlob(reg, reg.Stream)
			if err != nil {
				t.Fatalf("buildRegBlob: %v", err)
			}
			out := filepath.Join(t.TempDir(), "out.bin")
			if err := encryptAndSplice(reg, blob, capacity, out); err != nil {
				t.Fatalf("encryptAndSplice: %v", err)
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(reg.FileBytes) {
				t.Fatalf("output length %d, want %d", len(got), len(reg.FileBytes))
			}

			// Everything before UserData11 is untouched -- character slots,
			// UserData10, container header and all their digests.
			guardEnd := reg.UD11Off - md5Size
			if reg.Platform != PlatformPC {
				guardEnd = reg.UD11Off
			}
			if bad := firstMismatch(got[:guardEnd], reg.FileBytes[:guardEnd]); bad >= 0 {
				t.Fatalf("byte 0x%X outside UserData11 changed (got 0x%02X, want 0x%02X)",
					bad, got[bad], reg.FileBytes[bad])
			}
			// The " GER" header and the IV are reused verbatim.
			hdrEnd := reg.UD11Off + ud11UnkHdrSize + aesIVSize
			if bad := firstMismatch(got[reg.UD11Off:hdrEnd], reg.FileBytes[reg.UD11Off:hdrEnd]); bad >= 0 {
				t.Fatalf("UserData11 header/IV changed at +0x%X", bad)
			}
			// On PC the digest must now match the region as written.
			if reg.Platform == PlatformPC {
				want := md5.Sum(got[reg.UD11Off:])
				if !bytes.Equal(got[reg.UD11Off-md5Size:reg.UD11Off], want[:]) {
					t.Fatal("UserData11 digest does not match the region that was written")
				}
			}
			// And the regulation still decompresses to exactly the same PARAM archive.
			round, err := LoadRegulation(out)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			if round.Platform != reg.Platform {
				t.Fatalf("reopened platform = %v, want %v", round.Platform, reg.Platform)
			}
			if bad := firstMismatch(round.BND4, reg.BND4); bad >= 0 {
				t.Fatalf("reopened BND4 differs at 0x%X", bad)
			}
		})
	}
}

// A wrong digest is the failure mode that makes the game reject a PC save, so
// prove the refresh actually recomputes rather than copying the old value.
func TestRefreshUD11DigestRecomputes(t *testing.T) {
	reg, err := LoadRegulation(pcFixture(t))
	if err != nil {
		t.Fatalf("LoadRegulation: %v", err)
	}
	work := append([]byte(nil), reg.FileBytes...)
	// Perturb one byte inside UserData11's body; its digest must change.
	work[reg.UD11Off+0x100] ^= 0xFF
	if err := refreshUD11Digest(reg, work); err != nil {
		t.Fatalf("refreshUD11Digest: %v", err)
	}
	digest := work[reg.UD11Off-md5Size : reg.UD11Off]
	if bytes.Equal(digest, reg.FileBytes[reg.UD11Off-md5Size:reg.UD11Off]) {
		t.Fatal("digest unchanged after the region was modified")
	}
	want := md5.Sum(work[reg.UD11Off:])
	if !bytes.Equal(digest, want[:]) {
		t.Fatal("refreshed digest does not match MD5 of the region body")
	}
}

// The PS4 path must not grow a digest it never had.
func TestRefreshUD11DigestIsNoOpOnPS4(t *testing.T) {
	reg, err := LoadRegulation(ps4Fixture(t))
	if err != nil {
		t.Fatalf("LoadRegulation: %v", err)
	}
	work := append([]byte(nil), reg.FileBytes...)
	if err := refreshUD11Digest(reg, work); err != nil {
		t.Fatalf("refreshUD11Digest: %v", err)
	}
	if bad := firstMismatch(work, reg.FileBytes); bad >= 0 {
		t.Fatalf("PS4 save changed at byte 0x%X", bad)
	}
}
