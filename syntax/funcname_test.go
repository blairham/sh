// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
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

// TestASimpleCommandBodyFollowsTheFlag — one grammar refuses `f() echo hi`
// and three read it as a one-command body; the flag says which this is.
func TestASimpleCommandBodyFollowsTheFlag(t *testing.T) {
	strict := Core()
	strict.FuncBodyMustBeCompound = true
	if _, err := Parse(`f() echo hi`, strict); err == nil {
		t.Error("the refusing grammar accepted a simple body")
	} else if !strings.Contains(err.Error(), "echo") {
		t.Errorf("refusal = %v, want the token named", err)
	}
	for _, src := range []string{`f() { echo hi; }`, `f() if true; then :; fi`, `f() ( : )`} {
		if _, err := Parse(src, strict); err != nil {
			t.Errorf("%s: a compound body refused: %v", src, err)
		}
	}
	if _, err := Parse(`f() echo hi; f`, Core()); err != nil {
		t.Errorf("the accepting grammar refused: %v", err)
	}
}

// TestARefusedFunctionBodyNamesTheTokenItBeganWith — the refusal is decided
// only once the body has been read, so the token it names has to be the one
// saved before reading it rather than wherever the parser ended up.
//
// It is an ErrUnexpected and not a bare syntax error, which is what lets a
// dialect word it and echo the offending line: this path used to build its own
// sentence, so the dialect that repeats the source printed one line where the
// shell it grades against prints two.
func TestARefusedFunctionBodyNamesTheTokenItBeganWith(t *testing.T) {
	strict := Core()
	strict.FuncBodyMustBeCompound = true
	for _, c := range []struct{ src, token string }{
		{`f() echo hi; f`, "echo"},
		{`f() x=1; f`, "x=1"},
		{`f() >out; f`, ">"},
	} {
		_, err := Parse(c.src, strict)
		var se *Error
		if !errors.As(err, &se) {
			t.Errorf("%s: refusal = %v, want a syntax error", c.src, err)
			continue
		}
		if se.Kind != ErrUnexpected {
			t.Errorf("%s: kind = %v, want an unexpected token", c.src, se.Kind)
		}
		if se.Token != c.token {
			t.Errorf("%s: token = %q, want %q", c.src, se.Token, c.token)
		}
	}
	// The line the body began on, not the line the parser stopped on: an echo
	// of the offending line quotes the wrong one otherwise.
	_, err := Parse("f()\necho hi\n", strict)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("refusal = %v, want a syntax error", err)
	}
	if se.Pos.Line != 2 {
		t.Errorf("line = %d, want the body's own", se.Pos.Line)
	}
}

// TestAFunctionWithNoBodyAtAllNamesWhatStoodThere — `f() ;` has no body for
// any grammar, refusing or not, and every shell measured names the token
// rather than describing the function. Running out of input instead is the
// unterminated kind, with no construct left open to name.
func TestAFunctionWithNoBodyAtAllNamesWhatStoodThere(t *testing.T) {
	for _, d := range []Dialect{Core(), POSIX()} {
		_, err := Parse(`f() ;`, d)
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("refusal = %v, want a syntax error", err)
		}
		if se.Kind != ErrUnexpected || se.Token != ";" {
			t.Errorf("kind %v token %q, want the token named", se.Kind, se.Token)
		}
		_, err = Parse(`f()`, d)
		if !errors.As(err, &se) {
			t.Fatalf("refusal = %v, want a syntax error", err)
		}
		if se.Kind != ErrUnterminated || se.Construct != "" {
			t.Errorf("kind %v construct %q, want an end of input with nothing open",
				se.Kind, se.Construct)
		}
	}
}
