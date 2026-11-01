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
	t.Parallel()
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

// A `{name}&>file` reaches standard error the way `&>file` does — the save
// held for the command, a second redirection of the stream a join — while the
// name takes its number after the file's own, which is let go of before the
// join. Measured 2026-10-27 on zsh 5.9.2 under `-fc` (#6756).
func TestANamedBothStreamsRedirectionReachesStandardError(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{`echo {v}&>|/dev/null {w}&>!/dev/null; echo "[$v][$w]"`, "\n[12][14]\n"},
		{`: {v}&>/dev/null {w}</dev/null {u}</dev/null {x}</dev/null; echo $v $w $u $x`, "12 11 14 15\n"},
		{`: {v}&>/dev/null {w}&>/dev/null {x}</dev/null; echo $v $w $x`, "12 14 17\n"},
		{`: {v}&>/dev/null {w}&>/dev/null {u}&>/dev/null {x}</dev/null; echo $v $w $u $x`, "12 14 18 19\n"},
		{`: {v}&>/dev/null {w}&>/dev/null {u}&>/dev/null {t}&>/dev/null {x}</dev/null; echo $v $w $u $t $x`, "12 14 18 20 21\n"},
		{`: {v}&>/dev/null 2>/dev/null {x}</dev/null; echo $v $x`, "12 16\n"},
		{`: 2>/dev/null {v}&>/dev/null {x}</dev/null; echo $v $x`, "13 16\n"},
		{`: {v}&>/dev/null {w}&>/dev/null 2>/dev/null {x}</dev/null; echo $v $w $x`, "12 14 18\n"},
		{`: {v}&>/dev/null {w}&>/dev/null >/dev/null {x}</dev/null; echo $v $w $x`, "12 14 18\n"},
		{`: {v}&>/dev/null >/dev/null {x}</dev/null; echo $v $x`, "12 14\n"},
		{`: >/dev/null {v}&>/dev/null {x}</dev/null; echo $v $x`, "13 12\n"},
		{`: {v}&>/dev/null {w}>/dev/null {x}</dev/null; echo $v $w $x`, "12 11 14\n"},
		{`: {v}>/dev/null {w}&>/dev/null {x}</dev/null; echo $v $w $x`, "11 13 12\n"},
		{`print -u2 err 2>/dev/null {v}&>/dev/null; echo $v`, "13\n"},
		// The controls: a lone name, and everything given back after.
		{`: {v}&>/dev/null {x}</dev/null; echo $v $x`, "12 11\n"},
		{`: {v}&>/dev/null {w}&>/dev/null; exec {x}</dev/null; echo $v $w $x`, "12 14 11\n"},
	} {
		out, _, errs := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
		if out != c.want || errs != "" {
			t.Errorf("%s: got %q (%q), want %q", c.src, out, errs, c.want)
		}
	}
}
