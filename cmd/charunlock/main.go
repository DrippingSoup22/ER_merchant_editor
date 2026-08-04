// Command charunlock is a dev-only CLI over internal/character, for scriptable
// testing of the per-character merchant-unlock write path before any GUI
// wiring. See docs/CHAR_UNLOCK.md.
//
// List characters in a save:
//
//	go run ./cmd/charunlock -save <in> -list-chars
//
// List a character's still-locked rows for one merchant (no write):
//
//	go run ./cmd/charunlock -save <in> -char 0 -merchant "Patches" -list-locked
//
// Unlock every currently-locked row for one merchant, for one character:
//
//	go run ./cmd/charunlock -save <in> -out <out> -char 0 -merchant "Patches"
//
// Unlock specific rows by ID instead:
//
//	go run ./cmd/charunlock -save <in> -out <out> -char 0 -rows 100057,100058
//
// Re-lock instead of unlock (either -merchant or -rows form):
//
//	go run ./cmd/charunlock -save <in> -out <out> -char 0 -rows 100057,100058 -lock
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DrippingSoup22/ER_merchant_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_merchant_editor/internal/character"
)

func main() {
	savePath := flag.String("save", "", "input save file (read-only; never modified)")
	outPath := flag.String("out", "", "output save file to write (required unless -list-chars/-list-locked)")
	charIndex := flag.Int("char", -1, "character slot index (see -list-chars)")
	merchant := flag.String("merchant", "", "merchant name: act on every currently-locked row for this merchant")
	rowsFlag := flag.String("rows", "", "comma-separated row IDs to unlock, instead of -merchant")
	listChars := flag.Bool("list-chars", false, "print every character slot (index/name/level) and exit")
	listLocked := flag.Bool("list-locked", false, "print the target character's still-locked rows and exit, without writing")
	lock := flag.Bool("lock", false, "re-lock the target rows instead of unlocking them")

	flag.Parse()

	if *savePath == "" {
		fatal(fmt.Errorf("-save is required"))
	}

	c, err := catalog.New()
	if err != nil {
		fatal(err)
	}
	if err := c.LoadSave(*savePath); err != nil {
		fatal(err)
	}
	data, err := os.ReadFile(*savePath)
	if err != nil {
		fatal(err)
	}

	if *listChars {
		for _, ch := range character.ListCharacters(data) {
			fmt.Printf("%d\t%s\tlevel %d\n", ch.Index, ch.Name, ch.Level)
		}
		return
	}

	if *charIndex < 0 {
		fatal(fmt.Errorf("-char is required (see -list-chars)"))
	}
	rows, err := targetRows(c, *merchant, *rowsFlag)
	if err != nil {
		fatal(err)
	}

	if *listLocked {
		locked, err := character.LockedRows(data, *charIndex, rows)
		if err != nil {
			fatal(err)
		}
		for _, r := range locked {
			fmt.Printf("%d\t%s\tflag %d\n", r.RowID, r.Label, r.UnlockFlag)
		}
		fmt.Fprintf(os.Stderr, "%d/%d rows still locked\n", len(locked), len(rows))
		return
	}

	if *outPath == "" {
		fatal(fmt.Errorf("-out is required for a write"))
	}
	if *outPath == *savePath {
		fatal(fmt.Errorf("-out must differ from -save (never write the input path)"))
	}

	released := !*lock
	n, err := character.SetRelease(data, *charIndex, rows, released)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fatal(err)
	}
	verb := "unlocked"
	if *lock {
		verb = "locked"
	}
	fmt.Fprintf(os.Stderr, "%s %d row(s), wrote %s\n", verb, n, *outPath)
}

// targetRows resolves the -merchant or -rows selection into catalog rows.
// Exactly one of merchant/rowsCSV must be set.
func targetRows(c *catalog.Catalog, merchant, rowsCSV string) ([]*catalog.Row, error) {
	switch {
	case merchant != "" && rowsCSV != "":
		return nil, fmt.Errorf("-merchant and -rows are mutually exclusive")
	case merchant != "":
		return c.MerchantRows(merchant)
	case rowsCSV != "":
		byID, err := c.RowsByID()
		if err != nil {
			return nil, err
		}
		var out []*catalog.Row
		for _, s := range strings.Split(rowsCSV, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("-rows: invalid row id %q: %w", s, err)
			}
			r, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("-rows: no such row id %d", id)
			}
			out = append(out, r)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("one of -merchant or -rows is required (or use -list-chars)")
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
