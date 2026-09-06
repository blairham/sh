// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a loop that names more than one variable binds them to. The grammar
// half is syntax/formultiplenames_test.go; this is the stride, the ragged
// last pass, and what a trace says.

// runManyNames parses with the name list and the short spellings on, which is
// the combination one shell has, and runs it.
func runManyNames(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ForMultipleNames = true
		d.ForBraceBody = true
		d.ShortForm = true
		d.CloseBraceAlwaysReserved = true
		d.Foreach = true
	}, nil)
}

// The headline, and the shape a script walks a serialized key/value table
// with: two names take two words a pass.
func TestALoopWithTwoNamesTakesTwoWordsAPass(t *testing.T) {
	out, st := runManyNames(t, `for key value ( a 1 b 2 ) { echo "$key=$value" }`)
	if want := "a=1\nb=2\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The stride is the name count and not two: three names take three.
func TestTheStrideIsTheNameCount(t *testing.T) {
	out, st := runManyNames(t, `for a b c ( 1 2 3 4 5 6 ) { echo "[$a][$b][$c]" }`)
	if want := "[1][2][3]\n[4][5][6]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A last pass with fewer words than names still runs, and the names it did not
// reach are **empty rather than unset** — `${b-U}` is `[]` and not `U`.
// Measured in zsh 5.9.2. Stopping the loop instead, or leaving those names
// holding the previous pass's word, are the two wrong answers that look right
// on a list whose length divides.
func TestAShortLastPassLeavesTheRemainingNamesEmpty(t *testing.T) {
	out, st := runManyNames(t,
		`for a b ( 1 2 3 ) { echo "[${a-U}][${b-U}]" }; echo "after=[${a-U}][${b-U}]"`)
	if want := "[1][2]\n[3][]\nafter=[3][]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// An empty list runs the body no times at all, which is the boundary the rule
// above could have been written past: "run a last short pass" must not mean
// "run one pass over nothing".
func TestAnEmptyListRunsTheBodyNoTimes(t *testing.T) {
	out, st := runManyNames(t, `for a b ( ) { echo ran }; echo done`)
	if want := "done\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// With no list at all the loop walks the positional parameters, in groups of
// the same size — the absent list and the empty one stay different questions.
func TestNoListWalksThePositionalParametersInGroups(t *testing.T) {
	out, st := runManyNames(t, `set -- p q r s; for a b; do echo "[$a][$b]"; done`)
	if want := "[p][q]\n[r][s]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The names keep their last values after the loop, exactly as one name does.
func TestTheNamesKeepTheirLastValuesAfterTheLoop(t *testing.T) {
	out, st := runManyNames(t, `for a b ( 1 2 3 4 ) { : }; echo "[$a][$b]"`)
	if want := "[3][4]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The same name twice is not refused and not special: each is written in turn,
// so the last one wins the pass. Measured `[2]` then `[4]` in zsh.
func TestARepeatedNameTakesTheLastWordOfThePass(t *testing.T) {
	out, st := runManyNames(t, `for a a ( 1 2 3 4 ) { echo "[$a]" }`)
	if want := "[2]\n[4]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// `break` leaves the loop from a pass that bound several names, and the loop's
// status is still the body's.
func TestBreakLeavesALoopWithSeveralNames(t *testing.T) {
	out, st := runManyNames(t, `for a b ( 1 2 3 4 ) { echo "[$a]"; break }; echo "st=$?"`)
	if want := "[1]\nst=0\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A trace writes one assignment per name and then the body — measured
// `a=1`, `b=2`, `:`, `a=3`, `b=4`, `:`. The shell that quotes the header
// instead writes it once a pass however many names were bound, which is why
// the two are not the same call repeated.
func TestATraceWritesOneLinePerNameBound(t *testing.T) {
	out, st := runGrammar(t, `set -x; for a b ( 1 2 3 4 ) { : }`,
		func(d *syntax.Dialect) {
			d.ForMultipleNames = true
			d.ForBraceBody = true
			d.ShortForm = true
			d.CloseBraceAlwaysReserved = true
		},
		func(r *interp.Runner) {
			r.Diagnostics = &interp.Diagnostics{TraceForHeader: interp.TraceForAssign}
		})
	if want := "+ a=1\n+ b=2\n+ :\n+ a=3\n+ b=4\n+ :\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The header wording writes one line a pass and not one a name, so a dialect
// that quotes the source does not repeat itself.
func TestATraceThatQuotesTheHeaderWritesItOncePerPass(t *testing.T) {
	out, st := runGrammar(t, `set -x; for a b ( 1 2 3 4 ) { : }`,
		func(d *syntax.Dialect) {
			d.ForMultipleNames = true
			d.ForBraceBody = true
			d.ShortForm = true
			d.CloseBraceAlwaysReserved = true
		},
		func(r *interp.Runner) {
			r.Diagnostics = &interp.Diagnostics{TraceForHeader: interp.TraceForSource}
		})
	if want := "+ for a b ( 1 2 3 4 )\n+ :\n+ for a b ( 1 2 3 4 )\n+ :\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
