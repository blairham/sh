// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// The looser half of "a body ends at the closing parenthesis": the last line
// of the text inside `$( )` read as a delimiter **prefix**.
//
// heredocparen_test.go is the strict half — the delimiter with the `)`
// immediately behind it, which can be asked of every line because nothing but
// a delimiter is followed by one. This one can only be asked of the last line,
// and the difference is the whole of what makes it safe.
//
// The question is asked of the *inside* of the parentheses, because that is
// the text the rule is about: the lexer reads it in place where the grammar
// can find the `)` and an interpreter re-reads it at expansion time where it
// cannot, and both routes have to agree. See [Parser.InsideProgramParentheses].

// withPrefixAtEnd is the grammar of the one shell that reads it that way.
func withPrefixAtEnd() Dialect {
	d := Core()
	d.HeredocEndsAtClosingParen = true
	d.HeredocLastLineIsADelimiterPrefix = true
	return d
}

// insideParens parses text as the inside of parentheses holding a program,
// which is what an interpreter hands this package at expansion time.
func insideParens(t *testing.T, src string, d Dialect) *File {
	t.Helper()
	p := NewParser(src, d)
	p.InsideProgramParentheses()
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return f
}

// heredocBody is the body of the first here-document in a file.
//
// Reached through the one shape every case here writes — a statement holding a
// pipeline holding a simple command — rather than through a general walk,
// because a walk that quietly found nothing would read as an empty body.
func heredocBody(t *testing.T, f *File) string {
	t.Helper()
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*Pipeline)
		if !ok {
			continue
		}
		for _, cmd := range pipe.Cmds {
			simple, ok := cmd.(*SimpleCmd)
			if !ok {
				continue
			}
			for _, r := range simple.Redirs {
				if r.Heredoc != nil {
					return r.Heredoc.Literal()
				}
			}
		}
	}
	t.Fatal("the file holds no here-document")
	return ""
}

// The last line of the text is the delimiter plus whatever follows it, and
// what follows it is program text again.
//
// Measured against bash 5.3.20 — the table is in
// [Dialect.HeredocLastLineIsADelimiterPrefix] — and the rows here are the ones
// that separate the readings. `Ex` is the one a rule written around a blank
// gets wrong, and `EE` is the one written around "the remainder cannot begin
// with the delimiter".
func TestTheLastLineInsideParensIsReadAsADelimiterPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, body, rest string }{
		{"a blank then nothing", "cat <<E\nw\nE ", "w\n", ""},
		{"a command after it", "cat <<E\nw\nE x", "w\n", "x"},
		{"a command with an argument", "cat <<E\nw\nE x y", "w\n", "x y"},
		{"no blank at all", "cat <<E\nw\nEx", "w\n", "x"},
		{"a remainder starting with the delimiter", "cat <<E\nw\nEE", "w\n", "E"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := insideParens(t, c.src, withPrefixAtEnd())
			if got := heredocBody(t, f); got != c.body {
				t.Errorf("body = %q, want %q", got, c.body)
			}
			// What the remainder became, as the printer writes it back, so
			// the assertion is about the tree rather than about the bytes the
			// lexer stopped on.
			rest := strings.TrimSpace(Print(&File{Stmts: f.Stmts[1:]}))
			if got := rest; got != c.rest {
				t.Errorf("what followed the delimiter = %q, want %q", got, c.rest)
			}
		})
	}
}

// And the control that decides the whole shape: a body line that merely begins
// with the delimiter, with the document closed properly below it, is body.
//
// A rule asked of every line would end the document at `EXTRA` and hand
// `XTRA` to the parser. Measured: bash answers `[EXTRA]`, so it is a recovery
// at the end of the text and not a prefix match as the lines go by.
func TestABodyLineBeginningWithTheDelimiterIsStillBody(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, body string }{
		{
			name: "closed on its own line below",
			src:  "cat <<E\nEXTRA\nE\n",
			body: "EXTRA\n",
		},
		{
			name: "and closed by the prefix rule on the line after it",
			src:  "cat <<E\nEXTRA\nE ",
			body: "EXTRA\n",
		},
		{
			name: "several of them",
			src:  "cat <<E\nEXTRA\nEVEN\nE\n",
			body: "EXTRA\nEVEN\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := heredocBody(t, insideParens(t, c.src, withPrefixAtEnd())); got != c.body {
				t.Errorf("body = %q, want %q", got, c.body)
			}
		})
	}
}

// It is the dialect's and the caller's, and either one missing leaves the body
// running to the end of the text.
//
// Two negatives rather than one, because they fail the same way and for
// different reasons: a dialect without the rule is every shell but one, and a
// parser nobody told it was reading the inside of parentheses is every other
// read this package does — a script, a `-c` line, an `eval`.
func TestTheDelimiterPrefixNeedsBothTheDialectAndTheParentheses(t *testing.T) {
	t.Parallel()
	const src = "cat <<E\nw\nE x"
	const whole = "w\nE x"

	if got := heredocBody(t, insideParens(t, src, withParenEnd())); got != whole {
		t.Errorf("without the dialect flag the body is %q, want %q", got, whole)
	}
	// The same text read as an ordinary program, which is the plain-file
	// control the measurement turns on: `cat <<E` / `w` / `E x` in a file
	// gives cat the whole of it in every shell of the panel.
	p := NewParser(src, withPrefixAtEnd())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if got := heredocBody(t, f); got != whole {
		t.Errorf("outside parentheses the body is %q, want %q", got, whole)
	}
}

// The remark goes with it, on the line the delimiter was found as a prefix on.
//
// The shell this is measured from writes `warning: here-document at line 1
// delimited by end-of-file (wanted `E')` in exactly these cases and writes
// nothing for a document closed on its own line. Both halves are here, because
// dropping the remark would trade one wrong answer for another and raising it
// for the well-formed shape would be a warning nobody asked for.
func TestThePrefixMatchRemarksThatTheDocumentEndedAtEndOfInput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src string
		want      bool
		line      int32
	}{
		{"the prefix rule fired", "cat <<E\nw\nE ", true, 3},
		{"a command after the delimiter", "cat <<E\nw\nE x", true, 3},
		{"closed on its own line", "cat <<E\nw\nE\n", false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := NewParser(c.src, withPrefixAtEnd())
			p.InsideProgramParentheses()
			p.Parse()
			var got *Remark
			for i, r := range p.Remarks() {
				if r.Kind == RemarkHeredocAtEOF {
					got = &p.Remarks()[i]
				}
			}
			if (got != nil) != c.want {
				t.Fatalf("remark present = %v, want %v", got != nil, c.want)
			}
			if got != nil && got.Pos.Line != c.line {
				t.Errorf("the remark names line %d, want %d", got.Pos.Line, c.line)
			}
		})
	}
}
