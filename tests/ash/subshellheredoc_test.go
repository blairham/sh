// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// This column and dash are the pair that makes a subshell's here-document
// body a second axis: here a body fed to a **command of its own** leaves its
// write behind, and a body fed to a `( … )` does not. One field cannot say
// both, and ksh93 is the pair read the other way round.
//
// Measured 2026-09-26 in the digest-pinned alpine image
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`
// — `BusyBox v1.37.0 (2026-01-10 15:38:28 UTC) multi-call binary` — over a
// script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME, `n=0`
// before and `echo "n=[$n]"` after a body of `$(( n+=5 ))`:
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
		{"a subshell loses it", "( cat )", "n=[0]"},
		{"a group keeps it", "{ cat; }", "n=[5]"},
		{"a command of its own keeps it", "cat", "n=[5]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "n=0\n" + tc.cmd + " <<END\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n"
			out, _ := runIn(t, src)
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
// same body, the same day, on a subshell and on a command of its own.
func TestASubshellAndACommandPartHere(t *testing.T) {
	t.Parallel()
	const src = "unset u\n%s <<END\n${u:=made}\nEND\necho \"u=[${u-unset}]\"\n"
	sub, _ := runIn(t, strings.Replace(src, "%s", "( cat )", 1))
	cmd, _ := runIn(t, strings.Replace(src, "%s", "cat", 1))
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
	out, _ := runIn(t, "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
	if !strings.Contains(out, "u=[made]") {
		t.Errorf("out = %q, want the target's write kept", out)
	}
}

// A quoted delimiter expands nothing, so there is no write to place — the
// control that must not move.
func TestAQuotedDelimiterOnASubshellIsLiteralHere(t *testing.T) {
	t.Parallel()
	out, _ := runIn(t, "n=0\n( cat ) <<'END'\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n")
	if !strings.Contains(out, "$(( n+=5 ))") || !strings.Contains(out, "n=[0]") {
		t.Errorf("out = %q, want the body literal and no write", out)
	}
}

func TestAshSaysASubshellsHeredocBodyExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := ash.Semantics().HeredocBodyOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("HeredocBodyOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
