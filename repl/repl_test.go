// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
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

// Without a terminal there is still a shell — there is just no editor.
//
// Run used to refuse, which is right about the editor and wrong about the
// shell: every shell in the panel, handed `-i` on a pipe, prints a prompt and
// runs the lines. What goes away is cursor movement, completion and history,
// because there is nothing to edit on.
func TestWithoutATerminalTheEditorGoesAwayAndTheShellDoesNot(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "echo one\necho two\n")
	r := newTestRunner(nil)
	// The commands' output is the *runner's*, not the repl's: the repl owns
	// the prompt and the runner owns everything a command prints.
	r.Stdout, r.Stderr = &out, &errs
	s := Shell{Runner: r, In: in, Out: &out, Err: &errs}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("refused a pipe: %v", err)
	}
	if got := out.String(); got != "one\ntwo\n" {
		t.Errorf("out = %q, want both lines run", got)
	}
	// The prompt goes to the error stream, which is what keeps
	// `sh -i < script > out` producing the commands' output and nothing else.
	if got := errs.String(); !strings.Contains(got, "$ ") {
		t.Errorf("err = %q, want a prompt on it", got)
	}
	if strings.Contains(out.String(), "$") {
		t.Errorf("out = %q, want no prompt on the output stream", out.String())
	}
}

// A construct spanning lines is the one thing a prompt needs that a script
// runner does not, and it is not the editor's doing — so it survives without
// a terminal.
func TestAConstructSpansLinesWithoutATerminal(t *testing.T) {
	var out strings.Builder
	in := readerFile(t, "for i in 1 2; do\necho \"n=$i\"\ndone\n")
	r := newTestRunner(nil)
	r.Stdout = &out
	s := Shell{Runner: r, In: in, Out: &out, Err: &strings.Builder{}}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "n=1\nn=2\n" {
		t.Errorf("out = %q, want the loop to have run once whole", got)
	}
}

// A backslash continuation is the case that shows an unfinished line being run
// early, and the loop above cannot: an unfinished construct parses to no
// statements, so running it does nothing visible. `echo one \` is a *finished*
// command in a file — the shells all print `one` for it — and a promise at a
// prompt, so running it when it arrives prints one line too many.
func TestABackslashHoldsTheLineWithoutATerminal(t *testing.T) {
	var out strings.Builder
	in := readerFile(t, "echo one \\\ntwo\n")
	r := newTestRunner(nil)
	r.Stdout = &out
	s := Shell{Runner: r, In: in, Out: &out, Err: &strings.Builder{}}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "one two\n" {
		t.Errorf("out = %q, want one command, not the first half and then both", got)
	}
}

// readerFile is standard input that is not a terminal, holding what would
// have been typed.
func readerFile(t *testing.T, s string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(s)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	return r
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
