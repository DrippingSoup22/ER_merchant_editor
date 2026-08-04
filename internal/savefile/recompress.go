package savefile

// Write side: fully re-encode the patched BND4 blob as a fresh zstd stream.
//
// The game decoder rejects a stream whose zstd frame shape doesn't match what
// SoulsFormats (soulsmods/SoulsFormatsNEXT, the trusted source for FromSoft
// container formats) writes -- an offline-valid stream can still crash the
// game on load. The load-bearing invariant, from its ZstdHelper.WriteZstd:
//
//	Frame_Content_Size    absent  (ZSTD_c_contentSizeFlag = 0)
//	window                64KB    (ZSTD_c_windowLog = 16)
//	Content_Checksum      off
//
// windowLog=16 matches BetterPSN.dat's own measured window, corroborating
// this as a deliberate requirement. See docs/WRITEBACK.md's "Recompression"
// section for the full history (why the old Raw-block-patch approach was
// replaced, and the in-game crash that pinned down this frame shape).

import (
	"bytes"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// soulsFormatsWindowSize is zstd_c_windowLog=16 (2^16 = 64KB) from
// SoulsFormats' ZstdHelper.WriteZstd -- see package doc comment above.
const soulsFormatsWindowSize = 1 << 16

// buildRecompressedStream re-encodes bnd4 into the frame shape the game
// decoder requires (see package doc). Uses the streaming Write+Close API, not
// EncodeAll: EncodeAll always writes Frame_Content_Size, which is exactly the
// shape that doesn't match.
func buildRecompressedStream(bnd4 []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithWindowSize(soulsFormatsWindowSize),
		zstd.WithEncoderCRC(false),
	)
	if err != nil {
		return nil, fmt.Errorf("zstd encoder: %w", err)
	}
	if _, err := enc.Write(bnd4); err != nil {
		enc.Close()
		return nil, fmt.Errorf("zstd encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("zstd encode close: %w", err)
	}
	return buf.Bytes(), nil
}
