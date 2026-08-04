package editor

import (
	"bytes"
	"image"
	"image/color"
	_ "image/png" // registers the PNG decoder for image.Decode
	"sync"
	"time"

	"gioui.org/op/paint"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 3 vendored "PNGs" are really WebP (see decode)

	assets "er_merchant_editor"
)

// iconWorkers bounds how many icons decode concurrently. Decoding is
// CPU-bound (~1ms/icon); a small pool keeps fast-scroll bursts from starving
// the frame loop without saturating every core.
const iconWorkers = 4

// IconCache decodes item icon PNGs from the embedded assets.Icons FS into
// paint.ImageOps, asynchronously: Get never blocks the frame — an unseen icon
// returns the shared gray placeholder immediately and is queued for a worker
// pool, which requests a redraw when the real image is ready. Decoded ops are
// cached forever (the icon set is fixed and small enough to keep resident).
// A missing or undecodable icon resolves permanently to the placeholder.
//
// Concurrency: Get is called from the UI goroutine only; the cache map is
// mutex-guarded because workers complete entries from their own goroutines.
type IconCache struct {
	mu          sync.Mutex
	cache       map[string]paint.ImageOp
	pending     map[string]bool
	queue       chan string
	ready       chan struct{} // coalesced done-signal from workers (cap 1)
	placeholder paint.ImageOp
	invalidate  func() // requests a redraw; set via Start
}

// NewIconCache builds an empty cache with its placeholder op ready. Workers
// start on the first Get after Start wires the redraw callback.
func NewIconCache() *IconCache {
	c := &IconCache{
		cache:       make(map[string]paint.ImageOp),
		pending:     make(map[string]bool),
		queue:       make(chan string, 1024),
		ready:       make(chan struct{}, 1),
		placeholder: placeholderOp(),
	}
	for i := 0; i < iconWorkers; i++ {
		go c.worker()
	}
	go c.notifier()
	return c
}

// Start wires the redraw callback workers use when an icon becomes ready.
func (c *IconCache) Start(invalidate func()) {
	c.mu.Lock()
	c.invalidate = invalidate
	c.mu.Unlock()
}

// Get returns the icon for a repo-relative iconPath (e.g.
// "items/melee_armaments/dagger.png") if it is already decoded, else the
// shared gray placeholder while a worker decodes it in the background.
func (c *IconCache) Get(iconPath string) paint.ImageOp {
	if iconPath == "" {
		return c.placeholder
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if op, ok := c.cache[iconPath]; ok {
		return op
	}
	if !c.pending[iconPath] {
		select {
		case c.queue <- iconPath:
			c.pending[iconPath] = true
		default:
			// Queue full (pathological fast scroll): drop; a later frame
			// re-requests it.
		}
	}
	return c.placeholder
}

func (c *IconCache) worker() {
	for iconPath := range c.queue {
		op := c.decode(iconPath)
		c.mu.Lock()
		c.cache[iconPath] = op
		delete(c.pending, iconPath)
		c.mu.Unlock()
		// Coalesced completion signal — never blocks, at most one queued.
		select {
		case c.ready <- struct{}{}:
		default:
		}
	}
}

// notifier turns the workers' completion signals into at most ~25 redraw
// requests per second: a fast-scroll decode burst used to fire one
// invalidate per icon, forcing back-to-back full frames.
func (c *IconCache) notifier() {
	for range c.ready {
		time.Sleep(40 * time.Millisecond)
		c.mu.Lock()
		invalidate := c.invalidate
		c.mu.Unlock()
		if invalidate != nil {
			invalidate()
		}
	}
}

func (c *IconCache) decode(iconPath string) paint.ImageOp {
	raw, err := assets.Icons.ReadFile("data/icons/" + iconPath)
	if err != nil {
		return c.placeholder
	}
	// Sniff the format instead of assuming PNG: SaveForge's vendored set has 2
	// remaining WebP files with a .png extension (moonrithylls_knight_sword,
	// smithscript_shield; call_of_tibia was re-vended as a real PNG from
	// wiki.gg 2026-08-01, see docs/ITEM_IDS.md) that png.Decode rejected,
	// leaving permanent gray placeholders.
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return c.placeholder
	}
	// Downscale to half resolution (the source icons are ~256px; cells render
	// at 64dp) with ApproxBiLinear — cheap and good enough for thumbnails.
	b := src.Bounds()
	dstW, dstH := b.Dx()/2, b.Dy()/2
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return paint.NewImageOp(dst)
}

func placeholderOp() paint.ImageOp {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	// 0.3 gray, matching the old Dear PyGui missing-icon placeholder.
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x4D, G: 0x4D, B: 0x4D, A: 0xFF})
	return paint.NewImageOp(img)
}
