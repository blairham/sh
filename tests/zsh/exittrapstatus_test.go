// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// This shell fires an EXIT trap set inside a function when that **function**
// returns — interp.Semantics.ExitTrapIsFunctionLocal, and it is the only
// member of the panel that does. What a caller then reads is the call's own
// status and never the trap's.
//
// That is the reason the construct exists. `f() { trap cleanup EXIT; …;
// return 1 }` is written so that `f || die` still works, and a shell that
// reported the cleanup's result instead made every call look like a success
// — with no diagnostic and nothing in the output, only a branch not taken.
//
// Measured 2026-09-15 on zsh 5.9.2, `env -i` with a scratch HOME, over a
// script file. The rows below are that measurement; the last one is the
// limit, and it is measured rather than assumed.
func TestAFunctionLocalExitTrapKeepsTheCallSStatus(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"an explicit return",
			`g() { trap ':' EXIT; return 2; }; g; printf 'g -> %s\n' "$?"`,
			"g -> 2\n", 0,
		},
		{
			"a body falling off the end of a failing command",
			`h() { trap ':' EXIT; false; }; h; printf 'h -> %s\n' "$?"`,
			"h -> 1\n", 0,
		},
		{
			"a cleanup that fails does not spoil a call that did not",
			`m() { trap 'false' EXIT; return 0; }; m; printf 'm -> %s\n' "$?"`,
			"m -> 0\n", 0,
		},
		{
			"the body is told the call's status",
			`n() { trap 'printf "sees [%s]\n" "$?"' EXIT; return 7; }; n; printf 'n -> %s\n' "$?"`,
			"sees [7]\nn -> 7\n", 0,
		},
		{
			"a body naming a status outright keeps that one",
			`p() { trap 'exit 4' EXIT; return 3; }; p; printf 'unreached\n'`,
			"", 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q status %d", out, st, tc.want, tc.status)
			}
		})
	}
}
