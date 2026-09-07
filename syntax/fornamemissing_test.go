// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// **A loop-variable position holding no word at all is the grammar's
// complaint, not the name check's.** `for` with nothing after it, a newline
// straight after it, `for ;` — none of those is a word that could be tested
// against [isName], and three of the four dialects say so as an unexpected
// token where the fourth gives its one bad-loop-variable sentence.
//
// It reached the name check with the end of input in hand instead, which
// named `end of input` as though a script had written it, and — because
// StatusForParseError routes ErrForName to ForNameStatus — carried 1 on every
// route where the panel carries 2 and 3 (#1319).
//
// The tests name the flags and never a shell. What each dialect answers is in
// dialect/<shell>, and the recorded panel is in the corpus.

// missingNameError parses src and returns the refusal.
func missingNameError(t *testing.T, src string, d syntax.Dialect) *syntax.Error {
	t.Helper()
	_, err := syntax.Parse(src, d)
	if err == nil {
		t.Fatalf("%q parsed; every shell in the panel refuses it", src)
	}
	e, ok := err.(*syntax.Error)
	if !ok {
		t.Fatalf("%q: %T, want *syntax.Error", src, err)
	}
	return e
}

// With neither flag — the answer ksh93 and zsh give — a token that is not a
// word is refused as a token, and the end of the input is an unfinished
// construct naming the `for` that opened it.
func TestALoopVariableThatIsNoWordIsAGrammarFailure(t *testing.T) {
	d := loops()
	for _, tc := range []struct {
		src   string
		kind  syntax.ErrorKind
		token string
	}{
		{"for", syntax.ErrUnterminated, ""},
		{"for\ndo :; done\n", syntax.ErrUnexpected, "newline"},
		{"for ;", syntax.ErrUnexpected, ";"},
		{"for ; in a b\n", syntax.ErrUnexpected, ";"},
		{"select", syntax.ErrUnterminated, ""},
		{"select ;", syntax.ErrUnexpected, ";"},
	} {
		e := missingNameError(t, tc.src, d)
		if e.Kind != tc.kind {
			t.Errorf("%q: Kind = %v, want %v — a missing name is not a bad one", tc.src, e.Kind, tc.kind)
		}
		if e.Token != tc.token {
			t.Errorf("%q: Token = %q, want %q", tc.src, e.Token, tc.token)
		}
	}
}

// The unfinished form carries the construct that opened, which is what the
// two dialects reaching it name — one the innermost keyword, one the last
// token consumed. Both are the `for` itself, because no clause of it began.
func TestAnUnfinishedLoopHeaderNamesTheLoop(t *testing.T) {
	e := missingNameError(t, "for", loops())
	if e.Construct != "for" || e.Innermost != "for" {
		t.Errorf("Construct %q, Innermost %q — want the loop named by both", e.Construct, e.Innermost)
	}
	if e.LastToken != "for" {
		t.Errorf("LastToken = %q, want the keyword, which is all there was", e.LastToken)
	}
}

// ForNonWordIsANameError takes it back to the name check, which is the one
// dialect that words every bad loop variable the same way whatever stands
// there.
func TestTheNameErrorFlagKeepsEveryTokenOnTheNameCheck(t *testing.T) {
	d := loops()
	d.ForNonWordIsANameError = true
	for _, src := range []string{"for", "for\ndo :; done\n", "for ;", "for ; in a b\n"} {
		e := missingNameError(t, src, d)
		if e.Kind != syntax.ErrForName {
			t.Errorf("%q: Kind = %v, want ErrForName under the flag", src, e.Kind)
		}
	}
}

// ForNameEndOfInputIsANewline is the other dialect's answer, and it moves
// only the end-of-input row: a newline is what that shell has left over,
// because it is what it terminates its input with.
func TestTheEndOfInputIsANewlineUnderThatFlag(t *testing.T) {
	d := loops()
	d.ForNameEndOfInputIsANewline = true
	e := missingNameError(t, "for", d)
	if e.Kind != syntax.ErrUnexpected || e.Token != "newline" {
		t.Errorf("Kind %v Token %q, want an unexpected `newline'", e.Kind, e.Token)
	}
	// On the line the text ran out on, rather than the line after it — the
	// newline is a token that stood there, not the end of the file.
	if e.Pos.Line != 1 {
		t.Errorf("Pos.Line = %d, want 1", e.Pos.Line)
	}
	// And nothing else moves: a token that is present is still that token.
	if got := missingNameError(t, "for ;", d); got.Token != ";" {
		t.Errorf("`for ;` Token = %q, want `;` — the flag is about the end of input alone", got.Token)
	}
}

// The two flags are independent, and the name-error one wins where both are
// set: it decides *whether the grammar is asked at all*, and the other only
// spells the token once it has been.
func TestTheNameErrorFlagIsAskedFirst(t *testing.T) {
	d := loops()
	d.ForNonWordIsANameError = true
	d.ForNameEndOfInputIsANewline = true
	if e := missingNameError(t, "for", d); e.Kind != syntax.ErrForName {
		t.Errorf("Kind = %v, want ErrForName", e.Kind)
	}
}

// The control, and the boundary the change had to keep: a word that *is*
// present and is not a name is the name check's, in every combination of the
// flags. Nothing above may reach it.
func TestAPresentWordThatIsNoNameStaysOnTheNameCheck(t *testing.T) {
	for _, name := range []string{"plain", "name error", "end of input newline", "both"} {
		d := loops()
		switch name {
		case "name error":
			d.ForNonWordIsANameError = true
		case "end of input newline":
			d.ForNameEndOfInputIsANewline = true
		case "both":
			d.ForNonWordIsANameError, d.ForNameEndOfInputIsANewline = true, true
		}
		e := missingNameError(t, "for 1x in a b; do :; done\n", d)
		if e.Kind != syntax.ErrForName {
			t.Errorf("%s: Kind = %v, want ErrForName — `1x` is a word and a bad name", name, e.Kind)
		}
		if e.Token != "1x" {
			t.Errorf("%s: Token = %q, want the word as written", name, e.Token)
		}
	}
}

// And the other control: an ordinary unfinished construct is untouched, so
// the change is the loop header's and not the end of input's in general.
func TestAnUnfinishedConstructWithNoNameInItIsUnchanged(t *testing.T) {
	d := loops()
	d.ForNameEndOfInputIsANewline = true
	for _, src := range []string{"while", "if", "until"} {
		e := missingNameError(t, src, d)
		if e.Kind != syntax.ErrUnterminated {
			t.Errorf("%q: Kind = %v, want ErrUnterminated — no newline is invented here", src, e.Kind)
		}
	}
}
