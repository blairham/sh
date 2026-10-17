// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A function-local EXIT trap is not run for a call that an error the shell
// reported and gave up over ended (#6667), and reads `$?` as 0 for a call an
// `exit` ended (#6836).
//
// Measured 2026-10-16 on zsh 5.9.2 under `-f`, by `-c`, a script file and
// standard input alike, with `f(){ trap 'echo ft=$?' EXIT; <row>; }; f; echo
// after`. The controls are the stops the script asked for — `${x?word}` and
// errexit, which run it at 1 — and the ordinary ends of a call, which run it
// at the call's status; and the two places an error is survived inside the
// body, `eval` and a subshell, where the trap runs as the call returns. See
// Semantics.FunctionExitTrapSkippedByAGiveUp and
// Semantics.FunctionExitTrapReadsZeroOnAnExit.
func TestAFunctionExitTrapAfterAGiveUpOrAnExit(t *testing.T) {
	const f = "f(){ trap 'echo ft=$?' EXIT; "
	for _, c := range []struct {
		src, want  string
		wantStatus int
	}{
		{f + "echo $((1/0)); }; f; echo after\n", "", 1},
		// 0 through `-c` and 1 from a file or standard input, in zsh as
		// here: the status is #6666's question and not this one's, so only
		// the output is held to it.
		{f + "true; export x=$((1/0)); }; f; echo after\n", "", -1},
		{f + "set -o nosuch; }; f; echo after\n", "", 1},
		{f + "readonly r=1; r=2; }; f; echo after\n", "", 1},
		{"g(){ echo $((1/0)); }; " + f + "g; echo in; }; f; echo after\n", "", 1},
		{f + "exit 3; }; f; echo after\n", "ft=0\n", 3},
		{f + "false; exit 3; }; f; echo after\n", "ft=0\n", 3},
		{f + ": ${q?b}; }; f; echo after\n", "ft=1\n", 1},
		{f + "set -e; false; }; f; echo after\n", "ft=1\n", 1},
		{f + "eval 'echo $((1/0))'; echo in; }; f; echo after $?\n", "in\nft=0\nafter 0\n", 0},
		{f + "false; }; f; echo after\n", "ft=1\nafter\n", 0},
		{f + "return 4; }; f; echo after\n", "ft=4\nafter\n", 0},
	} {
		for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile, interp.RouteStandardInput} {
			out, st, _ := runZshSplitOnRoute(t, route, c.src)
			if out != c.want || (c.wantStatus >= 0 && st != c.wantStatus) {
				t.Errorf("%v %q: wrote %q at %d, want %q at %d", route, c.src, out, st, c.want, c.wantStatus)
			}
		}
	}
}
