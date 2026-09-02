// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The one thing an interactive shell needs from the parser that a script
// runner does not: telling input that has not finished from input that is
// wrong. A continuation prompt is that question, and nothing else.
func TestUnfinishedInputAsksForMore(t *testing.T) {
	for _, c := range []struct {
		name       string
		text       string
		unfinished bool
	}{
		{"a complete command", "echo hi\n", false},
		{"two of them", "echo one; echo two\n", false},
		{"an unclosed quote", "echo \"one\n", true},
		{"an unclosed single quote", "echo 'one\n", true},
		{"a loop with no done", "for i in a b\ndo\n", true},
		{"the same loop finished", "for i in a b\ndo\necho $i\ndone\n", false},
		{"an unclosed if", "if true\nthen\n", true},
		{"the same if finished", "if true\nthen\necho y\nfi\n", false},
		{"a trailing pipe", "echo hi |\n", true},
		{"a trailing &&", "true &&\n", true},
		{"an unclosed brace group", "{ echo hi\n", true},
		{"an unclosed subshell", "( echo hi\n", true},
		{"an unclosed expansion", "echo ${x\n", true},
		// A line continuation is the one case the parser cannot answer and
		// is right not to — see endsWithContinuation.
		{"a line continuation", "echo one \\\n", false},
		// Wrong is not the same as unfinished, and must not sit at a
		// continuation prompt forever.
		{"a stray closing brace", "}\n", false},
		{"a stray done", "done\n", false},
		{"a stray fi", "fi\n", false},
	} {
		p := syntax.NewParser(c.text, syntax.Core())
		_, err := collect(p)
		unfinished := err != nil && p.Incomplete()
		if unfinished != c.unfinished {
			t.Errorf("%s: unfinished=%v, want %v (err %v)", c.name, unfinished, c.unfinished, err)
		}
	}
}

// collect hands back every statement on the line, because a line may hold
// more than one and each is run in turn.
func TestCollectReadsEveryStatement(t *testing.T) {
	p := syntax.NewParser("echo one; echo two\necho three\n", syntax.Core())
	stmts, err := collect(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 2 {
		t.Errorf("got %d lines, want 2", len(stmts))
	}
}

// A trailing backslash is a promise at a terminal and a finished command in a
// file, so the shell answers it rather than the parser.
func TestATrailingBackslashAsksForMore(t *testing.T) {
	for _, c := range []struct {
		text string
		more bool
	}{
		{"echo one \\\n", true},
		{"echo one\n", false},
		// An escaped backslash is not a continuation, and two of them are
		// not either.
		{"echo \\\\\n", false},
		{"echo \\\\\\\n", true},
		{"", false},
		{"\n", false},
	} {
		if got := endsWithContinuation(c.text); got != c.more {
			t.Errorf("%q gave %v, want %v", c.text, got, c.more)
		}
	}
}

// A prompt comes from the parameter when one is set, and from the usual text
// when it is not.
func TestPrompts(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil)}
	if got := s.prompt("PS1", "$ "); got != "$ " {
		t.Errorf("unset PS1 gave %q, want the default", got)
	}
	s = Shell{Runner: newTestRunner(map[string]string{"PS1": "sh> "})}
	if got := s.prompt("PS1", "$ "); got != "sh> " {
		t.Errorf("set PS1 gave %q, want sh> ", got)
	}
	// An empty one is not a prompt, so the default stands rather than
	// leaving the user with no marker at all.
	s = Shell{Runner: newTestRunner(map[string]string{"PS1": ""})}
	if got := s.prompt("PS1", "$ "); got != "$ " {
		t.Errorf("empty PS1 gave %q, want the default", got)
	}
}

// Run refuses anything that is not a terminal rather than editing blind.
func TestRunNeedsATerminal(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil), Out: &strings.Builder{}}
	if _, err := s.Run(t.Context()); err == nil {
		t.Error("ran without a terminal, want it refused")
	}
}

// ^C at the prompt and ^C during a command are different events reaching the
// shell two different ways, and it has to survive both.
//
// At the prompt the terminal is raw with ISIG off, so ^C is a byte the editor
// returns ErrInterrupted for and no signal is sent at all. While a command
// runs the terminal is back in its own line discipline, so ^C reaches the
// whole foreground process group — this shell included.
func TestAnInterruptIsRememberedAndForgotten(t *testing.T) {
	in, stop := catchInterrupt()
	defer stop()
	if in.took() {
		t.Error("reported an interrupt before any arrived")
	}
	in.hit.Store(true)
	if !in.took() {
		t.Error("did not report the interrupt")
	}
	if in.took() {
		t.Error("reported the same interrupt twice — the prompt would get two newlines")
	}
}
