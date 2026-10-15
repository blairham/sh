// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A refused word that begins with `{` behind a compound command is named
// whole after a compound closed by a reserved word, in the grammar that reads
// a bare `{` as a word of its own, and the brace alone after every other kind.
// See Parser.wholeBraceWord.
//
// The rows hold the word fixed and move the compound, and the last pair holds
// the compound fixed and moves the word, so both halves of the key are varied.
func TestABraceWordBehindACompoundIsNamedByItsKind(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"if :; then :; fi {foo}", "{foo}"},
		{"while false; do :; done {foo}", "{foo}"},
		{"for i (1) { : } {foo}", "{foo}"},
		{"repeat 1 do :; done {foo}", "{foo}"},
		{"if :; then :; fi >/dev/null {foo}", "{foo}"},
		{"{ :; } {foo}", "{"},
		{"( : ) {foo}", "{"},
		{"[[ -n x ]] {foo}", "{"},
		{"(( 1 )) {foo}", "{"},
		{"if :; then :; fi {foo", "{foo"},
		{"if :; then :; fi { :; }", "{"},
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

// An arithmetic command's redirection refused, in the grammar that takes none
// there: the operator is blamed where nothing stands in front of it, and a
// number or a `{name}` in front of it is blamed itself, carrying the operator
// for the dialect that names that instead (#6552). The flag off reads every
// one. See [syntax.Dialect.ArithCommandTakesNoRedirection].
func TestARedirectionBehindAnArithmeticCommand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src, want, op string
		fdVar         bool
	}{
		{"(( 1 )) </dev/null", "<", "", false},
		{"(( 1 )) 3</dev/null", "3", "<", false},
		{"(( 1 )) 3>>/dev/null", "3", ">>", false},
		{"(( 1 )) {v}</dev/null", "{v}", "<", true},
		{"if (( 1 )) >/dev/null; then :; fi", ">", "", false},
	} {
		d := syntax.Core()
		d.FdVariableRedirections = true
		if _, err := syntax.Parse(c.src, d); err != nil {
			t.Errorf("%q without the flag: %v", c.src, err)
		}
		d.ArithCommandTakesNoRedirection = true
		_, err := syntax.Parse(c.src, d)
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err %v, want a syntax.Error", c.src, err)
			continue
		}
		if se.Token != c.want || se.FdVariable != c.fdVar || se.IONumberOperator != c.op {
			t.Errorf("%q: blamed %q (fd variable %v, operator %q), want %q (%v, %q)",
				c.src, se.Token, se.FdVariable, se.IONumberOperator, c.want, c.fdVar, c.op)
		}
	}
	d := syntax.Core()
	d.ArithCommandTakesNoRedirection = true
	for _, src := range []string{"[[ -n x ]] </dev/null", "for ((;0;)); do :; done >/dev/null"} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
