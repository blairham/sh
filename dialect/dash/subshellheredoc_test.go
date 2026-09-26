// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// This column and BusyBox ash are the pair that makes a subshell's
// here-document body a second axis: here a body fed to a **command of its
// own** leaves its write behind, and a body fed to a `( … )` does not. One
// field cannot say both, and ksh93 is the pair read the other way round.
//
// Measured 2026-09-26 on `/bin/dash` 0.5.12 — `not a Go executable` by `go
// version -m` — over a script file under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, `n=0` before and `echo "n=[$n]"` after a body of
// `$(( n+=5 ))`:
//
//	`<<END` on …                          n afterwards
//	( cat )         a subshell            0
//	{ cat; }        a group               5
//	cat             a command of its own  5
//
// The middle row is the control that says the parentheses decide, and the
// last is HeredocExpandsInTheCommandsProcess, unchanged by this one (#4700).
func TestASubshellsHeredocBodyIsExpandedInTheSubshellHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cmd, want string }{
		{"a subshell loses it", "( /bin/cat )", "n=[0]"},
		{"a group keeps it", "{ /bin/cat; }", "n=[5]"},
		{"a command of its own keeps it", "/bin/cat", "n=[5]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "n=0\n" + tc.cmd + " <<END\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n"
			out, _ := runDash(t, t.TempDir(), src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s: out = %q, want %q", tc.cmd, out, tc.want)
			}
			if !strings.Contains(out, "5\n") {
				t.Errorf("%s: out = %q, want the expanded body to have reached the command", tc.cmd, out)
			}
		})
	}
}

// The pair that says the two axes are not one, inside this column alone: the
// same body, the same day, on a subshell and on a command of its own, moving
// in opposite directions from the other four columns' reading.
func TestASubshellAndACommandPartHere(t *testing.T) {
	t.Parallel()
	const src = "unset u\n%s <<END\n${u:=made}\nEND\necho \"u=[${u-unset}]\"\n"
	sub, _ := runDash(t, t.TempDir(), strings.Replace(src, "%s", "( /bin/cat )", 1))
	cmd, _ := runDash(t, t.TempDir(), strings.Replace(src, "%s", "/bin/cat", 1))
	if !strings.Contains(sub, "u=[unset]") {
		t.Errorf("a subshell: out = %q, want the write confined", sub)
	}
	if !strings.Contains(cmd, "u=[made]") {
		t.Errorf("a command of its own: out = %q, want the write kept", cmd)
	}
}

// And the *target* goes the other way again, which is what says this is not
// the subshell-target axis either: `( : ) > "${u:=made}"` keeps the write
// here while the body's is lost.
func TestASubshellsTargetAndItsBodyPartHere(t *testing.T) {
	t.Parallel()
	out, _ := runDash(t, t.TempDir(), "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
	if !strings.Contains(out, "u=[made]") {
		t.Errorf("out = %q, want the target's write kept", out)
	}
}

// A quoted delimiter expands nothing, so there is no write to place — the
// control that must not move.
func TestAQuotedDelimiterOnASubshellIsLiteralHere(t *testing.T) {
	t.Parallel()
	out, _ := runDash(t, t.TempDir(), "n=0\n( /bin/cat ) <<'END'\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n")
	if !strings.Contains(out, "$(( n+=5 ))") || !strings.Contains(out, "n=[0]") {
		t.Errorf("out = %q, want the body literal and no write", out)
	}
}

func TestDashSaysASubshellsHeredocBodyExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := dash.Semantics().HeredocBodyOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("HeredocBodyOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
