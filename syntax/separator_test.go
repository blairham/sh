// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// withCompounds is the core plus the two compound forms these cases are
// written with, so that a construct is a construct here and not a word.
//
// Flags are named and shells are not: what belongs to a dialect lives under
// dialect/.
func withCompounds() syntax.Dialect {
	d := syntax.Core()
	d.ArithCommand = true
	d.DoubleBracket = true
	return d
}

// A command standing straight after one that ended itself is a second
// statement with no separator between, which the list production does not
// have. It is only ever visible after a compound: a simple command's words
// absorb whatever follows, so `true echo x` is one command with an argument
// and there is nothing to refuse.
func TestTwoCommandsWithNoSeparatorAreRefused(t *testing.T) {
	for _, src := range []string{
		`(( 1 )) echo x`,
		`[[ -n x ]] echo y`,
		`{ :; } echo x`,
		`(echo a) echo b`,
		`if true; then :; fi echo z`,
		`for i in a; do :; done echo w`,
		`while false; do :; done echo v`,
		`case a in a) :;; esac echo u`,
		`f() { :; } echo t`,
		`(echo a) (echo b)`,
		`(echo a) >/dev/null echo b`,
		`{ :; } 2>/dev/null echo b`,
		`echo one; { :; } echo x`,
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.Parse(src, withCompounds()); err == nil {
				t.Fatalf("parsed, want a syntax error")
			}
		})
	}
}

// The token named is the one the list stopped on, and the expectation beside
// it comes from whatever was waiting for the list. Both halves are graded,
// because a diagnostic that names the end of the input instead is what this
// parser used to produce: it read on, absorbed the rest, and only noticed
// there was trouble once there was no more input to absorb.
func TestTheTokenAfterTheMissingSeparatorIsNamed(t *testing.T) {
	for _, c := range []struct {
		src      string
		token    string
		expected string
	}{
		{`(( 1 )) echo x`, "echo", ""},
		{`(echo a) (echo b)`, "(", ""},
		{`( (echo a) echo b )`, "echo", ")"},
		// The closer is named only where the construct had something in it,
		// which is measured — an empty one has nothing to be in the middle
		// of, and the shell that prints expectations prints none here.
		{`( fi )`, "fi", ""},
		{`{ (echo a) echo b; }`, "echo", "}"},
		{`if true; then (echo a) echo b; fi`, "echo", "fi"},
		{`while true; do (echo a) echo b; done`, "echo", "done"},
		{`for i in a; do (echo a) echo b; done`, "echo", "done"},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, err := syntax.Parse(c.src, withCompounds())
			var e *syntax.Error
			if !errors.As(err, &e) {
				t.Fatalf("parse %q: %v, want a *syntax.Error", c.src, err)
			}
			if e.Kind != syntax.ErrUnexpected {
				t.Errorf("kind %v, want %v", e.Kind, syntax.ErrUnexpected)
			}
			if e.Token != c.token {
				t.Errorf("token %q, want %q", e.Token, c.token)
			}
			if e.Expected != c.expected {
				t.Errorf("expected %q, want %q", e.Expected, c.expected)
			}
			if strings.Contains(e.Msg, "end of input") {
				t.Errorf("blamed the end of the input: %q", e.Msg)
			}
		})
	}
}

// A statement written with a separator after it is still a statement, and the
// list carries on: the refusal above must not reach the ordinary line.
func TestASeparatedListStillParses(t *testing.T) {
	for _, c := range []struct {
		src   string
		stmts int
	}{
		{`(echo a); echo b`, 2},
		{"(echo a)\necho b", 2},
		{`(echo a) & echo b`, 2},
		{`(( 1 )) | cat`, 1},
		{`{ :; } && echo x`, 1},
		{`if true; then :; fi; echo z`, 2},
		{`{ (echo a); echo b; }`, 1},
	} {
		t.Run(c.src, func(t *testing.T) {
			f, err := syntax.Parse(c.src, withCompounds())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			if len(f.Stmts) != c.stmts {
				t.Fatalf("%d statements, want %d", len(f.Stmts), c.stmts)
			}
		})
	}
}

// A short loop body is a whole statement, terminator included, so the loop is
// terminated by whatever its body took and the list may carry on after it. A
// body closed by its own word is not: `done` and `}` end the body without any
// separator reaching the enclosing list.
func TestAShortBodySTerminatorTerminatesTheLoopToo(t *testing.T) {
	d := syntax.Core()
	d.ShortLoop = true
	d.ForBraceBody = true
	d.CloseBraceAlwaysReserved = true
	d.ArithCommand = true
	for _, c := range []struct {
		src   string
		stmts int // 0 means the line must be refused
	}{
		{`for i (a b) echo $i; echo end`, 2},
		{"for i (a b) echo $i\necho end", 2},
		{`while (( i < 2 )) echo hi; echo end`, 2},
		{`for i (a b) for j (c d) echo $j; echo end`, 2},
		{`for i (a b) { echo $i; }; echo end`, 2},
		{`for i (a b) { echo $i; } echo end`, 0},
		{`for i in a b; do echo $i; done echo end`, 0},
		// A terminator a body took is that statement's alone: the statement
		// after it has to find one of its own, and a parser keeping the last
		// one it saw would let this line through.
		{`for i (a b) echo $i; (echo a) echo b`, 0},
		{`for i (a b) echo $i; { :; } echo b`, 0},
	} {
		t.Run(c.src, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if c.stmts == 0 {
				if err == nil {
					t.Fatalf("parsed, want a syntax error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(f.Stmts) != c.stmts {
				t.Fatalf("%d statements, want %d", len(f.Stmts), c.stmts)
			}
		})
	}
}

// What the parser accepts, the printer has to write back in a form that parses
// again — and the separator is exactly what a printer can drop without any
// other test noticing, since every case above hands it a tree it built itself.
func TestPrintingPutsTheSeparatorBack(t *testing.T) {
	for _, src := range []string{
		`(echo a); echo b`,
		`{ :; }; echo x`,
		`if true; then :; fi; echo z`,
		`(( 1 )); (( 2 )); echo done`,
		"(echo a)\necho b",
	} {
		t.Run(src, func(t *testing.T) {
			f, err := syntax.Parse(src, withCompounds())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out := syntax.Print(f)
			again, err := syntax.Parse(out, withCompounds())
			if err != nil {
				t.Fatalf("reparse %q: %v", out, err)
			}
			if len(again.Stmts) != len(f.Stmts) {
				t.Fatalf("printed %q: %d statements, want %d", out, len(again.Stmts), len(f.Stmts))
			}
		})
	}
}
