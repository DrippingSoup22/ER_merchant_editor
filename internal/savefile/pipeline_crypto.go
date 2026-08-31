package savefile

// Crypto + container: UserData11 -> AES-256-CBC decrypt -> DCX/zstd
// decompress -> decompressed BND4 archive. Ported from er_pvp_mod/core/
// regulation.go (crypto/DCX) and tools/savescan.py (the proven-correct
// read implementation this whole package's read side follows). The BND4
// archive itself (pipeline_bnd4.go), inner PARAM header/row table
// (pipeline_param.go), and JSON schema loading (pipeline_schema.go) are
// sibling files -- this one covers everything up through a ready-to-parse
// decompressed blob.

import (
	"bytes"
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
	pcHeaderSize   = 0x300 // BND4 container header
	md5Size        = 0x10  // PC only: MD5(entry body) prefixes every entry
	slotSize       = 0x280000
	numSlots       = 10
	userdata10Size = 0x60000
	ud11UnkHdrSize = 0x10 // unk header, starts with " GER" on both platforms
	aesIVSize      = 16
)

// Platform distinguishes the two save containers this package can open. The
// regulation inside them is identical -- same AES-256-CBC key, same DCX/ZSTD,
// same BND4, same PARAM rows -- so platform only ever affects where UserData11
// starts and whether an MD5 digest has to be refreshed on write.
type Platform int

const (
	PlatformPS4 Platform = iota
	PlatformPC
)

func (p Platform) String() string {
	if p == PlatformPC {
		return "PC"
	}
	return "PS4"
}

// ps4Magic is the first four bytes of a decrypted PlayStation container.
var ps4Magic = []byte{0xCB, 0x01, 0x9C, 0x2C}

// classifyContainer identifies the container by leading magic and nothing else.
//
// Never by file extension: .sl2 and .dat are conventions, not guarantees. Never
// by trial decryption: accepting "decrypts to BND4" as proof of PC would let a
// PlayStation-origin file be opened and then written back in the wrong
// container shape, which is the worst failure available here. An unrecognized
// container is refused rather than guessed at.
//
// A PC save that Steam encrypted on Windows desktop has no leading BND4 and is
// therefore refused. That is deliberate: see errSteamEncrypted.
func classifyContainer(data []byte) (Platform, error) {
	if len(data) >= 4 {
		if string(data[0:4]) == "BND4" {
			return PlatformPC, nil
		}
		if bytes.Equal(data[0:4], ps4Magic) {
			return PlatformPS4, nil
		}
	}
	return 0, errUnknownContainer
}

var (
	// errUnknownContainer covers both a genuinely unsupported file and a
	// Steam-encrypted PC save, because from the leading bytes alone they are
	// indistinguishable and neither can be written back safely.
	errUnknownContainer = fmt.Errorf(
		"unrecognized save container: expected a PC save starting with \"BND4\" or a decrypted " +
			"PlayStation save starting with CB 01 9C 2C. A PC save encrypted by Steam on Windows " +
			"is not supported yet; decrypt it first, or copy it from a Steam Deck where saves are " +
			"stored unencrypted")
)

// userData11Bounds returns the [offset, end) of the UserData11 region.
//
// PS4 packs the twelve regions back to back after a 0x70 header. PC wraps them
// in a 0x300 BND4 and prefixes each with MD5(body), so every region start is
// shifted by one digest and the running total gains one per region.
func userData11Bounds(platform Platform, fileSize int) (int, int) {
	if platform == PlatformPC {
		off := pcHeaderSize +
			numSlots*(md5Size+slotSize) +
			(md5Size + userdata10Size) +
			md5Size // UserData11's own digest precedes its body
		return off, fileSize
	}
	off := ps4HeaderSize + numSlots*slotSize + userdata10Size
	return off, fileSize
}

// Regulation holds everything needed to re-splice a patched blob back.
type Regulation struct {
	FileBytes     []byte   // entire original save file
	Platform      Platform // which container this was read from; decides MD5 refresh on write
	UD11Off       int      // start of UserData11 within FileBytes
	IV            []byte   // 16-byte AES IV (reused verbatim on write)
	CiphertextLen int      // fixed capacity of the encrypted region (== plaintext len)

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
	platform, err := classifyContainer(fileBytes)
	if err != nil {
		return nil, err
	}

	ud11Off, ud11End := userData11Bounds(platform, len(fileBytes))
	if ud11Off >= len(fileBytes) {
		return nil, fmt.Errorf("file too small (%d bytes) for %s UserData11 at 0x%X", len(fileBytes), platform, ud11Off)
	}
	ud11 := fileBytes[ud11Off:ud11End]

	// Both containers hold the same UserData11: unk header (0x10, " GER"
	// magic) then IV+ciphertext. Checking the magic at the computed offset is
	// also the layout check -- if the platform arithmetic were wrong we would
	// be pointing at the middle of a character slot and this would not match.
	if len(ud11) < ud11UnkHdrSize+aesIVSize+16 {
		return nil, fmt.Errorf("UserData11 too short: %d bytes", len(ud11))
	}
	if !(ud11[0] == 0x20 && ud11[1] == 0x47 && ud11[2] == 0x45 && ud11[3] == 0x52) {
		return nil, fmt.Errorf("UserData11 at 0x%X does not carry the \" GER\" magic (got % X); "+
			"the file was read as a %s save but does not have that layout", ud11Off, ud11[0:4], platform)
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
		Platform:      platform,
		UD11Off:       ud11Off,
		IV:            iv,
		CiphertextLen: len(ciphertext),
		Plaintext:     plaintext,
		CompSize:      compSize,
		Stream:        stream,
		BND4:          bnd4,
	}, nil
}
