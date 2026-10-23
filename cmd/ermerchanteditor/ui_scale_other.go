//go:build !linux

package main

// Windows and macOS provide Gio with their native display scale.
func platformUIScale() float32 { return 1 }
