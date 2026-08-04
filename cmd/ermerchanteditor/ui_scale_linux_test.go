//go:build linux

package main

import "testing"

func TestPlatformUIScale(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("WSL_INTEROP", "")
	if got := platformUIScale(); got != 1 {
		t.Fatalf("native Linux scale = %v, want 1", got)
	}

	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	if got := platformUIScale(); got != 1.25 {
		t.Fatalf("WSL scale = %v, want 1.25", got)
	}
}
