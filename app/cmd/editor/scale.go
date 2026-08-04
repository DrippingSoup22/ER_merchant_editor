package main

import (
	"os"
	"strconv"

	"gioui.org/app"
)

const (
	uiScaleEnv = "ER_MERCHANT_EDITOR_SCALE"
	minUIScale = 0.75
	maxUIScale = 2.0
)

// applyUIScaleOverride provides a narrowly scoped escape hatch for desktop
// environments that report an incorrect UI scale (notably some WSLg setups).
// Normal Linux and Windows runs leave the platform-provided metric untouched.
func applyUIScaleOverride(e app.FrameEvent) app.FrameEvent {
	raw := os.Getenv(uiScaleEnv)
	if raw == "" {
		return e
	}
	scale, err := strconv.ParseFloat(raw, 32)
	if err != nil || scale < minUIScale || scale > maxUIScale {
		return e
	}
	e.Metric.PxPerDp *= float32(scale)
	e.Metric.PxPerSp *= float32(scale)
	return e
}
