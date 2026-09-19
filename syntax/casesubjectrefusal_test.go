// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A `case` subject that is not a word is the ordinary unexpected-token
// refusal, which is what lets every dialect word it the way it words every
// other one.
//
// It was a sentence of this parser's own — "expected a word after `case`" —
// for as long as the production existed, which made it the one place a
// refusal carried no token at all. Nothing downstream could recover the
// token, so a dialect had no way to say what it says everywhere else.
func TestACaseSubjectThatIsNotAWordNamesTheToken(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src   string
		token string
		class syntax.TokenClass
	}{
		{"case ; in x) ;; esac", ";", syntax.ClassOperator},
		{"case && in x) ;; esac", "&&", syntax.ClassOperator},
		{"case | in x) ;; esac", "|", syntax.ClassOperator},
		{"case ;; in x) ;; esac", ";;", syntax.ClassOperator},
		{"case ) in x) ;; esac", ")", syntax.ClassOperator},
		{"case\nin x) ;; esac", "newline", syntax.ClassNewline},
	} {
		_, err := syntax.Parse(tc.src, syntax.POSIX())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Kind != syntax.ErrUnexpected {
			t.Errorf("%q: kind = %v, want ErrUnexpected", tc.src, se.Kind)
		}
		if se.Token != tc.token {
			t.Errorf("%q: token = %q, want %q", tc.src, se.Token, tc.token)
		}
		if se.Class != tc.class {
			t.Errorf("%q: class = %v, want %v", tc.src, se.Class, tc.class)
		}
	}
}

// What would have stood there is a word of any spelling, and the refusal says
// so as a *class* rather than as a spelling.
//
// The two are not interchangeable downstream: the dialects that print an
// expectation quote a spelling and leave a class bare, so a subject refusal
// carrying `Expected: "word"` with the flag unset would come out
// `(expecting "word")` — a sentence that reads like a real one and is not.
// See Error.ExpectedIsAClass.
func TestTheExpectationAfterCaseIsAClassAndNotASpelling(t *testing.T) {
	t.Parallel()
	_, err := syntax.Parse("case ; in x) ;; esac", syntax.POSIX())
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Expected != "word" {
		t.Errorf("expected = %q, want %q", se.Expected, "word")
	}
	if !se.ExpectedIsAClass {
		t.Error("ExpectedIsAClass is false, so a dialect would quote the class")
	}

	// The other half: a refusal whose expectation really is one word leaves
	// the flag clear, so the two are told apart rather than being one value
	// that happens to be right for the newer path.
	_, err = syntax.Parse("for in x; do :; done", syntax.POSIX())
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Expected != "do" {
		t.Errorf("expected = %q, want %q", se.Expected, "do")
	}
	if se.ExpectedIsAClass {
		t.Error("ExpectedIsAClass is set for a one-word expectation")
	}
}

// The subject is a word position, so the flag that reads a run-out at a
// `case` word as the newline that would have ended the line reads it there
// too.
//
// Dialect.CaseWordRunsOutAsANewline was named and documented for the arm's
// pattern alone, on a sweep that asked every neighboring construct and not
// the one word inside `case` it had not reached.
func TestACaseSubjectRunOutIsANewlineWhereTheFlagIsSet(t *testing.T) {
	t.Parallel()
	asNewline, asRunOut := syntax.POSIX(), syntax.POSIX()
	asNewline.CaseWordRunsOutAsANewline = true

	_, err := syntax.Parse("case", asNewline)
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Kind != syntax.ErrUnexpected || se.Class != syntax.ClassNewline {
		t.Errorf("kind %v class %v, want ErrUnexpected and ClassNewline",
			se.Kind, se.Class)
	}
	if se.Pos.Line != 1 {
		t.Errorf("line = %d, want 1 — the line the `case` is on", se.Pos.Line)
	}

	// Without the flag the input ran out, which is what the other columns
	// say and what the construct-naming wordings need.
	_, err = syntax.Parse("case", asRunOut)
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Kind != syntax.ErrUnterminated {
		t.Errorf("kind = %v, want ErrUnterminated", se.Kind)
	}
	if se.Construct != "case" {
		t.Errorf("construct = %q, want %q", se.Construct, "case")
	}
	// The expectation is still a class, because what was missing is still a
	// word — the run-out wordings quote it or not on the same flag.
	if se.Expected != "word" || !se.ExpectedIsAClass {
		t.Errorf("expected %q class=%v, want \"word\" and true",
			se.Expected, se.ExpectedIsAClass)
	}
}
