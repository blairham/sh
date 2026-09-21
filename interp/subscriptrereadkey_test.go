// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subscript that reached arithmetic inside a *value* — `e='m[$k]'` — is
// found a second time and the `$k` in it is expanded here, which is the one
// place an expression's expansion runs twice. What that expansion produces
// is a character of the key and never quoting, exactly as a value's byte
// inside brackets the source wrote is.
//
// The control that makes this marking rather than a stopped quote removal is
// the same two apostrophes reached the other way — spelled in the re-read
// text itself rather than produced by its expansion — which the removing
// reading still takes off.
//
// Tests name axes and wordings, never shells.

// rereadKeyGrammar is what these probes need: brackets in a subscript, a
// declaration utility to make the table with, an array literal for the
// indexed control, and the quoting inside a subscript that makes quote
// removal a question at all.
func rereadKeyGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
	d.ArithSubscriptQuoting = true
}

// rereadKeyAxis answers the quoting axis these probes stand under, and pins
// the neighboring one so a row cannot move for its reason instead.
func rereadKeyAxis(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.SubscriptIsAQuotingContext = a
		// And the same answer for a subscript whose brackets arrived out of
		// a value, which is the axis the rows with no expansion in them
		// really turn on — the two real columns that read a subscript as a
		// quoting context part there, and this file's rows are written
		// against one reading at a time. See
		// Semantics.ArrivedSubscriptIsAQuotingContext.
		s.ArrivedSubscriptIsAQuotingContext = a
		// Whether a value's *bracket* is read back as subscript syntax is a
		// different question with a different consumer, and no row here
		// holds a bracket in a value. Answered so nothing below can be
		// refused for an unanswered axis.
		s.ArithSubscriptRereadsItsExpandedText = No
		r.Semantics = &s
	}
}

// The table every probe is run against. Both keys are put there through an
// expansion, so each is the characters its value holds rather than what a
// subscript's own quoting would make of them.
const rereadKeyTable = `typeset -A m; k="q'r'z"; m[$k]=4; b='a\c'; m[$b]=11; `

func TestASubscriptReReadOutOfAValueKeepsWhatItsExpansionProduced(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The issue's own shape: two apostrophes a value carried,
			// through brackets that also came out of a value.
			name: "apostrophes",
			src:  `e='m[$k]'; printf '[%s]' "$(( $e ))"`,
			want: "[4]",
		},
		{
			// A backslash the same way. Quote removal takes an unmarked one
			// off with the byte behind it, so this row fails differently
			// from the one above and not for a second reason.
			name: "backslash",
			src:  `e='m[$b]'; printf '[%s]' "$(( $e ))"`,
			want: "[11]",
		},
		{
			// The store side reads the same subscript through the same
			// function, so it moves with the read or the element lands
			// under a key nothing reads back.
			name: "store",
			src:  `e='m[$k]'; (( $e = 99 )); printf '[%s]' "${m[$k]}"`,
			want: "[99]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, rereadKeyTable+tc.src, rereadKeyGrammar, rereadKeyAxis(Yes))
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("removing quoting: %q, want %q", out, tc.want)
			}
			// The reading that removes nothing finds the same element, by
			// its own route: there is no quoting to take off either way.
			out, st = runGrammar(t, rereadKeyTable+tc.src, rereadKeyGrammar, rereadKeyAxis(No))
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("keeping quoting: %q, want %q", out, tc.want)
			}
		})
	}
}

// The discriminating control. The same two apostrophes, in the same
// brackets, reached by the same second reading — and *spelled in the re-read
// text* rather than produced by an expansion performed while reading it.
//
// The removing reading takes these off and looks up a key of three
// characters, which the table does not hold; the keeping reading finds the
// element. A fix that stopped removing quoting from a re-read subscript,
// rather than marking what its own expansion produced, would answer the
// element under both readings and this is what says so.
func TestQuotingSpelledInAReReadSubscriptIsStillRemoved(t *testing.T) {
	const src = rereadKeyTable + `g="m[q'r'z]"; printf '[%s]' "$(( $g ))"`
	out, st := runGrammar(t, src, rereadKeyGrammar, rereadKeyAxis(Yes))
	if st != 0 {
		t.Errorf("removing quoting: status %d, want 0", st)
	}
	if out != "[0]" {
		t.Errorf("removing quoting: %q, want [0] — the quotation was not removed", out)
	}
	out, st = runGrammar(t, src, rereadKeyGrammar, rereadKeyAxis(No))
	if st != 0 {
		t.Errorf("keeping quoting: status %d, want 0", st)
	}
	if out != "[4]" {
		t.Errorf("keeping quoting: %q, want [4]", out)
	}
}

// The other reading is untouched, and this says so from both sides.
//
// An indexed name's brackets hold an *expression*, which is handed back to
// the parser — so a mark there is what stops a value's bracket closing the
// subscript, and that is an axis one preset answers the other way. Marking
// for the key's sake must not reach it.
func TestAReReadSubscriptForAnIndexedNameIsStillAnExpression(t *testing.T) {
	const table = `a=(10 20 30); i=1; j='1]'; `
	out, st := runGrammar(t, table+`e='a[$i]'; printf '[%s]' "$(( $e ))"`,
		rereadKeyGrammar, rereadKeyAxis(Yes))
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if out != "[20]" {
		t.Errorf("%q, want [20] — the subscript is read as arithmetic", out)
	}
	// A bracket a value carried, which the expression reading hands to the
	// parser as it stands. The refusal names the text the script would see,
	// and the *token* it names is what moves if the key's marking reaches
	// here: a marked `]` is a byte of a value rather than the character the
	// parser stopped on.
	out, st = runGrammar(t, table+`e='a[$j]'; printf '[%s]' "$(( $e ))"`,
		rereadKeyGrammar, rereadKeyAxis(Yes))
	if st == 0 {
		t.Errorf("status %d, want a refusal", st)
	}
	if want := "sh: 1]: operator expected\n"; out != want {
		t.Errorf("%q, want %q — the value's bracket is the parser's to stop on", out, want)
	}
}
