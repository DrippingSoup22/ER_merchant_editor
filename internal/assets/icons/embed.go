// Package icons embeds the item icon images used by the desktop UI. Keep this
// package separate from the catalog data assets: embed variables are not
// dead-code eliminated, so importing it into a CLI would add the full icon set
// to that binary.
package icons

import "embed"

// FS holds items/<category>/<name>.png, addressed by the "items/..." paths in
// the embedded catalog data.
//
//go:embed all:items
var FS embed.FS
