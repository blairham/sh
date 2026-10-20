// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// This is the column that makes a subshell's target a second axis rather than
// a second reading of the command's. Here the word a `( … )` is aimed at is
// expanded **out here**, so what it assigns is still set afterwards — and the
// same target on a command of its own loses it.
//
// Measured 2026-09-26 on `/bin/ksh` — `Version AJM 93u+ 2012-08-01`, AT&T's
// build and not a Korn-shell lookalike, `not a Go executable` by `go version
// -m` — over a script file under `env -i PATH=/usr/bin:/bin` with a scratch
// HOME:
//
//	`> "${u:=made}"` on …            u afterwards
//	cat /dev/null   a command        unset
//	( : )           a subshell       made
//	( /bin/echo hi )                 made
//	( : ; : )                        made
//	( v=1 )                          made, and v unset
//
// The last row is what says this is about the redirection's word rather than
// about the subshell's state: `v` is confined and `u` is not (#4695).

func TestASubshellsTargetIsExpandedOutHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cmd, want string }{
		{"a subshell keeps it", "( : )", "u=[made]"},
		{"a subshell of an external command keeps it", "( /bin/echo hi )", "u=[made]"},
		{"a command of its own loses it", "cat /dev/null", "u=[unset]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "unset u\n" + tc.cmd + " > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n"
			out, _ := runKsh(t, t.TempDir(), src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// And the subshell's own state is still confined, which is what keeps the row
// above from reading as "this shell does not isolate parentheses at all".
func TestASubshellStillConfinesItsOwnAssignmentsHere(t *testing.T) {
	t.Parallel()
	out, _ := runKsh(t, t.TempDir(), "unset u v\n( v=1 ) > \"${u:=made}\"\necho \"u=[${u-unset}] v=[${v-unset}]\"\n")
	if !strings.Contains(out, "u=[made] v=[unset]") {
		t.Errorf("out = %q, want the target's write kept and the subshell's confined", out)
	}
}

// The failure side of the same answer, and the reason this column is not
// simply "no": the word was expanded out here, so whose failure it is becomes
// [interp.Semantics.RedirectTargetFailureIsTheRedirections] — the
// redirection's, here — and the script carries on at 1 with `||` catching it.
// Measured 2026-09-26 with the marker after a `;` on the redirection's own
// line.
func TestASubshellsFailedTargetIsTheRedirectionsHere(t *testing.T) {
	t.Parallel()
	t.Run("the rest of its own line runs", func(t *testing.T) {
		t.Parallel()
		out, st := runKsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) ; echo SAME\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("out = %q, want the subshell left unrun", out)
		}
		if !strings.Contains(out, "SAME") || st != 0 {
			t.Errorf("out = %q (status %d), want the rest of the line run", out, st)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		t.Parallel()
		out, _ := runKsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) || echo CAUGHT\n")
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
	t.Run("and 1 is left behind", func(t *testing.T) {
		t.Parallel()
		out, st := runKsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 ))\necho \"after st=$?\"\n")
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("out = %q (status %d), want the next line at 1", out, st)
		}
	})
}

func TestKshSaysASubshellsTargetDoesNotExpandInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := ksh.Semantics().RedirectTargetOnASubshellExpandsInTheSubshell, interp.No; got != want {
		t.Errorf("RedirectTargetOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
