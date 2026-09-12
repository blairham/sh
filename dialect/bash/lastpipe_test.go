// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `shopt -s lastpipe` is the one name in this builtin that a script sets and
// then immediately depends on, which is why it could not be closed by making
// the refusal quiet: a script turns it on so that `cmd | read v` leaves `v`
// set, and a shell that accepted the word without moving the pipeline would
// hand it an empty variable with nothing on standard error (#2361).
//
// Measured against bash 5.3.15 on 2026-09-12, under `-c` and from a script
// file alike. The other columns are in the corpus, where the split is the
// point: zsh 5.9.2 and ksh93 answer `[hi]` with no option to set, dash and
// bash 3.2.57 answer `[]` with no option either, and only bash 5.3 moves.
func TestLastpipeRunsTheLastElementInThisShell(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The issue's own repro.
		{`shopt -s lastpipe; set +m; echo hi | read x; echo "[$x]"`, "[hi]\n"},
		// Off with nothing said: this dialect's answer for the axis is a
		// subshell, so the pair is what says the option did the work.
		{`echo hi | read x; echo "[$x]"`, "[]\n"},
		// A live switch rather than a door, the way `expand_aliases` is.
		{`shopt -s lastpipe; shopt -u lastpipe; echo hi | read x; echo "[$x]"`, "[]\n"},
		// The loop the option exists for: the body's assignments outlive the
		// pipe, which is the single most cited surprise in shell scripting.
		{
			`shopt -s lastpipe
printf 'a\nb\nc\n' | while read l; do n=$((n+1)); done
echo "n=[$n]"`,
			"n=[3]\n",
		},
		// A brace group is observable for the same reason a builtin is.
		{`shopt -s lastpipe; echo hi | { read x; }; echo "[$x]"`, "[hi]\n"},
		// Only the *last* element. `read` in the middle still gets a subshell
		// whatever the option says, which is what parts an option from a
		// shell that stopped piping. Measured on bash 5.3.15 with `:` in the
		// last position as well as with `cat`, and both answer `[]`.
		{`shopt -s lastpipe; echo hi | read x | :; echo "[$x]"`, "[]\n"},
		// The monitor overrules it, which is the half bash ties the option to
		// and the reason it reads as a no-op in an interactive session.
		{`shopt -s lastpipe; set -m; echo hi | read x; echo "[$x]"`, "[]\n"},
		// And the monitor is read at the pipeline rather than when the option
		// was written: turning it on and off again restores the option's
		// effect with no second `shopt`.
		{`shopt -s lastpipe; set -m; set +m; echo hi | read x; echo "[$x]"`, "[hi]\n"},
		// A subshell keeps its own copy of the option.
		{`( shopt -s lastpipe; echo hi | read x; echo "in[$x]" ); echo "out[$x]"`, "in[hi]\nout[]\n"},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%q = %q status %d, want %q status 0", tc.src, out, st, tc.want)
		}
	}
}

// The option reads back through every face of the builtin, which is what an
// agent harness sources when it captures a shell with `shopt -p`.
func TestLastpipeReadsBack(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`shopt lastpipe`, "lastpipe            \toff\n", 1},
		{`shopt -s lastpipe; shopt lastpipe`, "lastpipe            \ton\n", 0},
		{`shopt -p lastpipe`, "shopt -u lastpipe\n", 1},
		{`shopt -s lastpipe; shopt -p lastpipe`, "shopt -s lastpipe\n", 0},
		{`shopt -q lastpipe`, "", 1},
		{`shopt -s lastpipe; shopt -q lastpipe`, "", 0},
		// It is not a `set -o` name, in either direction. Measured: bash
		// answers `invalid option name` for both spellings.
		{`shopt -o lastpipe`, "bash: line 1: shopt: lastpipe: invalid option name\n", 1},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// The sharpest consequence of running the last element here, and the one a
// script can be surprised by: `exit` in that position ends the shell rather
// than a subshell. Measured on bash 5.3.15 — `shopt -s lastpipe; echo a |
// exit 3; echo after` prints nothing and exits 3.
func TestLastpipeLetsTheLastElementEndTheShell(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `shopt -s lastpipe; echo a | exit 3; echo "after=$?"`)
	if out != "" || st != 3 {
		t.Errorf("got %q status %d, want %q status 3", out, st, "")
	}
}
