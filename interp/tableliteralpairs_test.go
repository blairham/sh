// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.BareElementsInATableLiteralMustPairOff, both ways round over one
// source.
//
// `typeset -A m=(a 1 b)` is three fields where the pairing wants an even
// number. One reading refuses the literal and gives up the input; the other
// takes the last field as a key with nothing under it, which is two keys and
// status 0.
func TestAnOddKeyedLiteralFollowsTheAxis(t *testing.T) {
	const src = `typeset -A m=(a 1 b); printf '[%s][%s][%s]' "$?" "${#m[@]}" "${m[b]-ABSENT}"; printf reached`
	t.Run("refused", func(t *testing.T) {
		out, st := runPairedLiteral(t, Yes, src)
		if strings.Contains(out, "reached") {
			t.Errorf("= %q, want the input given up before the next command", out)
		}
		if st == 0 {
			t.Errorf("status = %d, want non-zero", st)
		}
	})
	t.Run("the last field is a key with nothing under it", func(t *testing.T) {
		out, st := runPairedLiteral(t, No, src)
		if want := "[0][2][]reached"; out != want || st != 0 {
			t.Errorf("= %q status %d, want %q at 0", out, st, want)
		}
	})
}

// The three controls the refusing reading must still take, because each is a
// line every column accepts and a check written on the wrong noun refuses one
// of them.
//
// An **even** list pairs off; an **empty** literal has nothing to pair and is
// not the same as an odd one; and a **repeated key** is legal — the later
// value wins and the table holds one element, so a check counting *keys*
// rather than *fields* would see one key from two pairs and refuse.
func TestAnEvenKeyedLiteralIsTakenUnderEitherAnswer(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an even list", `typeset -A m=(a 1 b 2); printf '[%s][%s]' "$?" "${#m[@]}"`, "[0][2]"},
		{"an empty literal", `typeset -A m=(); printf '[%s][%s]' "$?" "${#m[@]}"`, "[0][0]"},
		{"a repeated key", `typeset -A m=(a 1 a 2); printf '[%s][%s][%s]' "$?" "${#m[@]}" "${m[a]}"`, "[0][1][2]"},
		{"every element carries a head", `typeset -A m=([a]=1); printf '[%s][%s]' "$?" "${#m[@]}"`, "[0][1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, axis := range []Answer{Yes, No} {
				out, st := runPairedLiteral(t, axis, tc.src)
				if out != tc.want || st != 0 {
					t.Errorf("axis %v: = %q status %d, want %q at 0", axis, out, st, tc.want)
				}
			}
		})
	}
}

// The noun is the **field count** and not the written word count, and this is
// the pair that says so: one element is written either way, and only the
// number of fields it came to moves the answer.
//
// It is also the case a script actually hits — a literal spelled with an odd
// number of words is a typo, a list handed in from somewhere else is a bug —
// so a check written over the source would close the harmless half and leave
// this one open.
//
// The elements have to be reaching the *splitting* reading of the axis beside
// this one for a written word to come to more than one field at all, so
// BareElementsInATableLiteralAreEachOneValue is set to No here rather than
// left at the floor. Under the floor's Yes, `$w` is a single field however
// many words are in it — which is a third answer to "how many fields", and
// the reason this test sets the field count rather than assuming it.
func TestTheOddCountIsOverFieldsAndNotWrittenWords(t *testing.T) {
	const odd = `w='a 1 b'; typeset -A m=($w); printf '[%s]' "$?"; printf reached`
	const even = `w='a 1 b 2'; typeset -A m=($w); printf '[%s][%s]' "$?" "${#m[@]}"; printf reached`
	out, st := runSplitPairedLiteral(t, odd)
	if strings.Contains(out, "reached") || st == 0 {
		t.Errorf("three fields from one word = %q status %d, want the refusal", out, st)
	}
	out, st = runSplitPairedLiteral(t, even)
	if want := "[0][2]reached"; out != want || st != 0 {
		t.Errorf("four fields from one word = %q status %d, want %q at 0", out, st, want)
	}
}

// And the target's **kind** is the other half of it: the same three words
// assigned to a name that is not a table are an ordinary three-element array
// under either answer, so the axis is about a keyed literal and not about
// literals.
func TestAnIndexedLiteralNeverAsksThePairingQuestion(t *testing.T) {
	for _, axis := range []Answer{Yes, No} {
		out, st := runPairedLiteral(t, axis, `a=(a 1 b); printf '[%s][%s]' "$?" "${#a[@]}"`)
		if want := "[0][3]"; out != want || st != 0 {
			t.Errorf("axis %v: = %q status %d, want %q at 0", axis, out, st, want)
		}
	}
}

// An unanswered axis refuses by name rather than guessing, which is the
// substrate's rule for every axis and is what `cmd/sh` with no dialect does
// here.
func TestAnUnansweredPairingRefusesByName(t *testing.T) {
	out, _ := runPairedLiteral(t, Unspecified, `typeset -A m=(a 1 b); printf reached`)
	if !strings.Contains(out, "odd number of fields") {
		t.Errorf("= %q, want the axis named", out)
	}
}

// runSplitPairedLiteral is runPairedLiteral with the refusing answer and with
// a bare element read as an ordinary word, so that one written word can come
// to several fields.
func runSplitPairedLiteral(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := testSemantics()
	sem.BareElementsInATableLiteralMustPairOff = Yes
	sem.BareElementsInATableLiteralAreEachOneValue = No
	return runGrammar(t, src, func(d *syntax.Dialect) { d.ParamIndirection = true },
		func(r *Runner) { r.Semantics = &sem })
}

func runPairedLiteral(t *testing.T, axis Answer, src string) (string, int) {
	t.Helper()
	sem := testSemantics()
	sem.BareElementsInATableLiteralMustPairOff = axis
	return runGrammar(t, src, func(d *syntax.Dialect) { d.ParamIndirection = true },
		func(r *Runner) { r.Semantics = &sem })
}
