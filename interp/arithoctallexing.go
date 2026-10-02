// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a leading zero names a numeral's base is a question about how text
// is **lexed** — it decides where the numeral ends — so it lives on
// syntax.Dialect and not on Semantics, and one dialect spells it as an option
// a running script switches. That is the same combination
// [Runner.SetBraceReservedWordReadings] has, and the same consequences
// follow: the answer that decides is the one in force when the text is
// parsed, the dialect is copied and replaced rather than written through, and
// the front end's run loop reads the rest of the program with whatever it
// finds.
//
// # Two halves and one name
//
// `octalzeroes` moves both. Semantics.ArithLeadingZeroIsOctal says what the
// digits are *worth*, and this says how far the reader gets before it stops.
// They are written through separate seams because they are read in separate
// places — the evaluator and the parser — and a change that moved only the
// first left them disagreeing: the numeral `08` was read whole in base ten
// and then handed to a converter that wanted octal, which refused the lot
// where the shell being copied had already stopped at the `8` and gone on to
// report a stray token. See Dialect.ArithLeadingZeroNamesOctalDigits, where
// the rows are.

// ArithLeadingZeroNamesOctalDigits reports whether a leading zero names the
// base the numeral's digits are read in.
func (r *Runner) ArithLeadingZeroNamesOctalDigits() bool {
	// One axis per lang call: the value it hands back is shared and carries
	// no adjustment, so a caller may read a field off it and nothing else.
	// See TestNothingReadsAnAdjustedAxisOffLang.
	return r.lang().ArithLeadingZeroNamesOctalDigits
}

// SetArithLeadingZeroNamesOctalDigits moves it, for a dialect whose option
// namespace has a name for it.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must
// not change the grammar of the shell that spawned it.
func (r *Runner) SetArithLeadingZeroNamesOctalDigits(on bool) {
	d := r.dialect()
	if d.ArithLeadingZeroNamesOctalDigits == on {
		return
	}
	d.ArithLeadingZeroNamesOctalDigits = on
	r.Dialect = &d
}

// ArithForcesFloat reports whether arithmetic reads every operand as a float.
// See Runner.forcedFloat.
func (r *Runner) ArithForcesFloat() bool { return r.arithForcesFloat }

// SetArithForcesFloat moves it, for a dialect whose option namespace has a
// name for it — zsh's `force_float`.
func (r *Runner) SetArithForcesFloat(on bool) { r.arithForcesFloat = on }
