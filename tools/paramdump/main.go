// Command paramdump prints one PARAM table of a save as JSON, for the Python
// generators in tools/: the table's bytes (base64) and its rows' IDs and data
// offsets. Reading the save is ER_shared's, so the generators need no
// cryptography or compression of their own.
//
//	go run ./tools/paramdump -save <save> -entry EquipParamWeapon.param
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	reg "github.com/DrippingSoup22/ER_shared/regulation"
)

type row struct {
	ID         int32 `json:"id"`
	DataOffset int   `json:"data_offset"`
}

func main() {
	savePath := flag.String("save", "", "save file (read only)")
	entry := flag.String("entry", "", "BND4 entry, such as EquipParamWeapon.param")
	flag.Parse()
	if *savePath == "" || *entry == "" {
		fmt.Fprintln(os.Stderr, "error: -save and -entry are required")
		os.Exit(2)
	}
	regulation, err := reg.Load(*savePath)
	if err != nil {
		fatal(err)
	}
	_, blob, _, paramRows, err := reg.LoadParamRows(regulation.BND4, *entry)
	if err != nil {
		fatal(err)
	}
	rows := make([]row, len(paramRows))
	for i, r := range paramRows {
		rows[i] = row{r.ID, r.DataOffset}
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Blob []byte `json:"blob"`
		Rows []row  `json:"rows"`
	}{blob, rows}); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
