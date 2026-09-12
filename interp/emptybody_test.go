// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A body with nothing in it succeeds rather than leaving the status the
// command before it set.
//
// Not an axis on Semantics and deliberately not: an axis records a
// *disagreement* about identical syntax, and only two panel columns can write
// an empty body at all — the other four refuse the line while parsing. Both
// that can answer 0, measured 2026-09-12 with `env -i PATH=/usr/bin:/bin` and
// a scratch HOME.
//
// The failure in front of each row is what makes it a measurement. A body the
// interpreter simply skipped would answer 1 here, and every row would pass
// against a shell that never touched `$?`.
func TestABodyWithNothingInItSucceeds(t *testing.T) {
	empty := func(d *syntax.Dialect) {
		d.EmptyCompoundBody = true
		d.CloseBraceAlwaysReserved = true
	}
	for _, src := range []string{
		"false; { }; echo st=$?",
		"false; ( ); echo st=$?",
		"false; if :; then fi; echo st=$?",
		"false; while :; do break; done; echo st=$?",
		"false; for i in a; do done; echo st=$?",
		"false; x() { }; x; echo st=$?",
	} {
		out, st := runGrammar(t, src, empty, nil)
		if out != "st=0\n" || st != 0 {
			t.Errorf("%q: out = %q (status %d), want %q at 0", src, out, st, "st=0\n")
		}
	}
}

// The rows that were already right, and the reason the change belongs where a
// list runs rather than in each construct: a body that is never *entered* has
// its own answer and it was already 0 in all six columns.
func TestABodyThatIsNeverEnteredStillSucceeds(t *testing.T) {
	for _, src := range []string{
		"false; case x in y) echo no;; esac; echo st=$?",
		"false; if false; then echo no; fi; echo st=$?",
		"false; while false; do echo no; done; echo st=$?",
		"false; for i in; do echo no; done; echo st=$?",
		"false; case x in x) ;; esac; echo st=$?",
	} {
		out, st := runGrammar(t, src, nil, nil)
		if out != "st=0\n" || st != 0 {
			t.Errorf("%q: out = %q (status %d), want %q at 0", src, out, st, "st=0\n")
		}
	}
}
