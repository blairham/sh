// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Where the answer to Semantics.UnterminatedBracketAfterASubExpression is
// "not a pattern at all", the verdict is reached **before any matching** —
// so it does not depend on the subject.
//
// It used to. The refusal was written from inside the match, and the matcher
// gives up on the first character that misses, so a pattern whose first
// literal already rules the subject out was never read as far as the bracket.
// Worse, spanByLength skips a candidate piece the pattern's edge literals
// could not fill, so for some subjects the matcher is not called at all. That
// prefilter is correct as a prefilter and it is exactly what makes a
// match-time refusal unreliable (#4659).
//
// The shape of this file is the pair: one subject that **reaches** the
// bracket and one that does not, asked of every surface that compiles a
// pattern. A suite that only ever used the first would have passed
// throughout.

// eagerSubSem answers both bracket axes "not a pattern" and reads no
// collating element, which is the pairing this is about: `[[:alpha:]` is the
// only shape here that leaves a bracket open.
func eagerSubSem() Semantics {
	s := permissive()
	s.UnterminatedBracket = BracketBadPattern
	s.UnterminatedBracketAfterASubExpression = BracketBadPattern
	s.CollatingElements = NoCollatingElements
	s.UnterminatedCharacterClass = UnterminatedClassIsOrdinaryCharacters
	return s
}

func TestABracketASubExpressionLeftOpenIsRefusedWhateverTheSubject(t *testing.T) {
	// Every surface that compiles a pattern, with a subject the pattern's
	// first literal rules out — the row the lazy verdict could not produce.
	for _, c := range []struct{ name, src string }{
		{"a prefix trim", `v=zzz; echo "${v#x[[:alpha:]}"`},
		{"a suffix trim", `v=zzz; echo "${v%[[:alpha:]y}"`},
		// The element filter is the fifth surface and it is not here: it
		// needs an axis the permissive core does not answer, so the row
		// would report that absence rather than this one. It is asserted in
		// the dialect package that has the construct.
		{"a case arm", `case zzz in x[[:alpha:]) echo hit;; *) echo miss;; esac`},
		{"a condition operand", `[[ zzz == x[[:alpha:] ]]; echo status=$?`},
		{"a filename pattern", `echo x[[:alpha:]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src+"\necho after", withSem(eagerSubSem()))
			if !strings.Contains(out, "bad pattern") {
				t.Errorf("got %q, want the refusal", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the script to stop at the refusal", out)
			}
		})
	}
}

// The other half of the pair, and it is what says the instrument can fire on
// both sides of the question: the **same pattern** against a subject its
// first character does not rule out was refused before this change too, and
// still is.
func TestTheSameBracketIsStillRefusedWhenTheMatcherReachesIt(t *testing.T) {
	out, _ := run(t, `v=zzz; echo "${v#[[:alpha:]}"`+"\necho after", withSem(eagerSubSem()))
	if !strings.Contains(out, "bad pattern") || strings.Contains(out, "after") {
		t.Errorf("got %q, want the refusal and no line after it", out)
	}
}

// And the controls, which keep the verdict keyed on *this* axis and on a
// bracket that is really left open.
func TestTheEagerVerdictIsKeyedOnTheAxisAndOnTheShape(t *testing.T) {
	t.Run("a closed class is not refused", func(t *testing.T) {
		out, st := run(t, `v=azzz; echo "[${v#[[:alpha:]]}]"`, withSem(eagerSubSem()))
		if out != "[zzz]\n" || st != 0 {
			t.Errorf("got %q status %d, want [zzz] at 0", out, st)
		}
	})
	t.Run("a bracket closed after the class is not refused", func(t *testing.T) {
		out, st := run(t, `v=zzz; echo "[${v#x[[:alpha:]]}]"`, withSem(eagerSubSem()))
		if out != "[zzz]\n" || st != 0 {
			t.Errorf("got %q status %d, want [zzz] at 0", out, st)
		}
	})
	t.Run("an escaped bracket opens nothing", func(t *testing.T) {
		out, st := run(t, `v=zzz; echo "[${v#x\[[:alpha:]}]"`, withSem(eagerSubSem()))
		if out != "[zzz]\n" || st != 0 {
			t.Errorf("got %q status %d, want [zzz] at 0", out, st)
		}
	})
	t.Run("another reading of the axis refuses nothing", func(t *testing.T) {
		s := eagerSubSem()
		s.UnterminatedBracketAfterASubExpression = BracketNoMatch
		out, st := run(t, `v=zzz; echo "[${v#x[[:alpha:]}]"`, withSem(s))
		if out != "[zzz]\n" || st != 0 {
			t.Errorf("got %q status %d, want [zzz] at 0", out, st)
		}
	})
	t.Run("the plain axis alone does not answer this shape", func(t *testing.T) {
		// Only the *neighbor* axis says "bad pattern"; this one does not, so
		// the pattern compiles. Without this row the whole file would pass
		// for an implementation that read one field for both questions.
		s := eagerSubSem()
		s.UnterminatedBracketAfterASubExpression = BracketLiteral
		out, st := run(t, `v=zzz; echo "[${v#x[[:alpha:]}]"`, withSem(s))
		if out != "[zzz]\n" || st != 0 {
			t.Errorf("got %q status %d, want [zzz] at 0", out, st)
		}
	})
}

// A collating delimiter leaves a bracket open only where the dialect reads
// one, which is the pair that says the scan asks the vector rather than
// assuming.
func TestACollatingDelimiterLeavesTheBracketOpenOnlyWhereItIsRead(t *testing.T) {
	reads := eagerSubSem()
	reads.CollatingElements = OneCharacterIsACollatingElement
	out, _ := run(t, `v=zzz; echo "[${v#x[[.a.]}]"`+"\necho after", withSem(reads))
	if !strings.Contains(out, "bad pattern") || strings.Contains(out, "after") {
		t.Errorf("reading collating elements: got %q, want the refusal", out)
	}
	// And where none is read the same four characters close the bracket at
	// the `]` they can see, so there is nothing to refuse.
	out, st := run(t, `v=zzz; echo "[${v#x[[.a.]}]"`, withSem(eagerSubSem()))
	if out != "[zzz]\n" || st != 0 {
		t.Errorf("reading none: got %q status %d, want [zzz] at 0", out, st)
	}
}

// A class nothing closes is a different axis, and this scan says so by
// leaving it alone: `x[[:alpha}` holds no `:]`, so what it means is
// Semantics.UnterminatedCharacterClass's question.
func TestAnUnterminatedClassIsNotThisQuestion(t *testing.T) {
	s := eagerSubSem()
	s.UnterminatedCharacterClass = UnterminatedClassIsOrdinaryCharacters
	out, st := run(t, `v=zzz; echo "[${v#x[[:alpha}]"`, withSem(s))
	// The characters are ordinary and the bracket never closes, so the
	// *plain* axis decides — which in this vector is also "bad pattern", and
	// the row is here to show the scan handed it over rather than answering.
	if !strings.Contains(out, "bad pattern") {
		t.Errorf("got %q status %d, want the plain axis to answer", out, st)
	}
	s.UnterminatedBracket = BracketLiteral
	out, st = run(t, `v=zzz; echo "[${v#x[[:alpha}]"`, withSem(s))
	if strings.Contains(out, "bad pattern") {
		t.Errorf("got %q status %d, want no refusal once the plain axis moved", out, st)
	}
}
