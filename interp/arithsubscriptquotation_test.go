// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subscript whose quotation never closes, which is only reachable from
// text that reached arithmetic **already word-expanded** — a `let` operand
// whose `$k` held an apostrophe.
//
// The brackets are scanned through quotations, so a `]` inside one is a
// character of a key. A quotation that never closes is not one, and the
// columns part over what is left: two of them scan again with the quoting
// ignored and read the three characters as the key they look like, and one
// calls the whole subscript bad and stores nothing. See
// Semantics.ArithSubscriptQuotationMustClose.
//
// Tests name axes and wordings, never shells.

// quotedKeyGrammar is what these probes need: brackets in a subscript, a
// declaration utility to make the table with, and the quoting inside a
// subscript that makes an unclosed quotation possible at all.
func quotedKeyGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
	d.ArithSubscriptQuoting = true
}

// quotedKeyAxis answers the axis and says whether the session has asked the
// round to stop.
func quotedKeyAxis(a Answer, stopped bool) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ArithSubscriptQuotationMustClose = a
		// Off, so a value's own quote characters keep their marks: with it
		// on there is no quotation for the scan to leave unclosed, and every
		// row below would answer the same way for a reason that is not the
		// axis. See Semantics.ArithSubscriptRereadsItsExpandedText.
		s.ArithSubscriptRereadsItsExpandedText = No
		s.SubscriptIsAQuotingContext = Yes
		r.Semantics = &s
		if stopped {
			r.SetExpandsAnOperandsSubscriptAgain(false)
		}
	}
}

// The table every probe is run against: one element under a key holding an
// apostrophe, put there through an expansion so the key is the three
// characters and not five.
const quotedKeyTable = `typeset -A a; k="q'r"; a[$k]=4; `

func TestAnArithmeticSubscriptsUnclosedQuotationIsRefusedByAxis(t *testing.T) {
	const src = quotedKeyTable + `let "++a[$k]"; printf '[%s]' "${a[$k]}"`
	refusing, st := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(Yes, false))
	if st != 0 {
		t.Errorf("refusing: status %d, want 0", st)
	}
	if !strings.Contains(refusing, "[4]") {
		t.Errorf("refusing: %q, want the element left at [4]", refusing)
	}
	// Twice, because the column that refuses writes the sentence twice per
	// subscript *read* — measured, `let "x = a[$k] + 1"` writes it twice
	// with no store to this name at all. Once would be a report per side of
	// the operator, which happens to total two here and is a different rule.
	if n := strings.Count(refusing, "a[q'r]: bad array subscript"); n != 2 {
		t.Errorf("refusing: %d refusals in %q, want 2", n, refusing)
	}
	reading, st := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(No, false))
	if st != 0 {
		t.Errorf("reading: status %d, want 0", st)
	}
	if reading != "[5]" {
		t.Errorf("reading: %q, want %q", reading, "[5]")
	}
}

// The assignment spelling of the same operand, which is a second route to
// the same store.
func TestAnUnclosedQuotationIsRefusedThroughAnAssignmentToo(t *testing.T) {
	const src = quotedKeyTable + `let "a[$k] += 1"; printf '[%s]' "${a[$k]}"`
	refusing, _ := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(Yes, false))
	if !strings.Contains(refusing, "a[q'r]: bad array subscript") || !strings.Contains(refusing, "[4]") {
		t.Errorf("refusing: %q, want the refusal and the element left at [4]", refusing)
	}
	if reading, _ := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(No, false)); reading != "[5]" {
		t.Errorf("reading: %q, want %q", reading, "[5]")
	}
}

// The two controls the row above needs, and neither of them may move under
// either answer.
//
// The first is the whole reason this is a surface and not a key: the same
// key reached through arithmetic's *own* expansion carries the marks that
// keep a value's apostrophe out of the scan, so no quotation is ever opened
// and there is nothing for the axis to refuse. A hook wired at the key
// rather than at the giving-up scan would move it and every column in the
// panel would disagree.
//
// The second is a key with nothing in it for a quotation to open, which
// says the rows above turn on the apostrophe rather than on the operand
// having been expanded at all.
func TestASubscriptWithNoUnclosedQuotationIsTheSameElementEitherWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"arithmetic expanding the key itself",
			quotedKeyTable + `(( a[$k]++ )); printf '[%s]' "${a[$k]}"`,
			"[5]",
		},
		{
			"a key holding no quote at all",
			`typeset -A a; p=plain; a[$p]=4; let "++a[$p]"; printf '[%s]' "${a[$p]}"`,
			"[5]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No} {
				out, st := runGrammar(t, tc.src, quotedKeyGrammar, quotedKeyAxis(a, false))
				if out != tc.want || st != 0 {
					t.Errorf("%v: %q (status %d), want %q at 0", a, out, st, tc.want)
				}
			}
		})
	}
}

// The session switch is read after the axis and not instead of it: a script
// that asked for the one round it already had gets the reading answer out of
// the refusing dialect, and a dialect that reads the key was never refusing
// for the switch to move.
func TestASessionCanWithholdTheArithmeticSubscriptsRefusal(t *testing.T) {
	const src = quotedKeyTable + `let "++a[$k]"; printf '[%s]' "${a[$k]}"`
	withheld, st := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(Yes, true))
	if withheld != "[5]" || st != 0 {
		t.Errorf("withheld: %q (status %d), want %q at 0", withheld, st, "[5]")
	}
	if standing, _ := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(No, true)); standing != "[5]" {
		t.Errorf("standing: %q, want %q", standing, "[5]")
	}
}

// An axis nothing has answered is a refusal by name rather than a guess at
// which of the two readings was wanted.
func TestAnUnansweredArithmeticSubscriptQuotationIsRefusedByName(t *testing.T) {
	const src = quotedKeyTable + `let "++a[$k]"; printf '[%s]' "${a[$k]}"`
	out, _ := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(Unspecified, false))
	if !strings.Contains(out, "quotation never closes") {
		t.Errorf("unanswered: %q, want the axis named", out)
	}
}

// A quotation the key *closes* is not this axis and must not be refused by
// it: the brackets were found on the first scan, with the quoting read, so
// nothing gave up and there is nothing to refuse.
//
// It is the nearest neighbor and the control that says the refusal turns on
// the giving-up rather than on there being a quote in the key at all. Only
// the refusal is asserted, because which element the two closed quotes name
// is a second question this shell answers with bash and against ksh93 — the
// `let` route carries no marks, so quote removal takes them off and the key
// is `qrz` — and that is not moved by this axis in either direction.
func TestAQuotationTheKeyClosesIsNotRefused(t *testing.T) {
	const src = `typeset -A a; k="q'r'z"; a[$k]=4; let "++a[$k]"; printf 'done'`
	for _, a := range []Answer{Yes, No} {
		out, st := runGrammar(t, src, quotedKeyGrammar, quotedKeyAxis(a, false))
		if strings.Contains(out, "bad array subscript") || st != 0 {
			t.Errorf("%v: %q (status %d), want no refusal at 0", a, out, st)
		}
	}
}
