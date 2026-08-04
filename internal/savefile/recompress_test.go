package savefile

import (
	"os"
	"path/filepath"
	"testing"
)

// parsedFrameHeader is the subset of a zstd frame header this package's
// compatibility contract cares about (see recompress.go's doc comment).
type parsedFrameHeader struct {
	checksum      bool
	windowSize    int
	contentSizeOK bool // true if a Frame_Content_Size field is present
}

// parseFrameHeader is a minimal, test-only zstd frame header reader (mirrors
// the wire format walkZSTDBlocks/frameenc.go used, kept independent so this
// test doesn't just re-check the encoder against itself).
func parseFrameHeader(t *testing.T, stream []byte) parsedFrameHeader {
	t.Helper()
	fhd := stream[4]
	singleSeg := (fhd>>5)&1 != 0
	checksum := (fhd>>2)&1 != 0
	didFlag := int(fhd & 3)
	fcsFlag := int((fhd >> 6) & 3)

	pos := 5
	windowSize := -1
	if !singleSeg {
		wd := stream[pos]
		exponent := int(wd >> 3)
		mantissa := int(wd & 7)
		windowBase := 1 << (10 + exponent)
		windowSize = windowBase + (windowBase/8)*mantissa
		pos++
	}
	didSizes := [4]int{0, 1, 2, 4}
	pos += didSizes[didFlag]
	fcsSizes := [4]int{0, 2, 4, 8}
	contentSizeOK := (singleSeg && fcsFlag == 0) || fcsSizes[fcsFlag] > 0

	return parsedFrameHeader{checksum: checksum, windowSize: windowSize, contentSizeOK: contentSizeOK}
}

// TestBuildRecompressedStreamMatchesRequiredFrameShape is the regression
// guard for the 2026-08-02 in-game crash (see docs/WRITEBACK.md's
// "Recompression" section): our first recompression attempt produced a
// zstd frame with Content_Checksum_flag set, an explicit Frame_Content_Size,
// and an 8MB window -- none of which match vanilla's own stream or
// save_files/BetterPSN.dat (a real, confirmed-working-in-game third-party
// save). The authoritative shape comes from SoulsFormats'
// Utilities/Compression/ZstdHelper.cs (WriteZstd): contentSizeFlag=0,
// windowLog=16 (64KB) -- treat SoulsFormats as the most trusted source for
// any FromSoft container-format question going forward. This test asserts
// our own output never drifts from that shape again.
func TestBuildRecompressedStreamMatchesRequiredFrameShape(t *testing.T) {
	fixture := filepath.Join("..", "..", "save_files", "vanilla_fresh_character.dat")
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture save not present, skipping: %v", err)
	}
	reg, err := LoadRegulation(fixture)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := buildRecompressedStream(reg.BND4)
	if err != nil {
		t.Fatal(err)
	}
	got := parseFrameHeader(t, stream)
	if got.checksum {
		t.Error("Content_Checksum_flag is set; SoulsFormats/known-working files never set it")
	}
	if got.windowSize != soulsFormatsWindowSize {
		t.Errorf("window size = %d, want %d (SoulsFormats' windowLog=16)", got.windowSize, soulsFormatsWindowSize)
	}
	if got.contentSizeOK {
		t.Error("Frame_Content_Size field is present; SoulsFormats writes with contentSizeFlag=0 (absent)")
	}
}
