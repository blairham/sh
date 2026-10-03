// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestLocalLoopsStopsALoopControlAtTheFunction pins `localloops`. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155). See
// interp.Runner.endLocalLoops.
func TestLocalLoopsStopsALoopControlAtTheFunction(t *testing.T) {
	const brk = "zsh:1: `break' active at end of function scope\n"
	cases := []struct{ src, want string }{
		{"setopt localloops; f(){ break }; for i in 1 2; do print $i; f; done", "1\n" + brk + "2\n" + brk},
		{"f(){ break }; for i in 1 2; do print $i; f; done", "1\n"},
		{"setopt localloops; f(){ setopt nolocalloops; break }; for i in 1 2; do print $i; f; done", "1\n" + brk + "2\n" + brk},
		{
			"setopt localloops; f(){ continue }; for i in 1; do f; print after; done",
			"zsh:1: `continue' active at end of function scope\n" + brk + "after\n",
		},
		{"f(){ setopt localloops }; f; [[ -o localloops ]] && print on || print off", "off\n"},
		{"setopt localloops; f(){ setopt nolocalloops; g(){ break }; for i in 1 2; do print $i; g; done }; f", "1\n"},
		{"setopt localloops; f(){ break; print in }; for i in 1; do f; print out; done", brk + "out\n"},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
