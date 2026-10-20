// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// `&>file` saves and joins its two streams one at a time, with the file it
// opened held on the lowest free number until the last save it takes.
// Measured 2026-10-15 and 2026-10-16 on zsh 5.9.2 under `-fc` (#6756).
func TestBothStreamsOntoAFileHoldTheFileWhileTheyAreSaved(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`: &>/dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "11 14\n"},
		{`: >&/dev/null {v}</dev/null; echo $v`, "11\n"},
		{`: >/dev/null 2>&1 {v}</dev/null {w}</dev/null; echo $v $w`, "12 14\n"},
		{`: 2>/dev/null &>/dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "16 17\n"},
		{`: 1>/dev/null &>/dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "12 17\n"},
		{`: &>/dev/null >/dev/null {v}</dev/null; echo $v`, "16\n"},
		{`: &>/dev/null &>/dev/null {v}</dev/null; echo $v`, "19\n"},
		{`: >/dev/null 2>/dev/null &>/dev/null {v}</dev/null; echo $v`, "19\n"},
		{`: &>/dev/null; exec {v}</dev/null; echo $v`, "11\n"},
	} {
		out, _, errs := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
		if out != c.want || errs != "" {
			t.Errorf("%s: got %q (%q), want %q", c.src, out, errs, c.want)
		}
	}
}
