// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// bareKeyword is the core with the anonymous function and the flag this file
// is about; keywordNoBare is the same grammar with only the flag off, which
// is what makes every row below a statement about the flag rather than about
// the constructs it stands beside.
func bareKeyword() Dialect {
	d := keywordNoBare()
	d.BareFunctionKeyword = true
	return d
}

func keywordNoBare() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.AnonymousFunction = true
	return d
}

// The keyword standing alone is a command: an anonymous function with no body
// at all, which is not the same node as one given an empty brace group. See
// [Dialect.BareFunctionKeyword], where the measurement is.
func TestTheKeywordAloneIsAnAnonymousFunctionWithNoBody(t *testing.T) {
	f, err := Parse("function\n", bareKeyword())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
	if !ok {
		t.Fatalf("came to %T, want an AnonFunc", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if !fn.Bare {
		t.Error("Bare is false, so nothing downstream can tell this from `function { }`")
	}
	if !fn.Keyword {
		t.Error("Keyword is false, so the spelling that was written is lost")
	}
	if fn.Body == nil {
		t.Fatal("Body is nil, which every reader of a body would have to guard")
	}
	// A body that was not written has no extent, and it stands where the
	// keyword ended so that the node's own End is the keyword's.
	if got, want := fn.Body.Pos(), fn.Body.End(); got != want {
		t.Errorf("the placeholder body runs %v..%v, want an empty extent", got, want)
	}
	// And the brace group somebody typed is a different tree, which is the
	// half that says Bare is not decoration.
	braced, err := Parse("function { :; }\n", bareKeyword())
	if err != nil {
		t.Fatalf("parse braced: %v", err)
	}
	if b, ok := braced.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc); !ok || b.Bare {
		t.Errorf("`function { :; }` came to %T with Bare=%v, want an AnonFunc that is not bare",
			braced.Stmts[0].Expr.(*Pipeline).Cmds[0], ok && b.Bare)
	}
}

// Where the keyword may stand alone, and where it may not. The set is
// measured on [Dialect.BareFunctionKeyword]; these rows are the flag's half
// of it, so they name tokens rather than a shell.
func TestWhereTheBareKeywordStands(t *testing.T) {
	for _, src := range []string{
		"function\n",
		"function",
		"function;\n",
		"function; echo after\n",
		"function | cat\n",
		"echo a | function\n",
		"function && echo after\n",
		"false || function\n",
		"( function )\n",
		"{ function }\n",
		"case x in x) function ;; esac\n",
		"function > out\n",
		"function 2> out\n",
		"function < in\n",
		"for i in 1; do function; done\n",
	} {
		mustParse(t, src, bareKeyword(), "the keyword stands alone here")
		// And the same line is a refusal with only the flag off, which is
		// what says the flag is what took it.
		mustFail(t, src, keywordNoBare(), "without the flag the keyword needs a name")
	}
}

// The reach stops short of the background operators, and of every reserved
// word but `}`: those are read as the name this form does not have, so the
// line is a refusal with the flag on as well as off.
func TestWhereTheBareKeywordDoesNotStand(t *testing.T) {
	for _, src := range []string{
		"function & echo after\n",
		"function &\n",
		"if true; then function fi\n",
		"while false; do function done\n",
		"case x in x) function esac\n",
	} {
		mustFail(t, src, bareKeyword(), "the keyword does not stand alone here")
	}
}

// A token that is not a word at all is named as the token it is, rather than
// described as a name that was wanted. The wording belongs to the dialect;
// what this asserts is the *kind* of failure, which is the one the grammar
// produces everywhere else (#3732).
func TestTheKeywordWithNoNameNamesTheToken(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{"function;\n", ";"},
		{"function &\n", "&"},
		{"function | echo x\n", "|"},
		{"function\n", "newline"},
		{"( function )\n", ")"},
	} {
		_, err := Parse(tc.src, keywordNoBare())
		se, ok := err.(*Error)
		if !ok {
			t.Fatalf("%q: came to %T, want a *syntax.Error", tc.src, err)
		}
		if se.Kind != ErrUnexpected {
			t.Errorf("%q: kind %v, want ErrUnexpected", tc.src, se.Kind)
		}
		if se.Token != tc.token {
			t.Errorf("%q: token %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}

// The bare form prints back as the keyword and nothing else, and reads back
// as the same command. Printing the placeholder body would hand back
// `function { }`, which parses to a different tree and answers a different
// status — a printer that changed a program at status 0 is the failure this
// row exists for.
func TestTheBareKeywordPrintsBackAsItself(t *testing.T) {
	for _, tc := range []struct {
		src    string
		redirs int
	}{
		{"function\n", 0},
		{"function >out\n", 1},
		{"function 2>out <in\n", 2},
		{"function | cat\n", 0},
	} {
		f, err := Parse(tc.src, bareKeyword())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		out := Print(f)
		if strings.Contains(out, "{") {
			t.Errorf("%q printed %q, which writes a body nobody wrote", tc.src, out)
		}
		back, err := Parse(out, bareKeyword())
		if err != nil {
			t.Fatalf("reparse %q: %v", out, err)
		}
		fn, ok := back.Stmts[0].Expr.(*Pipeline).Cmds[0].(*AnonFunc)
		if !ok || !fn.Bare {
			t.Errorf("%q printed %q, which reads back as %T", tc.src, out, back.Stmts[0].Expr.(*Pipeline).Cmds[0])
			continue
		}
		if len(fn.Redirs) != tc.redirs {
			t.Errorf("%q printed %q, which reads back with %d redirections, want %d",
				tc.src, out, len(fn.Redirs), tc.redirs)
		}
	}
}

// A name takes no newline, so the input running out where one belonged is the
// second position [Dialect.EndOfInputIsANewlineWhereNoneCouldStand] answers —
// the loop variable was the first, and it was the flag's whole name while it
// was the only one.
func TestTheKeywordsNameIsTheOtherPositionThatTakesNoNewline(t *testing.T) {
	appended := keywordNoBare()
	appended.EndOfInputIsANewlineWhereNoneCouldStand = true

	_, err := Parse("function", appended)
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("came to %T, want a *syntax.Error", err)
	}
	if se.Kind != ErrUnexpected || se.Token != "newline" {
		t.Errorf("kind %v token %q, want ErrUnexpected on `newline`", se.Kind, se.Token)
	}

	// With the flag off the same text runs out rather than meeting a token,
	// which is the other reading and the one the rest of the panel gives.
	_, err = Parse("function", keywordNoBare())
	se, ok = err.(*Error)
	if !ok {
		t.Fatalf("came to %T, want a *syntax.Error", err)
	}
	if se.Kind == ErrUnexpected && se.Token == "newline" {
		t.Error("the end of input was read as an appended newline with the flag off")
	}
}
