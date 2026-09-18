// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `read -t` given a word that is not a written number (#3209).
//
// One column reads the argument as an *expression* — the same shape
// UlimitOperandIsArithmetic records at the other builtin — so an unset name
// is nought there and the read times out at once, in silence. The axis is
// moved both ways over the same words, and the controls are the words that
// are already numbers: those reach the question from neither side.
func TestAReadTimeoutThatIsNotAWrittenNumber(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arith   Answer
		word    string
		refused bool
	}{
		{name: "a name, as an expression", arith: Yes, word: "abc"},
		{name: "a name, refused", arith: No, word: "abc", refused: true},
		{name: "a hexadecimal numeral, as an expression", arith: Yes, word: "0x3"},
		{name: "a hexadecimal numeral, refused", arith: No, word: "0x3", refused: true},
		{name: "blanks around a number, as an expression", arith: Yes, word: " 3 "},
		{name: "blanks around a number, refused", arith: No, word: " 3 ", refused: true},
		{name: "an empty word, as an expression", arith: Yes, word: ""},
		{name: "an empty word, refused", arith: No, word: "", refused: true},
		// A word the expression grammar cannot read either: refused under
		// both answers, which is what says the reading is an evaluation and
		// not an acceptance.
		{name: "not an expression either, as an expression", arith: Yes, word: "3abc", refused: true},
		{name: "not an expression either, refused", arith: No, word: "3abc", refused: true},
		// The controls.
		{name: "a written number, as an expression", arith: Yes, word: "3"},
		{name: "a written number, refused", arith: No, word: "3"},
		{name: "a written fraction, as an expression", arith: Yes, word: "1.5"},
		{name: "a written fraction, refused", arith: No, word: "1.5"},
		// A negative timeout is refused whichever way this is answered: it
		// is a number, so the axis is never reached.
		{name: "a negative number, as an expression", arith: Yes, word: "-1", refused: true},
		{name: "a negative number, refused", arith: No, word: "-1", refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			// The letters are the dialect's; this one needs the `-t`
			// its argument is the whole question about.
			sem.ReadOptions = "rt:"
			// A neighboring axis the timed read consults, answered so
			// that a row reaching the wait is about the operand.
			sem.ReadTimeoutBoundsReadability = Yes
			sem.ReadZeroTimeout = ReadZeroTimeoutTakesWhatIsWaiting
			sem.ReadTimeoutOperandIsArithmetic = tc.arith
			out, st := run(t, `read -t '`+tc.word+`' x </dev/null; echo "st=$?"`,
				func(r *Runner) { r.Semantics = &sem })
			said := strings.Contains(out, "invalid number")
			if said != tc.refused {
				t.Errorf("out %q status %d, refused=%v want %v", out, st, said, tc.refused)
			}
			// Either way the read hits end of file and reports 1, so what
			// parts the answers is the diagnostic and not the status.
			if !strings.Contains(out, "st=1") {
				t.Errorf("out %q, want the read to report 1", out)
			}
		})
	}
	// And an unanswered axis refuses by name, for a word that reaches it.
	sem := permissive()
	sem.ReadOptions = "rt:"
	sem.ReadTimeoutBoundsReadability = Yes
	sem.ReadZeroTimeout = ReadZeroTimeoutTakesWhatIsWaiting
	sem.ReadTimeoutOperandIsArithmetic = Unspecified
	out, st := run(t, `read -t abc x </dev/null`, func(r *Runner) { r.Semantics = &sem })
	if st != 2 || !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("unanswered: out %q status %d, want the refusal by name at 2", out, st)
	}
	out, st = run(t, `read -t 3 x </dev/null; echo "st=$?"`, func(r *Runner) { r.Semantics = &sem })
	if st != 0 || !strings.Contains(out, "st=1") {
		t.Errorf("a written number with the axis unanswered: out %q status %d", out, st)
	}
}
