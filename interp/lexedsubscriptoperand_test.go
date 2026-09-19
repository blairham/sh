// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An operand whose brackets the *parser* read — `unset m[$k]`, written with
// the `[` and the `]` outside every quoting construct — against the same
// operand handed to the builtin as one string.
//
// The two are the same characters by the time `unset` sees them and they
// name different elements, which is what these rows are for. A quote
// character between lexed brackets came out of a value and is a byte of the
// key; between brackets the builtin was handed whole it is quoting the shell
// has yet to take off.
//
// Tests name axes and wordings, never shells.

// lexedGrammar is what these operands need: brackets in a subscript, a
// declaration utility to make the table with, and the `-v` operator for the
// neighbor rows.
func lexedGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
	d.ParameterIsSetTest = true
	d.Herestring = true
}

// lexedQuoting answers the two fields these rows read: which quoting an
// operand's subscript carries, and whether a subscript that reached a
// builtin as text is expanded a second time.
//
// Both are answered the same way for every row, so that nothing here can be
// moved by changing an axis: the subject is the *route* the operand took.
func lexedQuoting(r *Runner) {
	s := *r.Semantics
	s.OperandSubscriptQuoting = OperandSubscriptEveryQuote
	s.UnsetExpandsAFlatSubscript = Yes
	s.SubscriptIsAQuotingContext = Yes
	s.PrintfAssignsWithV = Yes
	// The three the probes need to exist at all: a subscripted operand has
	// to be a name to `unset` and to a declaration for these rows to have a
	// subject, and a store through one has to reach the element.
	s.UnsetTakesASubscript = Yes
	s.TypesetTakesASubscript = Yes
	s.StoreOperandTakesASubscript = Yes
	// A refused name is reported and the script carries on, so a row can
	// show both halves of its probe.
	s.BadNameToUnsetFatal = No
	s.BadNameToReadFatal = No
	s.BadNameToPrintfFatal = No
	s.BadNameToDeclarationFatal = No
	r.Semantics = &s
}

// badName is what a builtin says about an operand that is no name of its —
// which is what `m[x'y]` is wherever the brackets were not the parser's, the
// apostrophe standing unclosed in a text nothing has unquoted.
func badName(builtin string) string {
	return "sh: " + builtin + ": `m[x'y]': not a valid identifier\n"
}

// A key holding an apostrophe, which is the character that makes the two
// routes visible: it is a quoting construct to the operand scan and an
// ordinary byte of a key.
func TestUnsetTakesAKeyHoldingAQuoteThroughLexedBrackets(t *testing.T) {
	const table = `typeset -A m; b="x'y"; m[$b]=v; `
	for _, tc := range []struct{ name, src, want string }{
		// The three spellings that write the brackets themselves. What is
		// quoted inside them is the key, never the brackets.
		{"an expanded subscript", table + `unset m[$b]`, "[GONE]"},
		{"the key written in double quotes", table + `unset m["x'y"]`, "[GONE]"},
		{"the quote written with a backslash", table + `unset m[x\'y]`, "[GONE]"},
		// And the three that hand the builtin a string. The apostrophe is
		// then an unterminated quoting construct, the operand is not one
		// `[…]`, and it names no element at all.
		{"the whole operand quoted", table + `unset "m[$b]"`, badName("unset") + "[v]"},
		{"the operand out of a value", table + `o="m[$b]"; unset $o`, badName("unset") + "[v]"},
		{"the operand out of a quoted value", table + `o="m[$b]"; unset "$o"`, badName("unset") + "[v]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + `; printf '[%s]' "${m[$b]-GONE}"`
			if out, st := runGrammar(t, src, lexedGrammar, lexedQuoting); out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The control the rows above need: a key with nothing in it to quote reaches
// the same element down both routes.
//
// Without it, a shell that had simply stopped reading a quoted subscript
// would pass three of those six rows for a reason that has nothing to do
// with which brackets the parser read.
func TestUnsetTakesAnOrdinaryKeyDownEitherRoute(t *testing.T) {
	const table = "typeset -A m; b=plain; m[$b]=v; "
	for _, tc := range []struct{ name, src string }{
		{"lexed brackets", table + `unset m[$b]`},
		{"the whole operand quoted", table + `unset "m[$b]"`},
		{"the operand out of a value", table + `o="m[$b]"; unset $o`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + `; printf '[%s]' "${m[plain]-GONE}"`
			if out, st := runGrammar(t, src, lexedGrammar, lexedQuoting); out != "[GONE]" || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, "[GONE]")
			}
		})
	}
}

// A subscript the parser read is not expanded a second time, where the same
// text handed over as a string is.
//
// The table holds both answers, so a row says which key the operand reached
// rather than only whether something was removed.
func TestUnsetDoesNotRoundASubscriptTheParserRead(t *testing.T) {
	const table = `typeset -A m; y=ZZZ; b='x$y'; m['x$y']=lit; m[xZZZ]=exp; `
	const show = `; printf '[%s][%s]' "${m['x$y']-GONE}" "${m[xZZZ]-GONE}"`
	for _, tc := range []struct{ name, src, want string }{
		// The lexed route takes the key the word already came to.
		{"lexed brackets", table + `unset m[$b]` + show, "[GONE][exp]"},
		// The text route rounds, under the axis that records it, and takes
		// the key the second reading produced.
		{"the whole operand quoted", table + `unset "m[$b]"` + show, "[lit][GONE]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runGrammar(t, tc.src, lexedGrammar, lexedQuoting); out != tc.want || st != 0 {
				t.Errorf("%q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// And the route is `unset`'s alone. Every other builtin with a name operand
// reads the brackets the same way down both routes, so a key holding a quote
// is no operand of theirs either way.
//
// The control that makes this discriminate is the second half of each row: a
// plain key goes through all of them, so a row saying the quoted key was not
// reached is not saying the builtin was broken.
func TestANeighboursOperandReadsBothRoutesAlike(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "read",
			src: `typeset -A m; b="x'y"; read m[$b] <<<Z; read m[plain] <<<Q; ` +
				`printf '[%s][%s]' "${m[$b]-U}" "${m[plain]-U}"`,
			want: badName("read") + "[U][Q]",
		},
		{
			name: "printf -v",
			src: `typeset -A m; b="x'y"; printf -v m[$b] P; printf -v m[plain] P; ` +
				`printf '[%s][%s]' "${m[$b]-U}" "${m[plain]-U}"`,
			want: badName("printf") + "[U][P]",
		},
		{
			name: "the is-set operator",
			src: `typeset -A m; b="x'y"; m[$b]=v; m[plain]=v; ` +
				`if test -v m[$b]; then printf T; else printf F; fi; ` +
				`if test -v m[plain]; then printf T; else printf F; fi`,
			want: "FT",
		},
		{
			name: "a declaration's operand",
			src: `typeset -A m; b="x'y"; typeset m[$b]=W; typeset m[plain]=W; ` +
				`printf '[%s][%s]' "${m[$b]-U}" "${m[plain]-U}"`,
			want: badName("typeset") + "[U][W]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runGrammar(t, tc.src, lexedGrammar, lexedQuoting); out != tc.want {
				t.Errorf("%q, want %q", out, tc.want)
			}
		})
	}
}
