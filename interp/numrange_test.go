// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// numRangeDialect is the core grammar with the one flag under test, and it is
// handed to the parser and to the runner alike: the parser decides that `<->`
// is a word and the matcher decides what it matches, and they read the same
// answer.
func numRangeDialect() syntax.Dialect {
	d := syntax.Core()
	d.NumericRangePattern = true
	return d
}

// runRange runs src with numeric ranges on, in dir.
func runRange(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	return runRangeSem(t, dir, src, nil)
}

// runRangeSem is runRange with one semantics axis moved, for the question
// that is not the dialect flag's.
func runRangeSem(t *testing.T, dir, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	d := numRangeDialect()
	return runGrammar(t, src, func(g *syntax.Dialect) { g.NumericRangePattern = true },
		func(r *Runner) {
			r.Dialect = &d
			if dir != "" {
				r.Dir = dir
			}
			if set != nil {
				sem := *r.Semantics
				set(&sem)
				r.Semantics = &sem
			}
		})
}

// TestANumericRangeMatchesTheValueAndNotTheText — the whole of what a range
// is: a run of digits whose *number* falls between the bounds. Every line is
// a measurement from zsh 5.9.2, 2026-09-05.
func TestANumericRangeMatchesTheValueAndNotTheText(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Any number, and only a number.
		{`[[ 1 = <-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 12345 = <-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ x = <-> ]] && echo hit || echo miss`, "miss\n"},
		{`[[ "" = <-> ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 1x = <-> ]] && echo hit || echo miss`, "miss\n"},
		// A leading `-` is not part of a number here: the digits are the
		// run, and the sign is a character the pattern has no term for.
		{`[[ -1 = <-> ]] && echo hit || echo miss`, "miss\n"},
		// The three bounded shapes, on both sides of each bound.
		{`[[ 42 = <1-100> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 200 = <1-100> ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 5 = <5-5> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 5 = <6-> ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 6 = <6-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 5 = <-4> ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 4 = <-4> ]] && echo hit || echo miss`, "hit\n"},
		// Leading zeros belong to the run and not to the number.
		{`[[ 007 = <1-10> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 007 = <8-10> ]] && echo hit || echo miss`, "miss\n"},
		// The length of the run is decided by what follows it, so the
		// matcher has to be able to stop short.
		{`[[ 100 = <1-10>0 ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 12 = <-><-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ x1 = x<-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 1 = x<-> ]] && echo hit || echo miss`, "miss\n"},
		// A subject too large to hold is above every upper bound and below
		// no lower one, which is the answer zsh gives.
		{`[[ 99999999999999999999 = <-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 99999999999999999999 = <1-> ]] && echo hit || echo miss`, "hit\n"},
		{`[[ 99999999999999999999 = <1-5> ]] && echo hit || echo miss`, "miss\n"},
		// And a *bound* too large is not a range at all, so the text stays
		// literal and matches nothing. zsh refuses the same match with a
		// diagnostic about the nineteenth digit.
		{`[[ 1 = <1-99999999999999999999> ]] && echo hit || echo miss`, "miss\n"},
		// A reversed range matches nothing rather than being an error.
		{`[[ 3 = <5-1> ]] && echo hit || echo miss`, "miss\n"},
		// A `case` arm reads the same pattern.
		{`case 42 in <->) echo num;; *) echo other;; esac`, "num\n"},
		{`case abc in <->) echo num;; *) echo other;; esac`, "other\n"},
		{`case 42 in <1-9>) echo small;; <10-100>) echo big;; esac`, "big\n"},
	} {
		if got, _ := runRange(t, "", tc.src); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestQuotingDecidesWhetherARangeIsARange — the same per-span rule that
// decides whether `a*` is a pattern, which is why the matcher is handed
// escaped text rather than a plain string.
func TestQuotingDecidesWhetherARangeIsARange(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ 1 = "<->" ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 1 = '<->' ]] && echo hit || echo miss`, "miss\n"},
		{`[[ 1 = \<-\> ]] && echo hit || echo miss`, "miss\n"},
		// The literal text is what a quoted range matches.
		{`[[ "<->" = "<->" ]] && echo hit || echo miss`, "hit\n"},
	} {
		if got, _ := runRange(t, "", tc.src); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
	// A range that arrived from an expansion is a separate question, and it
	// belongs to a *semantics* axis rather than to this flag: whether the
	// result of an expansion is read as a pattern at all. The shell that has
	// ranges answers no, so `p="<->"; [[ 1 = $p ]]` does not match there —
	// measured — while the same text under the other answer does. Both are
	// asserted, because a matcher that ignored the escaping would give the
	// first one `hit` and look right in every other test here.
	const fromAVariable = `p="<->"; [[ 1 = $p ]] && echo hit || echo miss`
	if got, _ := runRangeSem(t, "", fromAVariable, func(s *Semantics) { s.GlobExpansionResults = No }); got != "miss\n" {
		t.Errorf("%s: got %q, want %q with the expansion not re-read", fromAVariable, got, "miss\n")
	}
	if got, _ := runRangeSem(t, "", fromAVariable, func(s *Semantics) { s.GlobExpansionResults = Yes }); got != "hit\n" {
		t.Errorf("%s: got %q, want %q with the expansion re-read", fromAVariable, got, "hit\n")
	}
	// And the matcher follows the dialect flag rather than reading a range
	// wherever one is written: with the flag off and the expansion re-read
	// as a pattern, `<->` is four ordinary characters. This is the only
	// route that can ask the question, because a dialect without the flag
	// has no grammar that would let a bare `<->` reach a pattern at all.
	off, _ := runGrammar(t, fromAVariable, nil, func(r *Runner) {
		d := syntax.Core()
		r.Dialect = &d
		sem := *r.Semantics
		sem.GlobExpansionResults = Yes
		r.Semantics = &sem
	})
	if off != "miss\n" {
		t.Errorf("%s: got %q without the flag, want %q", fromAVariable, off, "miss\n")
	}
}

// TestANumericRangeNamesNumberedFiles — against the filesystem it is a
// pattern like any other: matched per component, sorted with the rest, and a
// miss handled by whatever the dialect does with an unmatched pattern.
func TestANumericRangeNamesNumberedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"0", "1", "2", "10", "007", "21", "22", "2x", "abc"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{`echo <->`, "0 007 1 10 2 21 22\n"},
		{`echo <1-5>`, "1 2\n"},
		{`echo <2->`, "007 10 2 21 22\n"},
		{`echo <-1>`, "0 1\n"},
		// The digits in front are part of the word rather than a
		// descriptor, so this is a pattern and not a redirection.
		{`echo 2<->`, "21 22\n"},
		// And a quoted one is a word that matches nothing, so it is passed
		// through the way any unmatched pattern is here.
		{`echo "<->"`, "<->\n"},
	} {
		if got, st := runRange(t, dir, tc.src); got != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q status 0", tc.src, got, st, tc.want)
		}
	}
}
