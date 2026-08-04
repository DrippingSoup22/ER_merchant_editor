// Package assets embeds the item icon PNGs. This lives at the repo root
// because go:embed patterns cannot contain ".." — the root is the only
// package that can reach data/icons. Import it ONLY from the GUI
// (app/editor): embed variables are never dead-code-eliminated, so any
// other importer (e.g. the shopwrite CLI) would swallow ~85MB of PNGs.
package assets

import "embed"

// Icons holds data/icons/items/<category>/<name>.png, addressed by the
// "items/..." iconPath values in data/items.json prefixed with "data/icons/".
//
//go:embed all:data/icons
var Icons embed.FS
