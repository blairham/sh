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
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/panicguard"
	"github.com/blairham/sh/internal/secret"
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

	// Style is what this dialect does to a prompt parameter's value before
	// it is drawn. The zero value draws it as it stands.
	Style PromptStyle

	// Editor is what this dialect draws while a line is being typed. The
	// zero value draws nothing of its own.
	Editor EditorStyle

	// Clock is what a prompt with the time in it reads. Nil is the real one.
	Clock func() time.Time

	// Gate and Events are the session's policy and its observer, carried
	// straight through from the front end and used here for the one file
	// this package opens itself: the history. Both nil is what a shell
	// without a policy is, and costs a nil check.
	//
	// The values are the same ones the Runner holds — a session cannot be
	// gated for what a script does and ungated for what the prompt does, or
	// the boundary has a hole shaped exactly like `HISTFILE=/somewhere`.
	Gate   interp.Gate
	Events interp.Sink

	// Session identifies this run, and is the same string the Runner carries
	// and every event of this run is stamped with.
	//
	// It is here rather than made in this package because it is the *front
	// end's* identity for a run: what a prompt records about a command and
	// what the event stream records about the same command are two accounts of
	// one session, and two identities made independently would leave them
	// looking joinable and not being. Empty is a run the front end gave no
	// identity, which records fine and joins to nothing.
	Session string

	// PanicTrace prints the stack of an interpreter bug caught while running
	// a line, as well as the report that one was caught. The default is off
	// because a trace at a prompt scrolls the session away and buries the
	// line that said what happened.
	//
	// A field rather than an environment variable read here: this package is
	// a library in the same sense interp is, and a library that consults the
	// process is one two of them in one program cannot agree about. The front
	// end reads the variable and says so — see driver's panic.go.
	PanicTrace bool

	// counts are the running totals a prompt can draw. Set by the loops,
	// which are the only things that know a line has been accepted.
	counts *counts

	// Name is what the shell calls itself, for a prompt that draws it —
	// bash's `\s`. Empty draws nothing, which is what a caller that has not
	// said gets.
	Name string
}

// Run reads, evaluates and prints until the input ends.
//
// It returns the status of the last command, which is what the shell exits
// with — the same thing a script's last command decides.
func (s Shell) Run(ctx context.Context) (int, error) {
	// What earlier sessions typed. Loaded before the two loops part company,
	// because both of them number their prompts from where it left off:
	// measured, bash given `-i` on a pipe with three lines in HISTFILE draws
	// `!4 #1` at its first prompt, editor or no editor.
	hist := s.historyFile()
	earlier := hist.load(ctx)
	s.counts = &counts{history: len(earlier)}
	// Where this session records a command and what came of it. Opened here
	// rather than in either loop so the two cannot disagree about whether a
	// session keeps blocks, which is the mistake beforeReading already
	// documents having been made once with what goes between lines.
	store := s.blocksStore()
	defer func() { _ = store.Close() }()
	// And what it keeps of what those commands printed, when the session asked
	// for it. Nil is the default, and the reason it is the default is that a
	// captured stream is not a terminal to the child on the other end of it —
	// see blocks.Capture.Stream.
	capture := s.captureOutput()
	if !IsTerminal(s.In) {
		// A prompt without a terminal is not a mistake to refuse: every shell
		// in the panel, given `-i` on a pipe, still prints a prompt and runs
		// the lines — it only says that job control is off. The *editor* is
		// what needs a terminal, and it is the editor that goes away.
		return s.runPlain(ctx, store, capture)
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

	ed := s.newEditor()
	// Where to add what this session types. The count of what was already
	// there is kept so only the new lines are written back: the rest are in
	// the file already, and appending them again doubles it every time a
	// shell is opened.
	ed.history = earlier
	loaded := len(earlier)
	defer func() {
		if err := hist.save(ctx, ed.history[min(loaded, len(ed.history)):]); err != nil {
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
		// Counted here rather than per line read. A construct typed over
		// four lines is one entry in the history and one command, which is
		// what bash draws: `: x`, a for loop, `: y` numbered 1, 2, 3 and not
		// 1, 5, 6. accept already keeps the history that way — it remembers
		// the whole accumulated text as one line — and only the numbering
		// disagreed with it.
		stmts, text, perr, ready := s.take(&pending, s.recording(ed.remember), line)
		if !ready {
			continue
		}
		if perr != nil {
			// Remembered but not run, and the two numbers say so: measured,
			// bash draws `!3 #2` at the prompt after a line that would not
			// parse.
			//
			// Not a block either, and for the same reason it is not a command:
			// nothing ran. The store records what a shell did, and a line the
			// parser refused never became something it could do.
			s.errf("%s", s.report(perr))
			continue
		}
		b := s.beginBlock(text)
		done := s.run(ctx, state, stmts)
		s.closeBlock(ctx, store, capture, b)
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
//
// This is where a session survives an interpreter bug, and the unit is the
// line because the line is what a person typed: one bad line costs that line
// and the shell keeps its variables, its functions, its jobs and its
// directory. Here rather than in either loop, so that the editor's loop and
// the piped one cannot disagree about it — and inside run's restore, so the
// report is written with the terminal in its own line discipline, exactly as a
// command's own output is. interp goes on panicking, which is correct for a
// library; see internal/panicguard.
func (s Shell) runStmts(ctx context.Context, stmts []*syntax.File) (done bool) {
	if s.guard().Do(func() { done = s.runEach(ctx, stmts) }) {
		// The line never finished, so it has no status of its own and must
		// not keep the one before it: `$?` says it failed, and the `&&` on
		// the next line reads it the way it reads any other failure.
		s.Runner.SetExitStatus(panicguard.Status)
		return false
	}
	return done
}

// guard is what a typed line is run behind.
func (s Shell) guard() panicguard.Guard {
	return panicguard.Guard{Name: s.Name, Err: s.Err, Trace: s.PanicTrace}
}

// runEach is runStmts without the guard around it.
func (s Shell) runEach(ctx context.Context, stmts []*syntax.File) bool {
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
		return s.prompt("PS2", or(s.Style.DefaultContinued, "> "))
	}
	return s.prompt("PS1", or(s.Style.Default, "$ "))
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
func (s Shell) runPlain(ctx context.Context, store *blocks.Store, capture *blocks.Capture) (int, error) {
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

		stmts, text, perr, ready := s.take(&pending, nil, line)
		if !ready {
			// Nothing to run yet. take returns no statements and no error in
			// that case, so this guard cannot change an outcome — it says
			// what the loop is doing, and the contract it relies on is
			// asserted in TestAcceptReturnsNothingUntilTheConstructIsDone.
			continue
		}
		if perr != nil {
			s.errf("%s", s.report(perr))
			continue
		}
		b := s.beginBlock(text)
		done := s.runStmts(ctx, stmts)
		s.closeBlock(ctx, store, capture, b)
		if done {
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
// The accumulated text is returned as well as the statements, because two
// things want it and only this function has it. The history wants the finished
// construct, which is what remember is handed; the block store wants the same
// text for a different reason — a block records the line *as typed*, before
// expansion, and an event cannot supply that, since what an event carries for
// an exec is argv after expansion. `echo $HOME` is a block whose command is
// `echo $HOME`.
func (s Shell) accept(pending *strings.Builder, remember func(string), line string) ([]*syntax.File, string, error, bool) {
	pending.WriteString(line)
	pending.WriteString("\n")
	text := pending.String()

	p := syntax.NewParser(text, s.Dialect)
	// Unconditionally, unlike the script path: every shell in the panel
	// expands aliases at a prompt, and the dialect's answer is only about a
	// *non-interactive* one. This is the place that knows there is a person
	// at the keyboard.
	if s.Runner != nil {
		p.Aliases = s.Runner.LookupAlias
	}
	stmts, err := collect(p)
	// Incomplete rather than incomplete-and-failed: input can be unfinished
	// without being wrong, and a here-document with no delimiter yet is
	// exactly that — the parser takes what it has and says there may be
	// more, which is an error to a script and a question to a prompt.
	if p.Incomplete() || endsWithContinuation(text) {
		// Kept for the continuation prompt: what the next line goes on with
		// is what the parser is still inside, and this is the only place it
		// is known.
		s.counted().open = p.Open()
		return nil, "", nil, false
	}
	pending.Reset()
	// The construct is whole, so nothing is waiting on the next line.
	s.counted().open = nil
	text = strings.TrimSuffix(text, "\n")
	if remember != nil {
		// Nil where there is nothing to recall with: a session without an
		// editor has no way to reach a history and no reason to keep one.
		remember(text)
	}
	return stmts, text, err, true
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
//
// The counting is syntax's, because a prompt is no longer the only reader that
// has to ask: a shell taking its program off a descriptor a line at a time
// meets the same half-written command, and two answers to one question is how
// they come to differ.
func endsWithContinuation(text string) bool { return syntax.EndsWithContinuation(text) }

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
		return s.render(v)
	}
	return s.render(fallback)
}

// render turns a prompt parameter's value into the text to draw.
//
// Each time it is drawn rather than once when it is set. That is the whole
// point of expanding it: a prompt holding `$PWD` is expected to follow the
// directory, and one holding a command substitution to run the command again.
func (s Shell) render(value string) string {
	if value == "" {
		return value
	}
	// Three passes, in the order the panel draws them: the dialect's table of
	// codes, then expansion, then the character that stands for the history
	// number.
	value = s.table(value)
	if s.Style.Expand {
		value = s.Runner.Expand(value)
	}
	return s.history(value)
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
	h := historyFrom(s.Runner.GetVar, home)
	// The session's boundary, so an open this package makes is asked about
	// the same way one the interpreter makes is.
	h.bound = boundary.Boundary{Gate: s.Gate, Events: s.Events}
	return h
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

// or is the first of two that has something in it.
//
// For a default a dialect may or may not have said anything about: an empty
// one means it has not said, and the substrate's own text stands.
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// take adds one line to what is pending and says what became of it: nothing
// yet, something that would not parse, or statements to run.
//
// The counting is here because this is where the decision already is, and
// because writing it in both loops is how the terminal one came to have a
// copy that nothing exercised — which the note on beforeReading warned about,
// having already happened once with what goes between lines.
func (s Shell) take(pending *strings.Builder, remember func(string), line string) ([]*syntax.File, string, error, bool) {
	blank := strings.TrimSpace(pending.String()+line) == ""
	stmts, text, perr, ready := s.accept(pending, remember, line)
	if !ready {
		return nil, "", nil, false
	}
	s.counted().accepted(blank, perr == nil)
	return stmts, text, perr, true
}

// recording is what an accepted line goes into the session's history
// through, and it is where a credential is noticed.
//
// The check is on the way *in* rather than on the way out, because the notice
// is the whole difference between a feature and a shell that has lost your
// line: printed here it lands directly under what was typed, while the file
// is written as the shell exits, which is the worst moment there is for
// something a person is meant to read. The file itself is guarded separately,
// in withoutCredentials, so the invariant does not depend on this.
//
// The line is still remembered, and that is the interesting half. Measured on
// 2026-09-05: bash 5.3.15 given `HISTIGNORE='*SECRET*'` drops a matching line
// from the history list entirely — its own `history` builtin cannot see it —
// while zsh 5.9.2 given `HISTORY_IGNORE='*SECRET*'` keeps it in the list and
// leaves it out of the file. zsh's is the answer taken here, for a reason
// that is about this feature rather than about zsh: a line that was refused
// is very often a line about to be retyped — the token had a character
// missing — and a scrubber that also takes away the up arrow is one people
// work around by turning it off. Nothing that was on the screen anyway is
// being protected by forgetting it.
func (s Shell) recording(remember func(string)) func(string) {
	if remember == nil {
		return nil
	}
	return func(line string) {
		if rule, found := secret.Default().Match(line); found {
			s.errf("%s: history: not saving this line (matched %s)\n", or(s.Name, "sh"), rule)
		}
		remember(line)
	}
}

// newEditor is the line editor this shell types into.
//
// A method rather than a literal in the loop so that what a dialect says
// reaches the editor is something a test can look at: the editor itself is
// only built where there is a terminal, and a dialect's answer dropped on the
// way looks exactly like a dialect that did not answer.
func (s Shell) newEditor() *editor {
	return &editor{
		in: s.In, out: s.Out, comp: s.completer(),
		// What this dialect marks an abandoned line with, which is `^C` in
		// two of the four and nothing in the other two.
		interrupt:       s.Editor.Interrupt,
		listQuery:       s.Editor.ListQuery,
		listQueryEchoes: s.Editor.ListQueryEchoesTheKey,
		listQueryStrict: s.Editor.ListQueryAcceptsOnlyYesOrNo,
		// The width comes from the input, which is the terminal; the output
		// may be a file the session was started with, and its size is not the
		// screen's.
		width: func() int { return terminalWidth(s.In) },
	}
}
