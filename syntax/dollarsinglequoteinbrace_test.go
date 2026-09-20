// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `$'…'` written in a `${…}` body is one construct, so its backslash quotes
// the byte behind it and the `'` that ends it is the first unescaped one.
//
// The brace scanner stepped over a `$( )` whole and over a `'…'` as a plain
// quoted run, and read `$'` as a dollar beside a quote — which ends the run at
// the backslashed quote, lets the quote behind it open a second run, and walks
// off the end of the input. So a body holding nothing but an escaped quote was
// fatal in every dialect that has the construct, while the same string written
// outside braces printed (#3896):
//
//	echo ${x:-$'\''}
//
// The rows are about [Dialect.DollarSingleQuote] and name no shell. What the
// four reference shells print for each is in
// dialect/bash/dollarsinglequoteinbrace_test.go and its two siblings.
func TestADollarSingleQuoteInsideABraceBodyKeepsItsEscapes(t *testing.T) {
	t.Parallel()
	on := Core()
	on.DollarSingleQuote = true
	off := Core()
	off.DollarSingleQuote = false
	for _, tc := range []struct {
		name string
		src  string
		body string // the ParamExp span's Value
	}{
		// The case the issue was filed on, and the operators it reaches
		// through.
		{"a default word", `echo ${x:-$'\''}`, `x:-$'\''`},
		{"an alternate word", `echo ${x+$'\''}`, `x+$'\''`},
		{"a prefix pattern", `echo ${x#$'\''}`, `x#$'\''`},
		{"a replacement", `echo ${v/x/$'\''}`, `v/x/$'\''`},
		// A `}` inside the run is hidden by it, exactly as it is hidden by
		// a plain quoted run — which already worked and is the control that
		// says the escape is what moved.
		{"a brace inside the run", `echo ${x:-$'a}b'}`, `x:-$'a}b'`},
		{"a brace inside a plain run", `echo ${x:-'a}b'}`, `x:-'a}b'`},
		// Every other escape was already right, because none of them ends
		// the run early.
		{"an ordinary escape", `echo ${x:-$'a\tb'}`, `x:-$'a\tb'`},
		{"no escape at all", `echo ${x:-$'hello'}`, `x:-$'hello'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, on)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			spans := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args[1].Spans
			var got string
			var found bool
			for _, s := range spans {
				if s.Kind == ParamExp {
					got, found = s.Value, true
				}
			}
			if !found {
				t.Fatalf("%s: no ParamExp span: %#v", tc.src, spans)
			}
			if got != tc.body {
				t.Errorf("%s: body %q, want %q", tc.src, got, tc.body)
			}
		})
	}
	// The control that must not move: a run the input really does end
	// inside is still a failure, so the change is "the escape is honored"
	// and not "the scanner stopped caring where a quote ends". All four
	// reference shells refuse this one.
	if _, err := Parse(`echo ${x:-$'\'}`, on); err == nil {
		t.Errorf(`${x:-$'\'} parsed, want the unterminated run refused`)
	}
	// And with the construct off, `$'` is a dollar beside an ordinary quote
	// again — the flag is what the rule hangs on, so a dialect without it
	// reads the text the way it always did.
	if _, err := Parse(`echo ${x:-$'\''}`, off); err == nil {
		t.Errorf(`${x:-$'\''} parsed with DollarSingleQuote off, want the plain quoted reading`)
	}
}
