// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether an apostrophe a script **writes** between a subscript's brackets
// stops the expansion it holds, or the expansion is performed and the
// apostrophes are then dealt with by quote removal. See
// Semantics.WrittenSubscriptQuotationStopsItsExpansion.
//
// Tests name axes and wordings, never shells.

// writtenQuotationAxis answers this axis and pins the two beside it, so a
// probe cannot pass because one of those moved instead.
func writtenQuotationAxis(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.WrittenSubscriptQuotationStopsItsExpansion = a
		s.SubscriptIsAQuotingContext = Yes
		s.ArrivedSubscriptIsAQuotingContext = Yes
		r.Semantics = &s
	}
}

func TestAWrittenSubscriptQuotationStopsOrPerformsWhatItHolds(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m['$kq'] = 42 )); ` +
		`printf '[%s][%s]' "${m[q]}" "${m['$kq']}"`
	for _, tc := range []struct {
		name string
		axis Answer
		want string
	}{
		// Stopped, so the key is the three characters the apostrophes held
		// and the expansion never ran.
		{"stopped", Yes, "[][42]"},
		// Performed, so the key is what the expansion produced and the
		// apostrophes came off with quote removal.
		{"performed", No, "[42][]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(tc.axis))
			if out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A double quotation performs what it holds under either answer, which is
// the control the axis rests on: it is the apostrophe and not quoting in
// general.
func TestAWrittenSubscriptsDoubleQuotationPerformsWhatItHoldsEitherWay(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m["$kq"] = 42 )); printf '[%s]' "${m[q]}"`
	for _, axis := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(axis))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", axis, out, st, "[42]")
		}
	}
}

// And an apostrophe *inside* a double quotation stops nothing, because the
// double quotation is the one in charge — the second control, and the one a
// rule written over the apostrophe alone would get wrong.
func TestAnApostropheInsideADoubleQuotationStopsNothing(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m["'$kq'"] = 42 )); printf "[%s]" "${m["'q'"]}"`
	for _, axis := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(axis))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", axis, out, st, "[42]")
		}
	}
}

// A subscript whose brackets **arrived** already word-expanded answers the
// other question, and does not move with this one: the apostrophes stop the
// expansion there under both answers here.
func TestAnArrivedSubscriptsQuotationIsNotThisAxis(t *testing.T) {
	const src = `typeset -A m; kq=q; e="m['\$kq']"; (( $e = 42 )); printf '[%s]' "${m['$kq']}"`
	for _, axis := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(axis))
		if out != "[42]" || st != 0 {
			t.Errorf("%v: %q (status %d), want %q at 0", axis, out, st, "[42]")
		}
	}
}

// The expansion is not performed, rather than performed and its result
// thrown away: a command substitution an apostrophe holds is a command that
// does not run.
func TestAStoppedSubscriptExpansionIsNeverPerformed(t *testing.T) {
	const src = `typeset -A m; ran=no; (( m['$(ran=yes; echo x)'] = 1 )); printf '[%s]' "$ran"`
	out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(Yes))
	if out != "[no]" || st != 0 {
		t.Errorf("stopped: %q (status %d), want %q at 0", out, st, "[no]")
	}
}

// An axis nothing has answered is a refusal by name rather than a guess at
// which of the two keys was meant.
func TestAnUnansweredWrittenSubscriptQuotationIsRefusedByName(t *testing.T) {
	const src = `typeset -A m; kq=q; (( m['$kq'] = 42 ))`
	out, _ := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(Unspecified))
	if !strings.Contains(out, "stopping the expansion") {
		t.Errorf("unanswered: %q, want the axis named", out)
	}
}

// A subscript with an apostrophe but no expansion inside it never puts the
// question, so a dialect that has not answered still reaches the element.
func TestAWrittenQuotationWithNoExpansionAsksNothing(t *testing.T) {
	const src = `typeset -A m; (( m['q'] = 42 )); printf '[%s]' "${m[q]}"`
	out, st := runGrammar(t, src, quotedKeyGrammar, writtenQuotationAxis(Unspecified))
	if out != "[42]" || st != 0 {
		t.Errorf("%q (status %d), want %q at 0", out, st, "[42]")
	}
}
