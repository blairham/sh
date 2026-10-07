// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The C library's buffer is the platform's: measured 2026-10-07 through dash
// 0.5.12's reads, 1024 bytes on macOS and 8192 in Debian (#6322, #6329).
func TestTheCBufferIsThePlatforms(t *testing.T) {
	for _, c := range []struct {
		goos string
		size interp.ReadSize
		want int
	}{
		{"darwin", interp.ReadSizeCBuffer, 1024},
		{"linux", interp.ReadSizeCBuffer, 8192},
		{"darwin", interp.ReadSizeLine, 0},
		{"linux", 2047, 2047},
	} {
		if got := c.size.BytesOn(c.goos); got != c.want {
			t.Errorf("%d on %s: %d, want %d", c.size, c.goos, got, c.want)
		}
	}
}
