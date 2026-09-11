// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"

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

// A prompt comes from the parameter when there is one, and from the usual
// text when there is not.
//
// "When there is one" includes two cases this used to get wrong, and the panel
// is unanimous on both. A PS1 inherited from the environment is used: a shell
// started from another shell is handed its prompt that way, and reading only
// the assigned variables printed the default over the top of it. And an empty
// PS1 is a prompt of nothing rather than a missing one — `PS1=` is how a
// person turns the prompt off, and falling back to the default made it the
// one thing that could not be done.
func TestPrompts(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		env  []string
		want string
	}{
		{"unset", nil, nil, "$ "},
		{"assigned", map[string]string{"PS1": "sh> "}, nil, "sh> "},
		{"empty is silence, not absence", map[string]string{"PS1": ""}, nil, ""},
		{"inherited from the environment", nil, []string{"PS1=env> "}, "env> "},
		{"inherited and empty", nil, []string{"PS1="}, ""},
		{
			"an assignment covers what was inherited",
			map[string]string{"PS1": "mine> "},
			[]string{"PS1=env> "},
			"mine> ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRunner(tc.vars)
			r.Env = tc.env
			s := Shell{Runner: r}
			if got := s.prompt("PS1", "$ "); got != tc.want {
				t.Errorf("prompt = %q, want %q", got, tc.want)
			}
		})
	}
}

// And with no runner at all there is still a default to print.
func TestAPromptWithoutARunner(t *testing.T) {
	var s Shell
	if got := s.prompt("PS1", "$ "); got != "$ " {
		t.Errorf("prompt = %q, want the default", got)
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

// What ended while the last command ran is reported before the next prompt,
// which is the only place it can go: the shell is inside the editor for all of
// the time between one command and the next, and a line arriving half typed
// over would be unreadable.
func TestFinishedJobsAreReportedBeforeThePrompt(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "true &\nsleep 0.2\necho after\n")
	r := newTestRunner(nil)
	r.Stdout, r.Stderr = &out, &errs
	r.JobControl = true
	sem := interp.PosixSemantics()
	sem.AnnouncesBackgroundJob = interp.No
	sem.JobsShowBackgroundCommand = interp.Yes
	r.Semantics = &sem
	// And a monitor, which is what a prompt runs: the finish notice rides on
	// it, and with it off no shell in the panel says anything about a job
	// that ended (#1738).
	r.Terminal = true
	r.SetInteractiveMonitor()
	s := Shell{Runner: r, In: in, Out: &out, Err: &errs}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "Done") {
		t.Errorf("err = %q, want the finished job reported", errs.String())
	}
	// Beside the prompt rather than in the output, the same as the prompt
	// itself: `sh -i < in > out` collects what the commands printed.
	if strings.Contains(out.String(), "Done") {
		t.Errorf("out = %q, want the notice off the output stream", out.String())
	}
}

// Not at a continuation prompt: the line being typed is unfinished, and a
// notice in the middle of it says the same thing about the display that one
// arriving mid-line does.
func TestNoNoticeInTheMiddleOfAConstruct(t *testing.T) {
	var out, errs strings.Builder
	// The job ends while the loop is still being typed.
	in := readerFile(t, "for i in 1; do\ntrue &\nsleep 0.2\ndone\necho after\n")
	r := newTestRunner(nil)
	r.Stdout, r.Stderr = &out, &errs
	r.JobControl = true
	sem := interp.PosixSemantics()
	sem.AnnouncesBackgroundJob = interp.No
	sem.JobsShowBackgroundCommand = interp.Yes
	r.Semantics = &sem
	// And a monitor, which is what a prompt runs: the finish notice rides on
	// it, and with it off no shell in the panel says anything about a job
	// that ended (#1738).
	r.Terminal = true
	r.SetInteractiveMonitor()
	s := Shell{Runner: r, In: in, Out: &out, Err: &errs}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// It still arrives, at the next prompt that is not a continuation.
	if !strings.Contains(errs.String(), "Done") {
		t.Errorf("err = %q, want the notice once the construct finished", errs.String())
	}
	if n := strings.Count(errs.String(), "Done"); n != 1 {
		t.Errorf("%d notices, want one", n)
	}
}

// A here-document holds the line open. Unfinished input is not wrong input,
// so the parser reports no error for it — and a prompt that waited only for
// errors would have run the command with an empty body the moment the
// operator's line ended.
func TestAHereDocumentHoldsThePromptOpen(t *testing.T) {
	var out strings.Builder
	in := readerFile(t, "cat <<EOF\nbody\nEOF\necho after\n")
	r := newTestRunner(nil)
	r.Stdout = &out
	s := Shell{Runner: r, In: in, Out: &out, Err: &strings.Builder{}}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "body\nafter\n" {
		t.Errorf("out = %q, want the body read and then the next command", got)
	}
}

// A final line with no newline is still a line. It is the last thing typed
// before the input ends, and asking only whether the read failed loses it.
func TestAFinalLineWithoutANewlineStillRuns(t *testing.T) {
	var out strings.Builder
	in := readerFile(t, "echo one")
	r := newTestRunner(nil)
	r.Stdout = &out
	s := Shell{Runner: r, In: in, Out: &out, Err: &strings.Builder{}}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "one\n" {
		t.Errorf("out = %q, want the last line run", got)
	}
}

// accept returns nothing at all until the construct is finished — not a
// half-parsed statement and not the error that says it is unfinished. The
// loops rely on it: a caller that ran what it was handed anyway would run
// nothing, which is why the guard against it cannot be observed.
func TestAcceptReturnsNothingUntilTheConstructIsDone(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil), Out: &strings.Builder{}}
	var pending strings.Builder
	stmts, _, err, ready := s.accept(&pending, nil, "for i in 1 2; do")
	if ready {
		t.Fatal("said it was ready with the loop unfinished")
	}
	if stmts != nil || err != nil {
		t.Errorf("accept gave %v, %v — want nothing until it is finished", stmts, err)
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
