// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package repl is the interactive surface: a prompt, a line editor, and the
// loop between them.
//
// It is the part that makes this a shell someone types into rather than a
// library that runs scripts. Everything it needs from the substrate was
// already there — the parser reports whether input ended part-way through a
// construct, which is what a continuation prompt is, and the interpreter runs
// one statement at a time — so this package is the terminal and the loop, and
// borrows the rest.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Shell is one interactive session.
type Shell struct {
	// Runner is the interpreter the lines are run in. It carries the state a
	// session accumulates — variables, functions, the working directory — so
	// it is the caller's rather than made here.
	Runner *interp.Runner

	// Dialect is the grammar lines are parsed with.
	Dialect syntax.Dialect

	// In, Out and Err are the terminal. In has to be one for the editor to
	// work; Run says so rather than guessing otherwise.
	In       *os.File
	Out, Err io.Writer

	// Report renders a parse failure the way this dialect does. Nil prints
	// the error plainly, which is what a caller without a dialect gets.
	Report func(err error) string
}

// Run reads, evaluates and prints until the input ends.
//
// It returns the status of the last command, which is what the shell exits
// with — the same thing a script's last command decides.
func (s Shell) Run(ctx context.Context) (int, error) {
	if !isTerminal(s.In) {
		// A prompt without a terminal is not a mistake to refuse: every shell
		// in the panel, given `-i` on a pipe, still prints a prompt and runs
		// the lines — it only says that job control is off. The *editor* is
		// what needs a terminal, and it is the editor that goes away.
		return s.runPlain(ctx)
	}
	state, err := makeRaw(s.In)
	if err != nil {
		return 0, err
	}
	// Deferred rather than restored at each return: a panic here would
	// otherwise leave the terminal with echo off, which is a broken terminal
	// and not merely a crash.
	defer func() { _ = state.restore() }()

	// A command runs with the terminal back in its own line discipline, so
	// ^C then reaches the foreground process group as a signal — and this
	// shell is in it. Without a handler the shell dies with the command,
	// which is the one failure that would make a prompt unusable.
	//
	// Caught rather than ignored, and the difference matters: an ignored
	// signal is *inherited* across exec, so children would stop dying on ^C
	// too. A handler is reset to the default in a child, so the child still
	// dies and the shell still does not.
	sig, stop := catchInterrupt()
	defer stop()

	ed := &editor{
		in: s.In, out: s.Out, comp: s.completer(),
		// The width comes from the input, which is the terminal; the output
		// may be a file the session was started with, and its size is not the
		// screen's.
		width: func() int { return terminalWidth(s.In) },
	}
	// What earlier sessions typed, and where to add what this one does. The
	// count is kept so only the new lines are written back: the rest are
	// already in the file, and appending them again doubles it every time a
	// shell is opened.
	hist := s.historyFile()
	ed.history = hist.load()
	loaded := len(ed.history)
	defer func() {
		if err := hist.save(ed.history[min(loaded, len(ed.history)):]); err != nil {
			s.errf("%v\n", err)
		}
	}()
	var pending strings.Builder
	for {
		line, err := ed.readLine(s.beforeReading(&pending))
		switch {
		case errors.Is(err, ErrInterrupted):
			// ^C abandons whatever was half-typed, including the earlier
			// lines of an unfinished construct — which is the whole point of
			// it at a continuation prompt.
			pending.Reset()
			continue
		case errors.Is(err, io.EOF):
			return s.status(), nil
		case err != nil:
			return s.status(), err
		}
		stmts, perr, ready := s.accept(&pending, ed.remember, line)
		if !ready {
			continue
		}
		if perr != nil {
			s.errf("%s", s.report(perr))
			continue
		}
		done := s.run(ctx, state, stmts)
		if sig.took() {
			// The terminal echoed `^C` where the cursor was and left it
			// there, so the next prompt would land on top of it.
			s.write("\n")
		}
		if done {
			return s.status(), nil
		}
	}
}

// run executes the statements of one accepted line, reporting whether the
// shell should stop.
//
// The terminal goes back to its own line discipline first. A command is not
// the editor: it may want echo, it may want ^C to interrupt it, and it will
// print lines that need the terminal translating them.
func (s Shell) run(ctx context.Context, state *terminalState, stmts []*syntax.File) bool {
	if err := state.restore(); err != nil {
		s.errf("%v\n", err)
	}
	defer func() {
		if _, err := makeRaw(s.In); err != nil {
			s.errf("%v\n", err)
		}
	}()
	return s.runStmts(ctx, stmts)
}

// runStmts executes the statements of one accepted line, reporting whether the
// shell should stop. Without a terminal there is nothing to hand back, which
// is the only difference between this and run.
func (s Shell) runStmts(ctx context.Context, stmts []*syntax.File) bool {
	for _, st := range stmts {
		if err := s.Runner.RunPart(ctx, st); err != nil {
			// Refused rather than silently skipped, the same way the script
			// driver does it.
			s.errf("%v\n", err)
			return false
		}
		if s.Runner.Exited() {
			return true
		}
	}
	return false
}

// beforeReading is everything that happens between one line and the next: what
// to say about the jobs, and what to prompt with.
//
// One place, used by both loops. They differ in how a line is read and in
// nothing else that happens first, and having written this twice is how the
// editor's copy came to be the one nothing exercised.
func (s Shell) beforeReading(pending *strings.Builder) string {
	continuing := pending.Len() > 0
	s.reportFinishedJobs(continuing)
	if continuing {
		return s.prompt("PS2", "> ")
	}
	return s.prompt("PS1", "$ ")
}

// reportFinishedJobs says what ended while the last command was running.
//
// Before the prompt rather than the moment the job ends, which is what every
// shell in the panel does and is the only place it can go: a line arriving
// half-typed-over would be unreadable, and the shell is inside the editor for
// all of the time between one command and the next.
//
// Not at a continuation prompt. The line being typed is unfinished, and
// putting a notice in the middle of it says the same thing about the display
// that arriving mid-line does.
func (s Shell) reportFinishedJobs(continuing bool) {
	if continuing {
		return
	}
	for _, line := range s.Runner.FinishedJobNotices() {
		s.errf("%s\n", line)
	}
}

// runPlain reads lines from something that is not a terminal.
//
// No editor, so no cursor movement, no completion and no history — there is
// nothing to edit on. What remains is what makes it a shell rather than a
// script runner: a prompt before each line, and a continuation prompt for a
// construct that has not finished. That is what the panel does with `-i` on a
// pipe, and it is also the only way to drive a prompt from a test.
//
// The prompt goes to the error stream, where a shell always puts it: the
// output of `sh -i < script > out` is the commands' output and nothing else.
func (s Shell) runPlain(ctx context.Context) (int, error) {
	in := bufio.NewReader(s.In)
	var pending strings.Builder
	for {
		s.errf("%s", s.beforeReading(&pending))

		line, err := in.ReadString('\n')
		if line == "" && err != nil {
			// End of input ends the session, exactly as ^D does at a
			// terminal. A final line without a newline is still a line,
			// which is why this asks about the text and not only the error.
			return s.status(), nil
		}
		line = strings.TrimSuffix(line, "\n")

		stmts, perr, ready := s.accept(&pending, nil, line)
		if !ready {
			// Nothing to run yet. accept returns no statements and no error
			// in that case, so this guard cannot change an outcome — it says
			// what the loop is doing, and the contract it relies on is
			// asserted in TestAcceptReturnsNothingUntilTheConstructIsDone.
			continue
		}
		if perr != nil {
			s.errf("%s", s.report(perr))
			continue
		}
		if s.runStmts(ctx, stmts) {
			return s.status(), nil
		}
	}
}

// accept adds a typed line to what is pending and says whether it is a command
// yet.
//
// Not ready means the construct has not finished and the next line continues
// it — the one thing an interactive shell needs from a parser that a script
// runner does not, and it was already there.
//
// The history is written here rather than by the caller, because *when* is the
// whole of the rule: only the finished construct is remembered, and only once.
// Remembering each continuation line as it was typed put a `for` loop in the
// history four times over — once per line and once entire — and left
// `do echo $i` there as something that can be recalled and cannot be run.
func (s Shell) accept(pending *strings.Builder, remember func(string), line string) ([]*syntax.File, error, bool) {
	pending.WriteString(line)
	pending.WriteString("\n")
	text := pending.String()

	p := syntax.NewParser(text, s.Dialect)
	stmts, err := collect(p)
	// Incomplete rather than incomplete-and-failed: input can be unfinished
	// without being wrong, and a here-document with no delimiter yet is
	// exactly that — the parser takes what it has and says there may be
	// more, which is an error to a script and a question to a prompt.
	if p.Incomplete() || endsWithContinuation(text) {
		return nil, nil, false
	}
	pending.Reset()
	if remember != nil {
		// Nil where there is nothing to recall with: a session without an
		// editor has no way to reach a history and no reason to keep one.
		remember(strings.TrimSuffix(text, "\n"))
	}
	return stmts, err, true
}

// endsWithContinuation reports whether the text ends with a backslash joining
// it to a line that has not been typed yet.
//
// The parser cannot answer this, and is right not to. `echo one \` at the end
// of a *file* is a finished command — the continuation joins it to nothing and
// the shells all print `one`. At a terminal the same text is a promise, and
// every shell in the panel gives a continuation prompt rather than running it.
// The difference is not in the text, so it belongs here, where the difference
// lives.
//
// Counted rather than looked for: `echo \\` ends with a backslash that is
// itself escaped, and is a finished command that prints one.
func endsWithContinuation(text string) bool {
	text = strings.TrimSuffix(text, "\n")
	n := 0
	for i := len(text) - 1; i >= 0 && text[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// collect reads every statement the parser can make of the text.
//
// The error is returned rather than reported, because the caller has to ask
// whether the input was merely unfinished before deciding it was wrong.
func collect(p *syntax.Parser) ([]*syntax.File, error) {
	var stmts []*syntax.File
	for {
		line, ok := p.NextLine()
		if !ok {
			return stmts, p.Err()
		}
		if err := p.Err(); err != nil {
			return nil, err
		}
		stmts = append(stmts, line)
	}
}

// prompt reads one of the prompt parameters, falling back to the usual text.
//
// Through GetVar rather than out of Vars, because Vars holds what this session
// assigned and the environment is a second source underneath it. A shell
// started from another shell is handed its prompt that way — `PS1='> ' sh` —
// and reading only the map dropped it: every one of the four uses an exported
// PS1, and we printed the default over the top of it.
//
// An empty value is a prompt of nothing rather than a missing one. `PS1=`
// silences the prompt in all four, and treating empty as absent made the one
// thing a person does to turn it off do the opposite.
func (s Shell) prompt(name, fallback string) string {
	if s.Runner == nil {
		return fallback
	}
	if v, ok := s.Runner.GetVar(name); ok {
		return v
	}
	return fallback
}

// historyFile is where this session reads and records its lines.
//
// HISTFILE and HISTFILESIZE come from the shell's own variables rather than
// from the process environment, because a session can set them at the prompt
// and mean it.
func (s Shell) historyFile() historyFile {
	if s.Runner == nil {
		return historyFile{}
	}
	home, _ := s.Runner.GetVar("HOME")
	return historyFrom(s.Runner.GetVar, home)
}

// completer answers Tab from this shell: its builtins, its functions, what is
// on its PATH and the files in its working directory.
//
// Built once per session rather than per keystroke for the names, which do not
// move; PATH and the directory are read each time because `cd` and an
// assignment both change them under it.
func (s Shell) completer() completer {
	if s.Runner == nil {
		return nil
	}
	return runnerCompleter{r: s.Runner}
}

// runnerCompleter reads the shell's state at the moment Tab is pressed.
type runnerCompleter struct{ r *interp.Runner }

func (c runnerCompleter) names() []string {
	names := append(c.r.BuiltinNames(), c.r.FuncNames()...)
	// The reserved words are commands too — `if` and `while` are what a line
	// most often starts with, and a completer that offered every builtin but
	// not those would feel broken.
	return append(names, reservedWords...)
}

func (c runnerCompleter) shell() shellCompleter {
	path, _ := c.r.GetVar("PATH")
	return shellCompleter{names: c.names(), path: path, dir: c.r.Dir}
}

func (c runnerCompleter) commands(prefix string) []string { return c.shell().commands(prefix) }
func (c runnerCompleter) files(prefix string) []string    { return c.shell().files(prefix) }

// reservedWords is the grammar's own vocabulary, which no builtin table holds.
var reservedWords = []string{
	"case", "do", "done", "elif", "else", "esac", "fi", "for",
	"function", "if", "in", "select", "then", "until", "while",
}

func (s Shell) status() int { return s.Runner.ExitStatus() }

func (s Shell) report(err error) string {
	if s.Report != nil {
		return s.Report(err)
	}
	return err.Error() + "\n"
}

func (s Shell) write(text string) {
	if s.Out != nil {
		_, _ = io.WriteString(s.Out, text)
	}
}

func (s Shell) errf(format string, a ...any) {
	if s.Err == nil {
		return
	}
	_, _ = fmt.Fprintf(s.Err, format, a...)
}
