// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether an associative key ends at the apostrophe-quoted run that
// performed an expansion in it, dropping whatever a script wrote after that
// run. See Semantics.SubscriptQuotationEndsTheKey.
//
// Tests name axes and wordings, never shells.

// keyEndsAxis answers this axis and pins the two beside it, so no probe can
// pass because one of those moved instead. The quotation has to be performed
// for a key to end at it, which is why the stopping axis is answered no.
func keyEndsAxis(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.SubscriptQuotationEndsTheKey = a
		s.WrittenSubscriptQuotationStopsItsExpansion = No
		s.SubscriptIsAQuotingContext = Yes
		s.ArrivedSubscriptIsAQuotingContext = No
		// What brackets left holding nothing mean, answered because one
		// row's key *is* the empty string once the run and the text after
		// it are gone — and an unanswered axis there is a refusal about
		// something other than this one.
		s.EmptyArithSubscript = EmptyArithSubscriptIsTheEmptyExpression
		r.Semantics = &s
	}
}

// The table stores through the subscript and reads back under both keys, so
// which element moved says which reading was taken.
func TestAKeyEndsOrDoesNotEndAtAQuotedExpansion(t *testing.T) {
	for _, tc := range []struct {
		name, src, ends, keeps string
	}{
		{
			// Text after the run, so the run and the text both go.
			"text after the run",
			`typeset -A m; kq=q; (( m[q'$kq'z] = 42 )); printf '[%s][%s]' "${m[q]}" "${m[qqz]}"`,
			"[42][]", "[][42]",
		},
		{
			// The run ends the subscript, so its own value is the tail of
			// the key and nothing is lost.
			"the run ends it",
			`typeset -A m; kq=q; (( m[q'$kq'] = 42 )); printf '[%s][%s]' "${m[qq]}" "${m[q]}"`,
			"[42][]", "[42][]",
		},
		{
			// Nothing in front of the run and text after it, so the key is
			// the empty string and neither spelling of the text is there.
			// Read back by what is *not* found, because reading the empty
			// key puts a different axis.
			"nothing in front and text after",
			`typeset -A m; kq=q; (( m['$kq'z] = 42 )); printf '[%s][%s]' "${m[q]}" "${m[qz]}"`,
			"[][]", "[][42]",
		},
		{
			// Two runs, and the *last* one decides: it ends the subscript,
			// so it contributes, and the prefix in front of it truncates by
			// the same rule to nothing.
			"the last run decides",
			`typeset -A m; kq=q; (( m['$kq'z'$kq'] = 42 )); printf '[%s][%s]' "${m[q]}" "${m[qzq]}"`,
			"[42][]", "[][42]",
		},
		{
			// What the run contributes is its *content* read again, so a
			// double quotation inside it comes off too.
			"a double quotation inside the run",
			`typeset -A m; kq=q; d='"q"'; (( m['"$kq"'] = 42 )); printf '[%s][%s]' "${m[q]}" "${m[$d]}"`,
			"[42][]", "[][42]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, quotedKeyGrammar, keyEndsAxis(Yes))
			if out != tc.ends || st != 0 {
				t.Errorf("ending: %q (status %d), want %q at 0", out, st, tc.ends)
			}
			out, st = runGrammar(t, tc.src, quotedKeyGrammar, keyEndsAxis(No))
			if out != tc.keeps || st != 0 {
				t.Errorf("keeping: %q (status %d), want %q at 0", out, st, tc.keeps)
			}
		})
	}
}

// A run with **no expansion** in it is not one of these, which is the control
// that says this is the performing and not the quotation: the key is the
// whole subscript under either answer.
func TestAQuotedRunWithNoExpansionEndsNothing(t *testing.T) {
	const src = `typeset -A m; (( m[q'r'z] = 42 )); printf '[%s]' "${m[qrz]}"`
	for _, a := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, keyEndsAxis(a))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", a, out, st, "[42]")
		}
	}
}

// And an apostrophe *inside a double quotation* is not a run at all, so the
// key keeps every character under either answer — the second control.
func TestAnApostropheInsideADoubleQuotationEndsNothing(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m["'$kq'z"] = 42 )); printf "[%s]" "${m["'q'z"]}"`
	for _, a := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, keyEndsAxis(a))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", a, out, st, "[42]")
		}
	}
}

// A subscript that **arrived** already word-expanded is untouched: the
// apostrophes stop the expansion there, so there is nothing performed for a
// key to end at.
func TestAnArrivedSubscriptsKeyDoesNotEndEarly(t *testing.T) {
	const src = `typeset -A m; kq=q; e="m['\$kq'z]"; (( $e = 42 )); printf '[%s]' "${m[$'\x24'kqz]}"`
	for _, a := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, keyEndsAxis(a))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", a, out, st, "[42]")
		}
	}
}

// An axis nothing has answered is a refusal by name rather than a guess at
// which of the two keys was meant.
func TestAnUnansweredKeyEndingIsRefusedByName(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m[q'$kq'z] = 42 ))`
	out, _ := runGrammar(t, src, quotedKeyGrammar, keyEndsAxis(Unspecified))
	if !strings.Contains(out, "ending at the apostrophe-quoted run") {
		t.Errorf("unanswered: %q, want the axis named", out)
	}
}
