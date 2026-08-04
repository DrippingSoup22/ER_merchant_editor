package shopwrite

// BND4 archive: the container format holding named .param files inside the
// decompressed regulation blob.

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

type BND4Entry struct {
	Name   string
	Offset int
	Size   int
}

func ListBND4Entries(blob []byte) ([]BND4Entry, error) {
	if len(blob) < 0x10 || string(blob[0:4]) != "BND4" {
		return nil, fmt.Errorf("not a BND4 archive")
	}
	count := int(binary.LittleEndian.Uint32(blob[0x0C:0x10]))
	entries := make([]BND4Entry, 0, count)
	for i := 0; i < count; i++ {
		base := 0x40 + i*0x24
		if base+0x24 > len(blob) {
			return nil, fmt.Errorf("BND4 entry %d out of bounds", i)
		}
		compSize := int(binary.LittleEndian.Uint64(blob[base+8 : base+16]))
		dataOff := int(binary.LittleEndian.Uint32(blob[base+24 : base+28]))
		nameOff := int(binary.LittleEndian.Uint32(blob[base+32 : base+36]))
		name, err := readUTF16LECStr(blob, nameOff)
		if err != nil {
			return nil, err
		}
		entries = append(entries, BND4Entry{Name: name, Offset: dataOff, Size: compSize})
	}
	return entries, nil
}

func readUTF16LECStr(blob []byte, offset int) (string, error) {
	if offset < 0 || offset >= len(blob) {
		return "", fmt.Errorf("BND4 name offset 0x%X out of bounds", offset)
	}
	end := offset
	for end+1 < len(blob) && !(blob[end] == 0 && blob[end+1] == 0) {
		end += 2
	}
	units := make([]uint16, 0, (end-offset)/2)
	for i := offset; i < end; i += 2 {
		units = append(units, binary.LittleEndian.Uint16(blob[i:i+2]))
	}
	return string(utf16.Decode(units)), nil
}

// FindBND4Entry matches an entry by exact name or by "\name"/"/name" suffix
// (BND4 names are full N:\... paths). Same rule as savescan.py extract_bnd4_entry.
func FindBND4Entry(blob []byte, name string) (BND4Entry, error) {
	entries, err := ListBND4Entries(blob)
	if err != nil {
		return BND4Entry{}, err
	}
	for _, e := range entries {
		if e.Name == name || hasSep(e.Name, "\\"+name) || hasSep(e.Name, "/"+name) {
			return e, nil
		}
	}
	return BND4Entry{}, fmt.Errorf("no BND4 entry named %q", name)
}

func hasSep(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
