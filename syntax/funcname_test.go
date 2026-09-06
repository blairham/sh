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

// TestTheFunctionNamePunctuationIsTheMeasuredSet — the flag is a character
// class, not a list of the two marks that happened to be needed first.
//
// The twelve here are the punctuation every panel shell but dash parses in a
// function name in every position, in both definition forms; the ones below
// them split the panel and are deliberately outside the class. Asserted as
// whole names rather than as a set of bytes, because what the parser is asked
// is whether a name is one.
func TestTheFunctionNamePunctuationIsTheMeasuredSet(t *testing.T) {
	allow := Core()
	for _, name := range []string{
		"f!g", "f#g", "f%g", "f+g", "f,g", "f-g", "f.g", "f/g", "f:g",
		"f@g", "f]g", "f^g",
		// Every position, and a name that is nothing but the mark.
		":f", "f:", ":", "+f", "f+", "-f", "f-", "@f", "f@",
		// The one the daily driver needs, and a name outside ASCII.
		":zi-reload-and-run", "\u00e9f", "\u276ex\u276f",
	} {
		for _, src := range []string{name + `(){ echo ok; }`, `function ` + name + ` { echo ok; }`} {
			if _, err := Parse(src, allow); err != nil {
				t.Errorf("%s: refused where the flag allows: %v", src, err)
			}
		}
	}
	// Outside the class: the panel splits on these, so the flag does not
	// carry them and the word stays an ordinary command.
	for _, name := range []string{"f*g", "f?g", "f[g", "f{g", "f}g", "f~g"} {
		if _, err := Parse(name+`(){ echo ok; }`, allow); err == nil {
			t.Errorf("%s: accepted a mark the panel disagrees about", name)
		}
	}
	// And `=` is excluded whatever the flag says, or an empty array becomes
	// a definition of a function whose name ends in one.
	if _, err := Parse(`a=()`, allow); err != nil {
		t.Errorf("an empty array read as a function: %v", err)
	}
	if _, err := Parse(`f=g(){ echo ok; }`, allow); err == nil {
		t.Error("a name carrying `=` was accepted")
	}
	// The flag is what decides: none of it parses without one.
	posix := POSIX()
	for _, name := range []string{":f", "f+g", "a%b", "f-g", "a.b"} {
		if _, err := Parse(name+`(){ echo ok; }`, posix); err == nil {
			t.Errorf("%s: POSIX accepted a name it does not have", name)
		}
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

// TestARedirectionOnlyBodyFollowsItsOwnFlag — a third answer to the body
// question, narrower than the compound one: this grammar takes the simple
// command and refuses only what it redirects.
//
// The two flags are separate because the refusals are not nested. The compound
// one rejects `f() echo hi` outright and names the command; this one accepts
// it, and rejects `f() echo hi >out` at the operator with the command already
// read.
func TestARedirectionOnlyBodyFollowsItsOwnFlag(t *testing.T) {
	noRedir := Core()
	noRedir.FuncBodyTakesNoRedirection = true
	for _, c := range []struct{ src, token string }{
		{`f() >out; f`, ">"},
		{`f() echo hi >out; f`, ">"},
		{`f() x=1 >out; f`, ">"},
		{`f() 2>&1; f`, ">&"},
		{`f() cat <in; f`, "<"},
	} {
		_, err := Parse(c.src, noRedir)
		var se *Error
		if !errors.As(err, &se) {
			t.Errorf("%s: refusal = %v, want a syntax error", c.src, err)
			continue
		}
		if se.Kind != ErrUnexpected || se.Token != c.token {
			t.Errorf("%s: kind %v token %q, want the operator named as %q",
				c.src, se.Kind, se.Token, c.token)
		}
		if !se.Redirect {
			t.Errorf("%s: the error does not say it was a redirection", c.src)
		}
	}
	// A body with no redirection, and a redirection on a *compound* body:
	// both accepted, which is what makes this about the body rather than
	// about redirecting a function.
	for _, src := range []string{`f() echo hi; f`, `f() { echo hi; } >out; f`, `f() x=1; f`} {
		if _, err := Parse(src, noRedir); err != nil {
			t.Errorf("%s: refused: %v", src, err)
		}
	}
	// And the flag is what decides: the same text parses without it.
	if _, err := Parse(`f() >out; f`, Core()); err != nil {
		t.Errorf("the accepting grammar refused: %v", err)
	}
}

// TestEmptyParensAreOneTokenToTheGrammarThatSaysSo — the last token an
// end-of-input error names is `()` there and `)` everywhere else, which is the
// only place the granularity shows.
func TestEmptyParensAreOneTokenToTheGrammarThatSaysSo(t *testing.T) {
	joined := Core()
	joined.EmptyParensAreOneToken = true
	for _, c := range []struct {
		d    Dialect
		want string
	}{{joined, "()"}, {Core(), ")"}} {
		_, err := Parse(`f()`, c.d)
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("refusal = %v, want a syntax error", err)
		}
		if se.LastToken != c.want {
			t.Errorf("last token = %q, want %q", se.LastToken, c.want)
		}
	}
	// Only until something else is read: `f() ;` names the semicolon in both.
	_, err := Parse(`f() ;`, joined)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("refusal = %v, want a syntax error", err)
	}
	if se.Token != ";" {
		t.Errorf("token = %q, want the semicolon", se.Token)
	}
}

// TestAMissingFunctionBodyIsMarkedAsOne — the fact one dialect locates
// differently, recorded on the error rather than deduced from its kind: the
// two shapes below are an unexpected token and an end of input, and both are a
// body that never began.
func TestAMissingFunctionBodyIsMarkedAsOne(t *testing.T) {
	for _, src := range []string{`f() ;`, `f()`, `f() &`, `f() }`} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%s: refusal = %v, want a syntax error", src, err)
		}
		if !se.FuncBody {
			t.Errorf("%s: not marked as a missing function body", src)
		}
	}
	// A body that began and then ran out is not this: the input is unfinished
	// inside the body rather than missing one.
	for _, src := range []string{`f() {`, `f() { echo hi`, `if true`} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("%s: refusal = %v, want a syntax error", src, err)
		}
		if se.FuncBody {
			t.Errorf("%s: wrongly marked as a missing function body", src)
		}
	}
	// Nor is one where a newline came between the parens and the failure:
	// measured, the dialect that drops the line for this puts it back there.
	_, err := Parse("f()\n;", Core())
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("refusal = %v, want a syntax error", err)
	}
	if se.FuncBody {
		t.Error("a body expected on a later line was marked as a missing one")
	}
}
