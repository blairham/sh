// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	. "github.com/blairham/sh/syntax"
)

// Input does not always arrive whole. A shell reading its program off a
// descriptor takes a piece, runs it, and comes back for more — so the parser
// has to be able to say which line of the *input* a piece began at, or every
// diagnostic after the first piece names line 1.
func TestNewParserAtNumbersFromTheLineItIsGiven(t *testing.T) {
	p := NewParserAt("echo one\necho two\n", Core(), 7)
	for _, want := range []int{7, 8} {
		line, ok := p.NextLine()
		if !ok {
			t.Fatalf("no line where %d was expected", want)
		}
		if got := line.Stmts[0].Pos().Line; got != want {
			t.Errorf("line numbered %d, want %d", got, want)
		}
	}
	if _, ok := p.NextLine(); ok {
		t.Error("a third line where the input had two")
	}
}

// NewParser is NewParserAt from line 1, and nothing about a whole input
// changes because the other constructor exists.
func TestNewParserIsNewParserAtLineOne(t *testing.T) {
	one, _ := NewParser("echo hi\n", Core()).NextLine()
	at, _ := NewParserAt("echo hi\n", Core(), 1).NextLine()
	if one.Stmts[0].Pos() != at.Stmts[0].Pos() {
		t.Errorf("NewParser gave %v and NewParserAt gave %v", one.Stmts[0].Pos(), at.Stmts[0].Pos())
	}
}

// An offset stays relative to the text handed in, because it is only ever
// used to quote what a node was written as, and that text is what is here.
func TestNewParserAtKeepsOffsetsRelativeToItsOwnText(t *testing.T) {
	line, ok := NewParserAt("echo hi\n", Core(), 42).NextLine()
	if !ok {
		t.Fatal("no line")
	}
	if got := line.Stmts[0].Pos().Offset; got != 0 {
		t.Errorf("offset %d, want 0", got)
	}
}

// EndsWithContinuation is the question a parser is right not to answer: `echo
// one \` is a finished command at the end of a file and a promise at the end
// of what has been read so far, and only the reader knows which it is.
//
// Counted rather than looked for — an escaped backslash ends nothing.
func TestEndsWithContinuation(t *testing.T) {
	for _, c := range []struct {
		text string
		more bool
	}{
		{"echo one", false},
		{"echo one \\", true},
		{"echo one \\\n", true},
		{`echo \\`, false},
		{`echo \\\`, true},
		{"", false},
		{"\\", true},
	} {
		if got := EndsWithContinuation(c.text); got != c.more {
			t.Errorf("EndsWithContinuation(%q) = %v, want %v", c.text, got, c.more)
		}
	}
}
