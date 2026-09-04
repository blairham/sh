// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A function name may carry `-` and `.` where the dialect allows the
// punctuation; a dialect that commits at the paren without allowing it
// refuses the name once the parens close.

func TestPunctuatedFunctionNamesFollowTheFlag(t *testing.T) {
	allow := Core()
	if !allow.FunctionNamePunctuation {
		t.Fatal("the core should allow the punctuation three of the four accept")
	}
	for _, src := range []string{`f-g(){ echo ok; }`, `a.b(){ echo ok; }`, `function f-g { echo ok; }`} {
		if _, err := Parse(src, allow); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
	}

	strict := Core()
	strict.FunctionNamePunctuation = false
	strict.FuncDefAtParen = true
	if _, err := Parse(`f-g(){ echo ok; }`, strict); err == nil {
		t.Error("a committing dialect without the punctuation accepted the name")
	} else if !strings.Contains(err.Error(), "Bad function name") {
		t.Errorf("refusal = %v, want the name blamed", err)
	}
	// The shape that reaches the parens before the name question stays on
	// its own diagnosis — `[[` is a word to the dialect without the
	// conditional, and its `(` commits to a definition of a function
	// called `[[` whose parens never close.
	strict.DoubleBracket = false
	if _, err := Parse(`[[ ( -n x ) ]]`, strict); err == nil ||
		strings.Contains(err.Error(), "Bad function name") {
		t.Errorf("the unexpected-word path was rerouted: %v", err)
	}

	posix := POSIX()
	if _, err := Parse(`a=()`, allow); err != nil {
		t.Errorf("an empty array read as a function: %v", err)
	}
	if _, err := Parse(`f-g(){ echo ok; }`, posix); err == nil {
		t.Error("POSIX accepted a name it does not have")
	}
}
