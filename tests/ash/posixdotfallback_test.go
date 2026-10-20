// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// BusyBox ash looks in the current directory after PATH has missed, and
// **nothing takes that away**, because there is no POSIX mode here to enter.
//
// It matters because this shell reaches [interp.Runner.SetPosixMode] through
// the `sh` name and `sh` is the only name BusyBox ash has, so every run of
// this dialect is a run of that swap. The axis the swap reads has to say
// "unmoved" or the mode removes a reading the real shell has — measured
// 2026-09-16 in the digest-pinned Alpine image, where `. cwdlib.sh` runs the
// copy beside the script with the name absent from PATH.
//
// The same shape as this dialect's DotWithNoOperandIsFatalInPosixMode, and
// for the same reason: a mode the shell does not have must move nothing.
func TestThePosixSwapLeavesTheCurrentDirectoryFallbackAlone(t *testing.T) {
	s := ash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
	}{
		{"DotFallsBackToCurrentDirectory", s.DotFallsBackToCurrentDirectory},
		{"DotFallsBackToCurrentDirectoryInPosixMode", s.DotFallsBackToCurrentDirectoryInPosixMode},
	} {
		if tc.got != interp.Yes {
			t.Errorf("%s = %v, want Yes", tc.axis, tc.got)
		}
	}
}
