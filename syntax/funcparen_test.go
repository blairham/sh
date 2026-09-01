// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// FuncDefAtParen decides which token a malformed function definition is
// blamed on, by deciding whether the parser is inside one at all.
//
// This names the flag rather than the shells that set it, which is the rule
// for a test in this package.
func TestFuncDefAtParenDecidesWhereTheParserStops(t *testing.T) {
	commits, waits := syntax.Core(), syntax.Core()
	commits.FuncDefAtParen = true

	// Inside a definition, looking for `)`, so the word is what is wrong.
	_, err := syntax.Parse("f ( x )", commits)
	if err == nil || !strings.Contains(err.Error(), "x") {
		t.Errorf("committing: got %v, want the word blamed", err)
	}
	// Never entered one, so the paren is.
	_, err = syntax.Parse("f ( x )", waits)
	if err == nil || !strings.Contains(err.Error(), "(") {
		t.Errorf("waiting: got %v, want the paren blamed", err)
	}

	// The word before the paren is not checked for being a name, which is how
	// a construct the dialect does not have gets there: `[[` is a function
	// name to a parser without `[[`, and a parser with it never reaches this
	// at all — which is why the flag alone does not decide the case.
	without := syntax.POSIX()
	without.FuncDefAtParen = true
	if _, err := syntax.Parse("[[ ( -n x ) ]]", without); err == nil {
		t.Error("`[[ ( -n x ) ]]` should not parse without the construct")
	}
	if _, err := syntax.Parse("[[ ( -n x ) ]]", commits); err != nil {
		t.Errorf("with the construct it is a condition, not a definition: %v", err)
	}

	// Both halves that a change here is most likely to break.
	for _, src := range []string{"f () { echo hi; }", "a=(1 2)"} {
		d := commits
		d.ArrayLiteral = true
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q should still parse: %v", src, err)
		}
	}
}
