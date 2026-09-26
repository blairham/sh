// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A redirection written on a `( … )` of its own leaves a number one column
// answers differently from every other command's.
//
// The number a failed redirection reports is
// [Diagnostics.RedirectFailureStatus], and it is the answer for a command of
// its own, a group, a loop and a redirection written *inside* the
// parentheses. On the subshell's own redirection one column leaves its fatal
// number instead — see [Diagnostics.SubshellRedirectFailureStatus] for the
// nine rows it is read off, measured in the digest-pinned alpine image on
// 2026-09-26, and for the four controls that say the noun is the subshell's
// own redirection (#4716).
//
// The axis is asked of a failed **open** and a failed **expansion** alike,
// because the column that has it answers both the same way — `( cat ) <
// nosuch` and `( cat ) <<END` with `$(( 1/0 ))` in its body both leave 2
// there, where `cat <<END` with the same body leaves 1.

// subshellRedirectStatus runs src with the two numbers set apart and answers
// the status it left behind.
func coreDiagnosticsFor(r *Runner) Diagnostics {
	if r.Diagnostics != nil {
		return *r.Diagnostics
	}
	return CoreDiagnostics()
}

func subshellRedirectStatus(t *testing.T, src string) int {
	t.Helper()
	_, st := runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		// A body that will not expand is the redirection's failure, which
		// is the reading that reaches the number at all. The column this is
		// measured on answers it that way.
		s.HeredocBodyFailureIsTheRedirections = Yes
		s.HeredocExpandsInTheCommandsProcess = No
		s.HeredocBodyOnASubshellExpandsInTheSubshell = No
		r.Semantics = &s
		d := coreDiagnosticsFor(r)
		d.RedirectFailureStatus = 1
		d.SubshellRedirectFailureStatus = 2
		r.Diagnostics = &d
	})
	return st
}

func TestASubshellsOwnRedirectionLeavesItsOwnStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"a command of its own", "cat < /nonexistent/x\n", 1},
		{"a group", "{ cat; } < /nonexistent/x\n", 1},
		{"a loop", "while read -r x; do :; done < /nonexistent/x\n", 1},
		{"a redirection inside the parentheses", "( cat < /nonexistent/x )\n", 1},
		{"the subshell's own", "( cat ) < /nonexistent/x\n", 2},
		{"and not about what is inside it", "( : ) < /nonexistent/x\n", 2},
		{"a body that will not expand", "cat <<END\n$(( 1/0 ))\nEND\n", 1},
		{"a body on the subshell's own redirection", "( cat ) <<END\n$(( 1/0 ))\nEND\n", 2},
		{"a body inside the parentheses", "( cat <<END )\n$(( 1/0 ))\nEND\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := subshellRedirectStatus(t, tc.src); got != tc.want {
				t.Errorf("status %d, want %d", got, tc.want)
			}
		})
	}
}

// Unset, the subshell answers what every other command answers: a preset that
// never measured this does not move.
func TestASubshellWithNoNumberOfItsOwnTakesTheRedirectionsStatus(t *testing.T) {
	t.Parallel()
	_, st := runGrammar(t, "( cat ) < /nonexistent/x\n", nil, func(r *Runner) {
		d := coreDiagnosticsFor(r)
		d.RedirectFailureStatus = 1
		r.Diagnostics = &d
	})
	if st != 1 {
		t.Errorf("status %d, want the redirection's 1", st)
	}
}

// The command does not run and the script carries on, in every row — which is
// what says this is the number left behind and not a reach.
func TestTheSubshellsRedirectionStillOnlyCostsTheCommand(t *testing.T) {
	t.Parallel()
	out, _ := runGrammar(t, "( echo RAN ) < /nonexistent/x\necho after\n", nil, func(r *Runner) {
		d := coreDiagnosticsFor(r)
		d.RedirectFailureStatus, d.SubshellRedirectFailureStatus = 1, 2
		r.Diagnostics = &d
	})
	if strings.Contains(out, "RAN") || !strings.Contains(out, "after") {
		t.Errorf("out = %q, want the subshell unrun and the script carried on", out)
	}
}
