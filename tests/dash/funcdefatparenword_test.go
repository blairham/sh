// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// Every word but an assignment word announces a definition at the paren here
// — quoted, expanded, or holding an `=` that makes no assignment — and the
// name is refused once the parentheses close. Measured 2026-10-04 on dash
// 0.5.12:
//
//	x+=(a b)       word unexpected (expecting ")")
//	x+=()          Bad function name
//	a[1]=(x)       word unexpected (expecting ")")
//	"f"() { :; }   Bad function name
//	''() { :; }    Bad function name
//	a=(x)          "(" unexpected — an assignment word, the control
func TestAWordAtTheParenIsADefinitionUnlessItAssigns(t *testing.T) {
	for _, tc := range []struct{ src, token, msg string }{
		{`x+=(a b)`, "a", ""},
		{`x+=()`, "", "Bad function name"},
		{`a[1]=(x)`, "x", ""},
		{`"f"() { :; }`, "", "Bad function name"},
		{`''() { :; }`, "", "Bad function name"},
		{`a=(x)`, "(", ""},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%s: got %v, want a refusal", tc.src, err)
			continue
		}
		if tc.msg != "" && se.Msg != tc.msg {
			t.Errorf("%s: message %q, want %q", tc.src, se.Msg, tc.msg)
		}
		if tc.token != "" && se.Token != tc.token {
			t.Errorf("%s: refused %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}
