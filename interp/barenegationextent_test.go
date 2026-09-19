// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A bare negation reached through text the runner reads for itself — an
// `eval`, a substitution body, a process substitution — used to take the
// shell down rather than run.
//
// The shape is a `!` that the *text* ends at, with no separator behind it, so
// the statement has no `;` to be asked for instead and the runner asks the
// expression where it ended. A `!` written as a whole line of a script never
// reached it, which is what kept it hidden: the newline is the statement's
// terminator and answers first.
//
// Nothing runs and the negation inverts a success, so the status is 1 —
// [syntax.BareNegationReach] is where a dialect takes one at all, and every
// row below is the reach's own answer arriving through a nested reader rather
// than a new rule. Measured 2026-09-19 against the three columns that take a
// bare `!`: `eval "!"` is status 1 and the script goes on in all three.
func TestABareNegationInTextTheRunnerReads(t *testing.T) {
	bare := func(d *syntax.Dialect) {
		d.BareNegationReach = syntax.BareNegationAtEitherPlace
		d.RepeatedNegationToggles = true
		d.ProcessSubstitution = true
	}
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"an eval", `eval "!"; echo "st=$?"; echo end`, "st=1\nend\n", 0},
		{"a substitution body", `v=$(!); echo "v=[$v] st=$?"; echo end`, "v=[] st=1\nend\n", 0},
		{"a substitution body behind a command", `v=$(: ; !); echo "st=$?"`, "st=1\n", 0},
		{"the older spelling", "v=`!`; echo \"v=[$v] st=$?\"", "v=[] st=1\n", 0},
		{"an even run, which negates nothing", `v=$(! !); echo "st=$?"`, "st=0\n", 0},
		{"a bare negation the whole text is", `eval "! !"; echo "st=$?"`, "st=0\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, bare, func(r *Runner) {
				d := syntax.Core()
				bare(&d)
				r.Dialect = &d
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
		})
	}
}
