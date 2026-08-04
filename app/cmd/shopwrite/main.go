// Command shopwrite patches merchant shop rows in an Elden Ring PS4 save file.
// Thin CLI over the app/shopwrite library (same flags, exit codes and output
// as the pre-refactor standalone tool). See docs/WRITEBACK.md.
//
// Usage:
//
//	go run ./app/cmd/shopwrite -save <input.dat> -out <output.dat> -entry ShopLineupParam.param -edits <edits.json>
package main

import (
	"flag"
	"fmt"
	"os"

	"er_merchant_editor/app/shopwrite"
)

func main() {
	savePath := flag.String("save", "", "input save file (read-only; never modified)")
	outPath := flag.String("out", "", "output save file to write (must differ from -save)")
	entry := flag.String("entry", "ShopLineupParam.param", "BND4 param entry to edit")
	editsPath := flag.String("edits", "", "JSON file: array of {row_id, fields:{name:value}}")
	flag.Parse()

	if *savePath == "" || *outPath == "" || *editsPath == "" {
		fmt.Fprintln(os.Stderr, "error: -save, -out and -edits are all required")
		flag.Usage()
		os.Exit(2)
	}
	if *savePath == *outPath {
		fatal(fmt.Errorf("-out must differ from -save (never write the input path)"))
	}

	edits, err := shopwrite.LoadEditsFile(*editsPath)
	if err != nil {
		fatal(err)
	}
	if len(edits) == 0 {
		fatal(fmt.Errorf("%s contains no edits", *editsPath))
	}

	summary, err := shopwrite.Apply(*savePath, *outPath, *entry, edits)
	if err != nil {
		fatal(err)
	}
	summary.Print(os.Stderr)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
