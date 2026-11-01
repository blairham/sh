// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A pattern the pathname stage refuses behind a declaration's assignment
// operand leaves `$?` as it was, as every other failed expansion among a
// declaration's words does. From a command string a 0 is the shell's exit
// status; from a script file the shell turns it into 1 as it ends (#7005).
//
// Measured 2026-10-30 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`) under
// `env -i PATH=/usr/bin:/bin`, in a directory where nothing matches. Every
// row was 1 by both routes here: the stage runs after the words' own rules
// are put down, so a refusal there was raised under the command's rule.
//
// The three refusals are an unmatched pattern, a qualifier that will not read
// and a pattern that will not compile, and they move together. What decides
// is an assignment operand written in front of the pattern, which the
// controls hold fixed: the same words in the other order, a plain name in
// front, and a declaration not written as one are all 1.
func TestAPatternRefusedBehindADeclarationsAssignmentLeavesTheStatus(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src       string
		wantC, wantFile int
	}{
		{"typeset", `typeset y=1 *nomatch*`, 0, 1},
		{"export", `export y=1 *nomatch*`, 0, 1},
		{"readonly", `readonly y=1 *nomatch*`, 0, 1},
		{"declare", `declare y=1 *nomatch*`, 0, 1},
		{"integer", `integer y=1 *nomatch*`, 0, 1},
		{"local in a function", `f() { local y=1 *nomatch*; }; f`, 0, 1},
		{"an option in front", `typeset -x y=1 *nomatch*`, 0, 1},
		{"a plain name between", `typeset y=1 z *nomatch*`, 0, 1},
		{"an appending operand", `typeset y+=1 *nomatch*`, 0, 1},
		{"an array-literal operand", `typeset -a a=(1) *nomatch*`, 0, 1},
		{"behind a prefix", `x=1 typeset y=1 *nomatch*`, 0, 1},
		{"a failing status is kept", `(exit 3); typeset y=1 *nomatch*`, 3, 3},
		{"a substitution moves it", `(exit 3); typeset y=$(exit 5) *nomatch*`, 5, 5},
		{"a qualifier that will not read", `typeset y=1 *(Z)`, 0, 1},
		{"a pattern that will not compile", `typeset y=1 a[`, 0, 1},
		{"one inside an array literal", `typeset -a x=(a[)`, 0, 1},
		{"borrowed text", `(exit 3); eval 'typeset y=1 *nomatch*'`, 3, 3},
		{"an always block", `(exit 3); { typeset y=1 *nomatch* } always { : }`, 3, 3},
		{"a subshell", `(exit 3); (typeset y=1 *nomatch*); exit $?`, 3, 3},

		// The same failure with an arithmetic expansion behind the array
		// literal, which a failure there already followed for a scalar one.
		{"a division behind an array-literal operand", `typeset -a a=(1) $((1/0))`, 0, 1},

		// The controls.
		{"no assignment operand", `(exit 3); typeset *nomatch*`, 1, 1},
		{"the assignment after the pattern", `(exit 3); typeset *nomatch* y=1`, 1, 1},
		{"a plain name in front", `(exit 3); typeset y *nomatch*`, 1, 1},
		{"the array literal after the pattern", `(exit 3); typeset -a *nomatch* a=(1)`, 1, 1},
		{"an ordinary command", `(exit 3); echo *nomatch*`, 1, 1},
		{"builtin typeset", `(exit 3); builtin typeset y=1 *nomatch*`, 1, 1},
		{"an escaped name", `(exit 3); \typeset y=1 *nomatch*`, 1, 1},
		{"a qualifier with no assignment", `(exit 3); typeset *(Z)`, 1, 1},
		{"a bad pattern with no assignment", `(exit 3); typeset a[`, 1, 1},
		{"a bad pattern in an ordinary command", `(exit 3); echo a[`, 1, 1},
		{"a loop", `for i in 1; do (exit 3); typeset y=1 *nomatch*; done`, 1, 1},
		{"a pattern that may match nothing", `typeset y=1 *nomatch*(N)`, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, st := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); st != c.wantC {
				t.Errorf("%s from a command string = %d, want %d", c.src, st, c.wantC)
			}
			if _, st := runZshRoute(t, t.TempDir(), c.src, interp.RouteScriptFile); st != c.wantFile {
				t.Errorf("%s from a script file = %d, want %d", c.src, st, c.wantFile)
			}
		})
	}
}

// What an `always` half and the line after a subshell read is the status kept,
// not the fatal one. Measured as above: both write 3.
func TestWhatIsReadAfterARefusedPatternBehindADeclarationsAssignment(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`(exit 3); { typeset y=1 *nomatch* } always { print in=$? }`,
		`(exit 3); (typeset y=1 *nomatch*); print in=$?`,
	} {
		out, _ := runZshRoute(t, t.TempDir(), src, interp.RouteCommandString)
		if out != "zsh:1: no matches found: *nomatch*\nin=3\n" {
			t.Errorf("%s wrote %q, want the refusal and then in=3", src, out)
		}
	}
}
