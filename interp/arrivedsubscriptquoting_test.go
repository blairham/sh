// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether quote removal runs over an associative subscript whose brackets
// arrived already word-expanded, which is a second question from the one the
// same brackets pose when a script writes them inside `(( … ))`: one column
// removes the quoting in both places and another removes it only where the
// brackets are the source's, so the identical text names two different keys.
// See Semantics.ArrivedSubscriptIsAQuotingContext.
//
// Two constructs reach it and the axis is read from the text rather than from
// either of them — a builtin operand, and an expression whose subscript came
// out of a value — so both routes are driven here.
//
// Tests name axes and wordings, never shells.

// letQuotingAxis answers the two quoting axes independently, so a probe can
// say which of them decided the key it reached.
func letQuotingAxis(arrived, written Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ArrivedSubscriptIsAQuotingContext = arrived
		s.SubscriptIsAQuotingContext = written
		r.Semantics = &s
	}
}

// The table every probe below is run against: one element under a bare key
// and one under the same key wrapped in quote characters, so which of the two
// an increment reaches says which reading was taken.
//
// The quoted key is stored and read back through a **value**, because a
// value's quote characters are characters and not quoting under either
// answer — so the two spellings this file compares are the ones the operand
// wrote, and the table's own two keys stay put while the axis moves.
const letQuotedKeyTable = `typeset -A a; a[k]=1; q='"k"'; a[$q]=2; `

// The element each reading names, read back under both spellings.
const letQuotedKeyRead = `; printf '[%s][%s]' "${a[k]}" "${a[$q]}"`

func TestALetOperandsSubscriptIsAQuotingContextOrIsTakenAsWritten(t *testing.T) {
	const src = letQuotedKeyTable + `let '++a["k"]'` + letQuotedKeyRead
	for _, tc := range []struct {
		name string
		axis Answer
		want string
	}{
		// Quote removal ran, so the brackets named the bare key and it is
		// the one that moved.
		{"a quoting context", Yes, "[2][2]"},
		// Taken as written, so the three characters between the brackets
		// are the key — which is the one the table stored under the quoted
		// spelling, and it is the one that moved.
		{"taken as written", No, "[1][3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(tc.axis, Yes))
			if out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The control that makes the axis above a second one rather than a rename of
// SubscriptIsAQuotingContext: the same brackets inside `(( … ))` answer the
// *other* axis, and the two can disagree.
//
// It is the discriminating half. A shell that removed the quoting in both
// places and one that removed it only in the expression agree on every probe
// that reaches one route, so a test driving only `let` cannot tell an axis
// that was consulted from a value that happened to match.
func TestAnArithmeticCommandsSubscriptKeepsTheOtherQuotingAxis(t *testing.T) {
	const src = letQuotedKeyTable + `(( ++a["k"] ))` + letQuotedKeyRead
	for _, tc := range []struct {
		name  string
		inLet Answer
		want  string
	}{
		{"with the builtin removing quoting too", Yes, "[2][2]"},
		// The builtin's answer is no and the expression's is yes, which is
		// the split one real column has: this route must still take the
		// expression's.
		{"with the builtin taking its operand as written", No, "[2][2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(tc.inLet, Yes))
			if out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
	// And the expression's own answer still decides that route, which is
	// what says the row above is reading the axis rather than a constant.
	out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(Yes, No))
	if out != "[1][3]" || st != 0 {
		t.Errorf("expression taken as written: %q (status %d), want %q at 0", out, st, "[1][3]")
	}
}

// A backslash is taken off with the quotes or kept with them, so the axis is
// quote removal and not one character being spared.
func TestALetOperandsSubscriptKeepsOrDropsABackslashWithTheQuotes(t *testing.T) {
	const src = `typeset -A a; a[k]=1; e='\k'; a[$e]=2; let '++a[\k]'; printf '[%s][%s]' "${a[k]}" "${a[$e]}"`
	for _, tc := range []struct {
		axis Answer
		want string
	}{
		{Yes, "[2][2]"},
		{No, "[1][3]"},
	} {
		out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(tc.axis, Yes))
		if out != tc.want || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", tc.axis, out, st, tc.want)
		}
	}
}

// The shape a script actually meets: the quote characters arrive in a
// **value**, and the operand is word-expanded before the builtin sees it, so
// nothing in it carries syntax.ArithValueMark by the time the subscript is
// read.
//
// The control beside it is the same key reached by arithmetic's own
// expansion, where the value's quotes do carry marks and quote removal leaves
// a marked quote alone — unanimous in the panel, and unmoved by either answer
// here.
func TestAValuesQuoteCharactersReachALetOperandUnmarked(t *testing.T) {
	const table = `typeset -A a; k="q'r'z"; a[$k]=4; `
	for _, tc := range []struct {
		axis Answer
		want string
	}{
		// Removed, so the increment went to a key the table does not hold
		// and the stored element stands.
		{Yes, "[4]"},
		// Kept, so the two apostrophes are two characters of the key and
		// the element the script stored is the one that moved.
		{No, "[5]"},
	} {
		out, st := runGrammar(t, table+`let "++a[$k]"; printf '[%s]' "${a[$k]}"`,
			quotedKeyGrammar, letQuotingAxis(tc.axis, Yes))
		if out != tc.want || st != 0 {
			t.Errorf("let %v: %q (status %d), want %q at 0", tc.axis, out, st, tc.want)
		}
		out, st = runGrammar(t, table+`(( a[$k]++ )); printf '[%s]' "${a[$k]}"`,
			quotedKeyGrammar, letQuotingAxis(tc.axis, Yes))
		if out != "[5]" || st != 0 {
			t.Errorf("arithmetic %v: %q (status %d), want %q at 0", tc.axis, out, st, "[5]")
		}
	}
}

// An axis nothing has answered is a refusal by name rather than a guess at
// which of the two keys was meant.
func TestAnUnansweredLetOperandSubscriptQuotingIsRefusedByName(t *testing.T) {
	const src = letQuotedKeyTable + `let '++a["k"]'` + letQuotedKeyRead
	out, _ := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(Unspecified, Yes))
	if !strings.Contains(out, "quoting context") {
		t.Errorf("unanswered: %q, want the axis named", out)
	}
}

// A subscript with no quoting in it at all asks neither axis, so a dialect
// that answers neither still reaches the element — which is nearly every
// subscript a script writes.
func TestASubscriptWithNoQuotingAsksNeitherQuotingAxis(t *testing.T) {
	const src = `typeset -A a; a[k]=1; let '++a[k]'; printf '[%s]' "${a[k]}"`
	out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(Unspecified, Unspecified))
	if out != "[2]" || st != 0 {
		t.Errorf("%q (status %d), want %q at 0", out, st, "[2]")
	}
}

// The second route to the same axis: the brackets came out of a **value**,
// with no builtin in sight.
//
// This is the discriminating half of the axis's scope. A reading taken from
// which construct asked — the builtin's name — and a reading taken from where
// the bracketed text came from agree on every probe that drives only `let`,
// so a test that stopped there could not tell one from the other.
func TestAnArrivedSubscriptsQuotingIsTheAxisWhateverBroughtIt(t *testing.T) {
	// The table holds one element under the bare key and one under the same
	// key with its apostrophes, both stored through a value so the two
	// spellings stay put while the axis moves. The subscript under test
	// arrives whole, in `$e`.
	const src = `typeset -A a; a[qrz]=1; q="q'r'z"; a[$q]=2; e="a[q'r'z]"; ` +
		`(( ++$e )); printf '[%s][%s]' "${a[qrz]}" "${a[$q]}"`
	for _, tc := range []struct {
		name string
		axis Answer
		want string
	}{
		// Quote removal ran over the arrived brackets, so they named the
		// bare key and it is the one that moved.
		{"a quoting context", Yes, "[2][2]"},
		// Taken as written, so the five characters between the brackets are
		// the key and the quoted element is the one that moved.
		{"taken as written", No, "[1][3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(tc.axis, Yes))
			if out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The control beside it: the same five characters in the same brackets, this
// time the source's own, answer the *other* axis and do not move with this
// one.
func TestASourcesOwnBracketsDoNotTakeTheArrivedAnswer(t *testing.T) {
	const src = `typeset -A a; a[qrz]=1; q="q'r'z"; a[$q]=2; ` +
		`(( ++a[q'r'z] )); printf '[%s][%s]' "${a[qrz]}" "${a[$q]}"`
	for _, axis := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(axis, Yes))
		if out != "[2][2]" || st != 0 {
			t.Errorf("arrived %v: %q (status %d), want %q at 0", axis, out, st, "[2][2]")
		}
	}
}

// An arrived subscript that still holds an expansion has its quoting removed
// under either answer, because performing the expansion is what removes it.
//
// The row that separates the two readings the axis could have had: "an
// arrived subscript is never a quoting context" and "quote removal on one
// happens as part of performing an expansion in it" agree everywhere except
// here.
func TestAnArrivedSubscriptStillHoldingAnExpansionIsRemovedEitherWay(t *testing.T) {
	// The expansion is outside the apostrophes, because one *inside* them is
	// stopped rather than performed — that is the neighboring axis and it is
	// what the last probe in this file holds still.
	const src = `typeset -A a; a[qrz]=1; q="qr'z'"; a[$q]=2; r=r; e="a[q\$r'z']"; ` +
		`(( ++$e )); printf '[%s][%s]' "${a[qrz]}" "${a[$q]}"`
	for _, axis := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, letQuotingAxis(axis, Yes))
		if out != "[2][2]" || st != 0 {
			t.Errorf("arrived %v: %q (status %d), want %q at 0", axis, out, st, "[2][2]")
		}
	}
}
