package main

import "testing"

func TestConfiguredUIScale(t *testing.T) {
	const platformDefault = float32(1.25)
	tests := []struct {
		name string
		raw  string
		want float32
	}{
		{name: "platform default", want: platformDefault},
		{name: "explicit native scale", raw: "1", want: 1},
		{name: "fractional scale", raw: "1.5", want: 1.5},
		{name: "invalid text", raw: "large", want: platformDefault},
		{name: "too small", raw: "0.49", want: platformDefault},
		{name: "too large", raw: "3.01", want: platformDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configuredUIScale(tt.raw, platformDefault); got != tt.want {
				t.Fatalf("configuredUIScale(%q, %v) = %v, want %v", tt.raw, platformDefault, got, tt.want)
			}
		})
	}
}
