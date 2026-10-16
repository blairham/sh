// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A `{name}` redirection behind a compound command, in the grammar that
// refuses it there: a parse error naming the brace or the whole word by the
// compound's kind. See [syntax.Dialect.FdVariableRefusedAfterACompound].
//
// Every row is a pair that holds the compound fixed and moves one thing, so
// the noun the rule is keyed on is the one being varied: the brace-body `for`
// row and the brace-group row both end in `}` and name different tokens, and
// the redirection written in between moves neither.
func TestAnFdVariableAfterACompoundIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src, want string
	}{
		{"{ :; } {v}</dev/null", "{"},
		{"{ :; } >/dev/null {v}</dev/null", "{"},
		{"( : ) {v}</dev/null", "{"},
		{"case x in x) ;; esac {v}</dev/null", "{"},
		{"[[ -n x ]] {v}</dev/null", "{"},
		{"(( 1 )) {v}</dev/null", "{"},
		{"f() { :; } {v}</dev/null", "{"},
		{"if :; then :; fi {v}</dev/null", "{v}"},
		{"if :; then :; fi >/dev/null {v}</dev/null", "{v}"},
		{"while false; do :; done {v}</dev/null", "{v}"},
		{"for i in 1; do :; done {v}</dev/null", "{v}"},
		{"for i (1) { : } {v}</dev/null", "{v}"},
		{"for ((i=0;i<1;i++)) do :; done {v}</dev/null", "{v}"},
		{"repeat 1 do :; done {v}</dev/null", "{v}"},
		{"select i in a; do break; done {v}</dev/null", "{v}"},
	} {
		t.Run(c.src, func(t *testing.T) {
			t.Parallel()
			_, err := syntax.Parse(c.src, refusingDialect(true))
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("err %v, want a syntax.Error", err)
			}
			if se.Token != c.want {
				t.Errorf("blamed %q, want %q", se.Token, c.want)
			}
		})
	}
}

// Without a bare brace that is a word of its own, there is nothing for the
// refusal to name but the whole word — one dialect's `emulate sh`.
func TestAnFdVariableAfterABraceGroupIsNamedWholeWithoutABareBrace(t *testing.T) {
	t.Parallel()
	_, err := syntax.Parse("{ :; } {v}</dev/null", refusingDialect(false))
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("err %v, want a syntax.Error", err)
	}
	if se.Token != "{v}" {
		t.Errorf("blamed %q, want the whole word", se.Token)
	}
}

// The controls: the same text in a grammar that reads it, a numbered
// redirection behind a compound, and a `{name}` on a simple command — each
// of which the refusing grammar must still take.
//
// Without the bare brace, because with it a `{` where a command may begin is
// the reserved word before it is a descriptor's name, and `{v}</dev/null`
// there is a group and not this construct at all (#6616).
func TestAnFdVariableAfterACompoundAsksOnlyThere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src    string
		refuse bool
	}{
		{"{ :; } {v}</dev/null", false},
		{"if :; then :; fi {v}</dev/null", false},
		{"{ :; } 3</dev/null", true},
		{"true {v}</dev/null", true},
		{"for i (1) : {v}</dev/null", true},
	} {
		d := refusingDialect(false)
		d.FdVariableRefusedAfterACompound = c.refuse
		if _, err := syntax.Parse(c.src, d); err != nil {
			t.Errorf("%q (refusing=%v): %v", c.src, c.refuse, err)
		}
	}
}

// refusingDialect is the core grammar with the short forms the rows spell
// and the refusal on, and with or without the bare brace.
func refusingDialect(bareBrace bool) syntax.Dialect {
	d := syntax.Core()
	d.FdVariableRefusedAfterACompound = true
	d.OpenBraceNeedsNoBlank = bareBrace
	d.CloseBraceAlwaysReserved = bareBrace
	d.ShortForm = true
	d.ShortFormBody = true
	d.ForBraceBody = true
	d.Repeat = true
	d.Select = true
	return d
}
