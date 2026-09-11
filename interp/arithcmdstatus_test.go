// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ArithCommandErrorStatusIsTwo is the status `(( ))` leaves behind when the
// expression could not be evaluated, and it is the construct's question rather
// than the evaluator's: `let` on the same text is 1 under either answer.
//
// Both ways an expression can fail are here, because the axis covers both: an
// expression the arithmetic parser cannot read at all and one it reads and
// then cannot evaluate. A test with only the first would pass for a shell that
// answered the axis on the parse branch and left the other at 1, which is the
// shape the fix had to reach — the parse branch took the dialect's syntax
// status and never asked (#1625).
//
// The syntax status is pinned at 1 so that the two answers differ *here*. The
// POSIX preset's own is 2, which would have made the No case answer 2 on the
// parse branch by a route that has nothing to do with this axis, and a reader
// could not tell which of the two had produced it.
//
// `let` is in both rows for the same reason it decided where the axis lives:
// the same unreadable text through the builtin is 1 in every shell measured,
// so a status hung on arithmetic failure rather than on the construct would
// move this line too.
func TestTheStatusAFailedArithmeticCommandLeaves(t *testing.T) {
	const src = `(( 1+ )); echo "parse=$?"
(( 1/0 )); echo "eval=$?"
let "1+"; echo "let=$?"
(( 1 )); echo "true=$?"
(( 0 )); echo "false=$?"`

	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"one", No, "parse=1\neval=1\nlet=1\ntrue=0\nfalse=1\n"},
		{"two", Yes, "parse=2\neval=2\nlet=1\ntrue=0\nfalse=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.ArithCommandErrorStatusIsTwo = tc.answer
			diag := Diagnostics{SyntaxErrorStatus: 1}
			out, _ := run(t, src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &diag
				r.Stderr = nil
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// An unanswered axis is refused by name rather than given one shell's answer,
// and the refusal is reached only by a failing expression: an arithmetic
// command that evaluates cleanly never asks.
func TestAnUnansweredArithmeticCommandStatusIsRefused(t *testing.T) {
	sem := PosixSemantics()
	sem.ArithCommandErrorStatusIsTwo = Unspecified
	out, st := run(t, `(( 2 )); echo "ok=$?"`, func(r *Runner) { r.Semantics = &sem })
	if out != "ok=0\n" {
		t.Errorf("a clean expression got %q, want it never to have asked", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}

	out, st = run(t, `(( 1+ ))`, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "the status a failed `(( ))` leaves") || st != 2 {
		t.Errorf("got %q (status %d), want the unanswered axis named at 2", out, st)
	}
}
