//go:build linux

package main

import "os"

// WSLg commonly reports a 1.0 Gio metric even when its Windows host uses the
// standard 125% desktop scale. Native Linux compositors report their own scale
// and must not receive the WSL-specific correction.
func platformUIScale() float32 {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return 1.25
	}
	return 1
}
