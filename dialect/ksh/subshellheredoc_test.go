// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// This is the column that makes a subshell's here-document body a second axis
// rather than a second reading of a command's. Here the body a `( … )` is fed
// is expanded **out here**, so what it writes is still set afterwards — and
// the same body on a command of its own loses it.
//
// Measured 2026-09-26 on `/bin/ksh` — `Version AJM 93u+ 2012-08-01`, AT&T's
// build and not ksh93u+m, `not a Go executable` by `go version -m` — over a
// script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME, `n=0`
// before and `echo "n=[$n]"` after a body of `$(( n+=5 ))`:
//
//	`<<END` on …                          n afterwards
//	( cat )         a subshell            5
//	( : ; cat )                           5
//	( v=1; cat )                          5
//	( ( cat ) )                           5
//	{ cat; }        a group               5
//	cat             a command of its own  0
//	cat <<END | cat a pipeline element    0
//	cat <<END &     a background command  0
//
// The last three rows are what say this is **not** "wherever the fork has
// happened by now": this shell forks for all three and confines all three,
// and keeps only what the parentheses carry (#4700).
func TestASubshellsHeredocBodyIsExpandedOutHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cmd, want string }{
		{"a subshell keeps it", "( /bin/cat )", "n=[5]"},
		{"a subshell of two commands keeps it", "( : ; /bin/cat )", "n=[5]"},
		{"a nested subshell keeps it", "( ( /bin/cat ) )", "n=[5]"},
		{"a group keeps it", "{ /bin/cat; }", "n=[5]"},
		{"a command of its own loses it", "/bin/cat", "n=[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "n=0\n" + tc.cmd + " <<END\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n"
			out, _ := runKsh(t, t.TempDir(), src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s: out = %q, want %q", tc.cmd, out, tc.want)
			}
			if !strings.Contains(out, "5\n") {
				t.Errorf("%s: out = %q, want the expanded body to have reached the command", tc.cmd, out)
			}
		})
	}
}

// The rows that refute "wherever the fork has happened", held against the
// subshell row above: this shell confines the same body on a pipeline element
// and on a background command, both of which it forks for.
func TestAForkIsNotWhatDecidesItHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a pipeline element", "n=0\n/bin/cat <<END | /bin/cat\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n"},
		{"a background command", "n=0\n/bin/cat <<END &\n$(( n+=5 ))\nEND\nwait\necho \"n=[$n]\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), tc.src)
			if !strings.Contains(out, "n=[0]") {
				t.Errorf("out = %q, want the write confined", out)
			}
			// And the body really was expanded and really did reach the
			// command, so "confined" is not "never expanded".
			if !strings.Contains(out, "5\n") {
				t.Errorf("out = %q, want the expanded body to have reached the command", out)
			}
		})
	}
}

// And the subshell's own state is still confined, which is what keeps the row
// above from reading as "this shell does not isolate parentheses at all".
func TestASubshellStillConfinesItsOwnAssignmentsWithAHeredocHere(t *testing.T) {
	t.Parallel()
	out, _ := runKsh(t, t.TempDir(), "n=0\n( v=1; /bin/cat ) <<END\n$(( n+=5 ))\nEND\necho \"n=[$n] v=[${v-unset}]\"\n")
	if !strings.Contains(out, "n=[5] v=[unset]") {
		t.Errorf("out = %q, want the body's write kept and the subshell's confined", out)
	}
}

// The failure side is the same in this column whichever way the axis goes,
// which is why the axis is not asked there: a body that will not expand costs
// the parentheses, the rest of the line runs and `||` catches it.
func TestASubshellsFailedHeredocBodyIsContainedHere(t *testing.T) {
	t.Parallel()
	out, st := runKsh(t, t.TempDir(), "( echo RAN ) <<END ; echo SAME\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if strings.Contains(out, "RAN") {
		t.Errorf("out = %q, want the subshell left unrun", out)
	}
	if !strings.Contains(out, "SAME") || st != 0 {
		t.Errorf("out = %q (status %d), want the rest of the line run", out, st)
	}
	caught, _ := runKsh(t, t.TempDir(), "( echo RAN ) <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n")
	if !strings.Contains(caught, "CAUGHT") {
		t.Errorf("out = %q, want the failure caught", caught)
	}
}

func TestKshSaysASubshellsHeredocBodyDoesNotExpandInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := ksh.Semantics().HeredocBodyOnASubshellExpandsInTheSubshell, interp.No; got != want {
		t.Errorf("HeredocBodyOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
