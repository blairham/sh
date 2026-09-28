// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestFunctionNestingIsTheParameterAndItsWords — the four facts #4905 asked
// for together, because three of them without the fourth is a wrong answer
// where an absent parameter is merely a missing one.
//
// Every row below is byte-identical to `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, which `go version -m` calls *not a Go
// executable* where it calls ours `github.com/blairham/sh/cmd/zsh` — measured
// 2026-09-27 from script files under `env -i PATH=/usr/bin:/bin` with a
// scratch `HOME`, both shells in the same run.
func TestFunctionNestingIsTheParameterAndItsWords(t *testing.T) {
	const refusal = "maximum nested function level reached; increase FUNCNEST?"
	for _, tc := range []struct{ name, src, want string }{
		// The parameter itself: the shell's own integer, base ten written
		// down, at the reference's number. Before this it had no parameter
		// at all and `${+FUNCNEST}` was 0.
		{
			"the parameter",
			`print -r -- "${(t)FUNCNEST} ${+FUNCNEST}"; typeset -p FUNCNEST`,
			"integer-special 1\ntypeset -i10 FUNCNEST=500\n",
		},
		// The bound *being* that parameter, which is the fact a stored value
		// with no reader would not have: the body reaches exactly the bound
		// and the call past it is refused.
		{
			"the bound is the parameter",
			"typeset -g d=0\nf() { d=$((d+1)); print -r -- \"d=$d\"; f; }\nFUNCNEST=3\nf",
			"d=1\nd=2\nd=3\nf: " + refusal + "\n",
		},
		// The sentence, which names the parameter rather than the count —
		// and names the *function* in the location rather than in itself.
		{
			"the callee is in the location and the count is nowhere",
			"g() { h; }\nh() { g; }\nFUNCNEST=2\ng",
			"g: " + refusal + "\n",
		},
		// The default bound is reachable, which it was not while the
		// substrate's own ceiling sat below it: this is the row #4905 quoted,
		// and it answered `f: f: too deeply nested` here.
		{
			"the default bound answers the runaway",
			"f() { f; }\nf",
			"f: " + refusal + "\n",
		},
		// And the refusal ends the script: the `||` is not taken and the next
		// line never runs.
		{
			"the refusal ends the script",
			"f() { f; }\nFUNCNEST=2\nf || print -r -- or\nprint -r -- AFTER",
			"f: " + refusal + "\n",
		},
		// A zero is a bound of zero here and is "nothing said" in bash, which
		// is the one row the two columns that read this name part over. The
		// line is the caller's because the callee was never entered.
		{
			"zero admits no call at all",
			"f() { print -r -- in; }\nFUNCNEST=0\nf\nprint -r -- AFTER",
			"f:3: " + refusal + "\n",
		},
		// The integer attribute is what folds an empty value and a word into
		// that row rather than leaving them unreadable: both are stored as 0
		// before the bound is ever asked.
		{
			"a word is stored as zero and is that same bound",
			"f() { print -r -- in; }\nFUNCNEST=abc\nf\nprint -r -- AFTER",
			"f:3: " + refusal + "\n",
		},
		// And a negative is no bound in both columns — the control that says
		// this is not simply "every unreadable value bounds at zero". The
		// reference segmentation faults on a runaway here; this shell falls
		// through to its own ceiling, which is the one place the row is
		// deliberately not copied, so the function has a base case.
		{
			"a negative is no bound",
			"typeset -g d=0\nf() { d=$((d+1)); if (( d > 4 )); then return; fi; f; }\nFUNCNEST=-1\nf\nprint -r -- \"d=$d\"",
			"d=5\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// The instrument's own control: a name this shell does not have answers
	// `${+}` of 0 in the same harness, so the first row above is not a
	// listing that reports everything as present.
	if out, _ := runZsh(t, t.TempDir(), `print -r -- "${+NOSUCHBOUND}"`); strings.TrimSpace(out) != "0" {
		t.Errorf("control: got %q, want 0", out)
	}
}
