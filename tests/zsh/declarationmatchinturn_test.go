// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// From a declaration's first assignment operand on, the pathname stage runs as
// the walk goes: the words held in front of the operand are matched before it
// is expanded, and each word behind it before the next one is, so a refusal
// there leaves the later words unexpanded (#7125). Everywhere else the stage
// runs once every word is expanded (#6768), which the controls hold.
//
// Measured 2026-11-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f -c`) under
// `env -i PATH=/usr/bin:/bin`, in a directory holding one file `q`; every
// `want` is that shell's output with both streams on one pipe.
func TestADeclarationsWordsAreMatchedInTurnFromAnAssignment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		// The issue's rows.
		{
			"an ordinary command",
			"echo q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\nq\n",
		},
		{
			"a declaration with no assignment operand",
			"typeset q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\n",
		},
		{
			"behind a scalar operand",
			"typeset y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"behind an array-literal operand",
			"typeset -a a=(1) q*(e:'print -u2 Q':) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"a refusal leaves a later substitution unrun",
			"typeset y=1 *nomatch* $(echo S >&2)",
			"zsh:1: no matches found: *nomatch*\n",
		},
		{
			"a refusal leaves a later division unrun",
			"typeset y=1 *nomatch* $((1/0))",
			"zsh:1: no matches found: *nomatch*\n",
		},

		// What decides: an assignment operand reached, not one in front of
		// the pattern. The words held before it are matched at it.
		{
			"between two operands",
			"typeset y=1 q*(e:'print -u2 Q':) z=$(echo A >&2)",
			"Q\nA\n",
		},
		{
			"an operand after the pattern, then a word",
			"typeset q*(e:'print -u2 Q':) y=1 $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"an operand after the pattern and the word",
			"typeset q*(e:'print -u2 Q':) $(echo A >&2) y=1",
			"A\nQ\n",
		},
		{
			"matched before the operand's own value",
			"typeset q*(e:'print -u2 Q':) $(echo A >&2) y=$(echo Y >&2)",
			"A\nQ\nY\n",
		},
		{
			"matched before the prefix",
			"x=$(echo P >&2) typeset q*(e:'print -u2 Q':) y=$(echo A >&2)",
			"Q\nP\nA\n",
		},
		{
			"an array-literal operand after the pattern",
			"typeset -a q*(e:'print -u2 Q':) a=(1) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"each word before the next",
			"typeset y=1 q*(e:'print -u2 Q1':) $(echo A >&2) q*(e:'print -u2 Q2':) $(echo B >&2)",
			"Q1\nA\nQ2\nB\nq=''\n",
		},
		{
			"local",
			"f() { local y=1 q*(e:'print -u2 Q':) $(echo A >&2); }; f",
			"Q\nA\n",
		},
		{
			"export",
			"export y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"readonly",
			"readonly y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"an option in front",
			"typeset -x y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"Q\nA\n",
		},
		{
			"a refusal at the operand leaves the words behind it unrun",
			"typeset *nomatch* y=1 $(echo S >&2)",
			"zsh:1: no matches found: *nomatch*\n",
		},
		{
			"a pattern that may match nothing refuses nothing",
			"typeset y=1 *nomatch*(N) $(echo S >&2)",
			"S\n",
		},

		// The controls: the whole list first.
		{
			"a plain name in front",
			"typeset y q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\n",
		},
		{
			"export with no assignment operand",
			"export q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\n",
		},
		{
			"builtin typeset",
			"builtin typeset y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\n",
		},
		{
			"an escaped name",
			"\\typeset y=1 q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\n",
		},
		{
			"a refusal ahead of the only operand, last",
			"typeset *nomatch* $(echo S >&2) y=1",
			"S\nzsh:1: no matches found: *nomatch*\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "q"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			out, _ := runZsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("%s\nout  %q\nwant %q", tc.src, out, tc.want)
			}
		})
	}
}

// The status a refusal behind an assignment operand keeps is the one in front
// of the command, because the substitution after it never runs. Measured as
// above, from `-c` and from a script file.
func TestARefusalBehindADeclarationsAssignmentRunsNoLaterWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src       string
		wantC, wantFile int
	}{
		{"behind a scalar operand", "(exit 3); typeset y=1 *nomatch* $(exit 6)", 3, 3},
		{"behind an array-literal operand", "(exit 3); typeset -a a=(1) *nomatch* $(exit 6)", 3, 3},
		{"ahead of the operand", "(exit 3); typeset *nomatch* y=1 $(exit 6)", 1, 1},
		{"a division never reached", "typeset y=1 *nomatch* $((1/0))", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, st := runZshRoute(t, t.TempDir(), tc.src, interp.RouteCommandString); st != tc.wantC {
				t.Errorf("%s from a command string = %d, want %d", tc.src, st, tc.wantC)
			}
			if _, st := runZshRoute(t, t.TempDir(), tc.src, interp.RouteScriptFile); st != tc.wantFile {
				t.Errorf("%s from a script file = %d, want %d", tc.src, st, tc.wantFile)
			}
		})
	}
}
