package shopwrite

// Crypto + container: UserData11 -> AES-256-CBC decrypt -> DCX/zstd
// decompress -> decompressed BND4 archive. Ported from er_pvp_mod/core/
// regulation.go (crypto/DCX) and tools/savescan.py (the proven-correct
// read implementation this whole package's read side follows). The BND4
// archive itself (pipeline_bnd4.go), inner PARAM header/row table
// (pipeline_param.go), and JSON schema loading (pipeline_schema.go) are
// sibling files -- this one covers everything up through a ready-to-parse
// decompressed blob.

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

// AES-256-CBC key for the regulation.bin blob (same for PC and PS4), copied
// verbatim from er_pvp_mod / SaveForge (docs/MERCHANT_DATA.md).
var regulationKey = []byte{
	0x99, 0xBF, 0xFC, 0x36, 0x6A, 0x6B, 0xC8, 0xC6,
	0xF5, 0x82, 0x7D, 0x09, 0x36, 0x02, 0xD6, 0x76,
	0xC4, 0x28, 0x92, 0xA0, 0x1C, 0x20, 0x7F, 0xB0,
	0x24, 0xD3, 0xAF, 0x4E, 0x49, 0x3F, 0xEF, 0x99,
}

const (
	ps4HeaderSize  = 0x70
	slotSize       = 0x280000
	numSlots       = 10
	userdata10Size = 0x60000
	ud11UnkHdrSize = 0x10 // unk header, starts with " GER" on PS4
	aesIVSize      = 16
)

// userData11Bounds returns the fixed PS4 [offset, end) of the UserData11 region.
func userData11Bounds(fileSize int) (int, int) {
	off := ps4HeaderSize + numSlots*slotSize + userdata10Size
	return off, fileSize
}

// Regulation holds everything needed to re-splice a patched blob back.
type Regulation struct {
	FileBytes     []byte // entire original save file
	UD11Off       int    // start of UserData11 within FileBytes
	IV            []byte // 16-byte AES IV (reused verbatim on write)
	CiphertextLen int    // fixed capacity of the encrypted region (== plaintext len)

	Plaintext []byte // decrypted DCX blob (76-byte header + zstd stream + zero pad)
	CompSize  int    // DCX header compressed_size (length of the live zstd stream)
	Stream    []byte // the zstd compressed stream (Plaintext[76:76+CompSize])

	BND4 []byte // fully decompressed BND4 archive (mutable copy for patching)
}

func LoadRegulation(savePath string) (*Regulation, error) {
	fileBytes, err := os.ReadFile(savePath)
	if err != nil {
		return nil, err
	}
	ud11Off, ud11End := userData11Bounds(len(fileBytes))
	if ud11Off >= len(fileBytes) {
		return nil, fmt.Errorf("file too small (%d bytes) for PS4 UserData11 at 0x%X", len(fileBytes), ud11Off)
	}
	ud11 := fileBytes[ud11Off:ud11End]

	// PS4 layout only: unk header (0x10, " GER" magic) then IV+ciphertext.
	if len(ud11) < ud11UnkHdrSize+aesIVSize+16 {
		return nil, fmt.Errorf("UserData11 too short: %d bytes", len(ud11))
	}
	if !(ud11[0] == 0x20 && ud11[1] == 0x47 && ud11[2] == 0x45 && ud11[3] == 0x52) {
		return nil, fmt.Errorf("UserData11 unk header is not PS4 \" GER\" magic (got % X) - only PS4 saves are supported", ud11[0:4])
	}

	iv := make([]byte, aesIVSize)
	copy(iv, ud11[ud11UnkHdrSize:ud11UnkHdrSize+aesIVSize])
	ciphertext := ud11[ud11UnkHdrSize+aesIVSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("regulation ciphertext (%d bytes) not a multiple of AES block size", len(ciphertext))
	}

	block, err := aes.NewCipher(regulationKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)

	if len(plaintext) < 76 || string(plaintext[0:4]) != "DCX\x00" {
		return nil, fmt.Errorf("decrypted regulation does not start with DCX magic (got % X)", plaintext[0:4])
	}
	format := string(plaintext[40:44])
	if format != "ZSTD" {
		return nil, fmt.Errorf("DCX format is %q, only ZSTD is supported for the PS4 raw-block patch", format)
	}
	decompSize := int(binary.BigEndian.Uint32(plaintext[28:32]))
	compSize := int(binary.BigEndian.Uint32(plaintext[32:36]))
	if 76+compSize > len(plaintext) {
		return nil, fmt.Errorf("DCX compressed_size %d exceeds plaintext length %d", compSize, len(plaintext))
	}
	stream := plaintext[76 : 76+compSize]

	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	bnd4, err := dec.DecodeAll(stream, make([]byte, 0, decompSize))
	if err != nil {
		return nil, fmt.Errorf("zstd decompress: %w", err)
	}
	if len(bnd4) < 4 || string(bnd4[0:4]) != "BND4" {
		return nil, fmt.Errorf("decompressed blob is not a BND4 archive (got % X)", bnd4[0:4])
	}

	return &Regulation{
		FileBytes:     fileBytes,
		UD11Off:       ud11Off,
		IV:            iv,
		CiphertextLen: len(ciphertext),
		Plaintext:     plaintext,
		CompSize:      compSize,
		Stream:        stream,
		BND4:          bnd4,
	}, nil
}
