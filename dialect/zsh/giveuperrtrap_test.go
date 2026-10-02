// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// An error the shell gives up over raises ZERR first, here alone, and an
// error inside a handler ends the handler and nothing more. Standard output
// and the status only: the sentence on standard error is the same either
// way. See interp.Semantics.ErrTrapFiresForAnErrorTheShellGaveUpOver and
// interp.HandlerErrorReach.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin zsh -fc`
// (#5360, and C03traps of #5147).
func TestAGiveUpAndTheErrTrap(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"an unset parameter raises ZERR before the shell gives up", "set -u; trap 'print T' ZERR; : $nope; print after", "T\n", 1},
		{"so does a division by zero", "trap 'print T' ZERR; : $((1/0)); print after", "T\n", 1},
		{"and inside a function, once", "set -u; trap 'print T' ZERR; f() { : $nope; print inf; }; f; print after", "T\n", 1},
		{"a request to stop raises nothing", "trap 'print T' ZERR; : ${nope?boom}; print after", "", 1},
		{"a tested context raises nothing", "set -u; trap 'print T' ZERR; if [[ -n $nope ]]; then :; fi; print after", "", 1},
		{"the interrupted code keeps its status", "set -u; trap 'print T1; : $nope; print T2' ZERR; false; print after $?", "T1\nafter 1\n", 0},
		{"a request to stop in a handler ends the shell", "trap 'print T1; : ${nope?boom}; print T2' ZERR; false; print after $?", "T1\n", 1},
		{"a function handler's error ends only it", "set -u; TRAPZERR() { print T1; : $nope; print T2 }; false; print after $?", "T1\nafter 1\n", 0},
		{"a subshell runs on past it", "set -u; KO() { { : $KO } 2>&1 }; f() { trap 'print T1; KO; print T2' ZERR; false; print inf; }; ( f; print sub ); print after $?", "T1\nKO: KO: parameter not set\ninf\nsub\nafter 0\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, _ := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s\n got %q, %d\nwant %q, %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
