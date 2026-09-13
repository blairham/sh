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
	"strconv"
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
	//
	// An io.Reader rather than an *os.File because only the editor needs a
	// descriptor, and Run already parts company over exactly that: without a
	// terminal it takes runPlain, which reads lines and draws prompts and
	// wants nothing a reader cannot do. Widening it costs one type assertion,
	// in the branch that was already there, and it is what lets a front end
	// whose input is not a descriptor at all — driver.Shell.Stdin is an
	// io.Reader for the same reason — still offer `-i`. Handing such a caller
	// a nil descriptor instead would be a prompt that reads nothing, which
	// looks like a shell and is not one.
	In       io.Reader
	Out, Err io.Writer

	// Report renders a parse failure the way this dialect does. Nil prints
	// the error plainly, which is what a caller without a dialect gets.
	Report func(err error) string

	// ParseFailureStatus is what `$?` becomes after a line this dialect's
	// parser refused. Nil leaves the status alone, which is what a caller
	// without a dialect gets — the same fallback Report has, and for the same
	// reason: there is no value that is right for every shell.
	//
	// There is one, per shell, and it is not a constant across them. Measured
	// through a pseudo-terminal and again with `-i` on a pipe, `false`, then a
	// line the parser refuses, then `$?`: 2 in bash 5.3 and in the same bash
	// invoked as sh, 2 in dash, 3 in ksh93, 1 in zsh, and 258 in bash 3.2 —
	// which is the one column that answers a *different* number here than it
	// does as a program's exit status, since 258 cannot be one. The value does
	// not depend on what ran before: `true` in place of `false` moves no cell,
	// so this is a status being set rather than one being left.
	//
	// A function of the error rather than a number, because the dialect
	// already answers it that way for a script and the two routes must not
	// drift: some failures a shell finds while parsing are *run*-time errors
	// to it — a `for` with a name that is not one — and carry a different
	// status from a syntax error. driver hands this the same answer it gives a
	// script, so one table decides both.
	//
	// Ours left `$?` where the last command put it, which is the shape a
	// missing status always has: a refused line reported the status of the
	// command *before* it, so a prompt drawing a failure indicator, and the
	// hooks a prompt hands `$?` to, read "the last thing succeeded" at the
	// exact moment it did not (#1299).
	ParseFailureStatus func(err error) int

	// Remark renders something the parser had to say about input it accepted
	// anyway — the whole line, ready to write, or empty for nothing to say.
	//
	// Nil says nothing, which is what a caller without a dialect gets and
	// what three of the four dialects say about everything. The one remark
	// there is belongs to the end of the input: a here-document whose
	// delimiter never arrived is accepted and run by every shell in the
	// panel, and bash warns about it — at a prompt as well as in a script,
	// measured. The script routes have said it since there were remarks; the
	// prompt had no way to (#1892).
	Remark func(rk syntax.Remark) string

	// AskAgainAfterARefusedToken keeps the continuation prompt for a construct
	// the parser has *refused*, rather than refusing it where it stands.
	//
	// The zero value refuses at once, which is what a caller without a dialect
	// gets and what three of the four panel shells do. It is not a fallback
	// chosen for being safe: a prompt that asks for another line after
	// `if; then` reads the next command as part of the construct it has
	// already refused, so the command a person typed next disappears (#1893).
	//
	// See [Shell.refusedForGood] for the measurement and for which failures
	// this is about — a token the grammar cannot take, never input that has
	// merely not finished.
	AskAgainAfterARefusedToken bool

	// CountSessionLines numbers each line by how many the session has read,
	// rather than starting every construct's text at line 1.
	//
	// One dialect names a line at a prompt at all, and it counts the session:
	// the second line typed is line 2, whether what it says about it is a
	// parse failure or a command it could not find. The other three name no
	// line there, so the number is invisible to them and the zero value keeps
	// the numbering a construct's own.
	//
	// It is the position the *parser* is given rather than a wording, which
	// is what makes the two routes agree: a diagnostic about the text as
	// written comes from the parse, and one about running it comes from the
	// tree's own positions, and only a number carried in at the parse reaches
	// both (#2022).
	CountSessionLines bool

	// Style is what this dialect does to a prompt parameter's value before
	// it is drawn. The zero value draws it as it stands.
	Style PromptStyle

	// Editor is what this dialect draws while a line is being typed. The
	// zero value draws nothing of its own.
	Editor EditorStyle

	// KeyBindings is what a person has rebound, as a table from the bytes a
	// key sends to what it should do — one of this editor's actions, or the
	// name of one the shell performs itself. Nil — and an empty table — is a
	// session where every key does what the editor does by default, which is
	// what a caller without a `bindkey` gets.
	//
	// A function rather than a value because it is asked per keystroke: the
	// command that changes it is one a person runs at the prompt, and a value
	// read once at the start of a session would take effect on the next
	// shell. See bindings.go for what the editor does with it.
	//
	// **Asked for a keymap, which is what makes a command map live.** Both
	// shells with an editor keep a table per keymap, and both already accepted
	// bindings into the vi command map before this editor had a command mode
	// to read it in — so what was written there was stored and never fired.
	// The editor says which map it is in and the dialect answers for that one;
	// see Keymap, and vi.go for the mode itself.
	KeyBindings func(Keymap) map[string]Binding

	// ViEditing reports whether this session edits the vi way — whether
	// Escape leaves insert mode for a command mode, and whether
	// KeymapViCommand is ever asked for.
	//
	// Nil is a session with no command mode, which is what a front end that
	// has not said gets and what both dialects without a line editor are.
	//
	// **A dialect's answer and not the core's**, although the core has an
	// editing mode of its own. Measured under a pty: `bindkey -v` in zsh 5.9.2
	// turns the command mode on and leaves `set -o` reporting `vi off`, while
	// `set -o vi` in the same shell turns it on and reports it. Two commands,
	// one of which the core knows about, and either is enough — so the shell
	// has to be asked rather than the option read. In bash there is one
	// command and it is the option.
	//
	// A function for the reason KeyBindings is one: both spellings are
	// commands a person runs at the prompt.
	ViEditing func() bool

	// RunWidget runs one of the shell's own editing actions — a key bound to
	// something the shell was told about at run time rather than to one of
	// this editor's Widgets. It is given the line as the editor holds it and
	// answers with the line as the action left it; false is an action this
	// shell will not run, and leaves the line alone.
	//
	// Nil is a session where a key can only be bound to what this editor
	// does, which is what a front end that has not said gets. See
	// shellwidget.go, which carries the split and the two seams deliberately
	// not offered beside it.
	RunWidget func(ctx context.Context, name string, in Line) (Line, bool)

	// RunScheduled runs whatever the shell had set aside for a time that has
	// now passed. It is called once at every prompt, before the prompt is
	// drawn, in the terminal's own line discipline — the same place and the
	// same discipline a hook fires in, and for the same reasons. Nil is a
	// session with nothing that can be scheduled.
	//
	// **The moment is what this package contributes, and it is the whole of
	// it.** A shell that can put a command aside until a time needs somewhere
	// to notice that the time has come, and between one command and the next
	// is the only place that has one: a script has no such boundary and a
	// prompt is made of them. What a scheduled entry is, how its time was
	// spelled and what running it means are the dialect's, which is why this
	// hands back nothing and is told nothing.
	//
	// **Not while waiting for a key.** The shell being modeled fires an
	// elapsed entry from the *idle* read, so a command set aside for two
	// seconds runs two seconds later whether or not anybody types; here it
	// runs at the next prompt, which for an idle terminal is later — and for
	// a person who is typing, sooner is impossible either way.
	//
	// The idle read now exists — see WatchedDescriptors — and this still does
	// not use it, which is a decision and not an oversight. The editor waits
	// on more than the terminal only while a descriptor is actually armed, so
	// hanging a timer off that wait would fire a scheduled command on time in
	// a session that happened to have a watcher and late in one that did not.
	// Late in both is the answer a person can predict.
	RunScheduled func(ctx context.Context)

	// WatchedDescriptors is which descriptors, beside the terminal, this
	// editor should wait on while it waits for a key. Nil — and an empty list
	// — is a session that waits on the terminal alone, which is what a front
	// end without a way to arm one gets, and which is read exactly the way a
	// key was read before this seam existed.
	//
	// A function rather than a value for the reason KeyBindings is one: the
	// list is state that changes while the session runs, and a handler that
	// arms the next descriptor before it returns is the ordinary case.
	WatchedDescriptors func() []int

	// DescriptorReady is called when one of WatchedDescriptors has become
	// readable. It is given the line as the editor holds it and answers with
	// the line as it was left; false leaves the line alone and draws nothing.
	//
	// The same round trip RunWidget is, keyed on a descriptor rather than on a
	// key, and false means something different here: not "the shell declined"
	// but "the line is untouched and whatever was printed is the shell's
	// business". Measured, that is the common case — a descriptor callback in
	// the shell being modeled prints where the cursor was and does not redraw
	// — and the spelling that does want the line back is the exception. See
	// watchfd.go for the four measurements that shape this, and dialect/zsh
	// for what a shell calls the command that arms one.
	DescriptorReady func(ctx context.Context, fd int, in Line) (Line, bool)

	// History is how this dialect draws a search of the session's history and
	// what it declines to put in it. The zero value searches in the
	// substrate's own wording and filters nothing.
	History HistoryStyle

	// Hooks are the functions this dialect runs between commands — one
	// before every prompt and one before every line runs. The zero value is
	// a dialect with none, which is three of the four. See hooks.go, which
	// carries the measurement of when each fires, what it is told and what
	// it may not change.
	Hooks HookStyle

	// StartLine is called once before each new line is read, and not before a
	// continuation of one already begun.
	//
	// It exists for state a dialect keeps for the length of a line and must
	// not carry into the next — zsh's `region_highlight`, whose effect the
	// manual says "disappears as soon as the line is accepted". Nil is a
	// dialect with no such state, which is three of the four.
	//
	// Deliberately told nothing and answering nothing. What a line is made of
	// is this package's, and what a dialect keeps about one is the dialect's;
	// the only thing crossing here is the *moment*, which is the same reason
	// RunScheduled is shaped the way it is. Overloading that one instead was
	// the alternative and it would have tied "a command was put aside until a
	// time" to "a line began", which are unrelated facts that happen to share
	// an instant.
	StartLine func()

	// Highlighter colors the line as it is typed. Nil draws it plainly, which
	// is what every shell in the panel does and what a front end that has not
	// said gets.
	//
	// **In-process only, and never a plugin.** It is asked on every keystroke,
	// which is the hot-path exclusion docs/design/plugins.md already writes
	// down, one step hotter than completion. See highlight.go, which carries
	// the measurement that decided the seam's shape and the reason this one
	// must not be generalized.
	//
	// Not a dialect's, and this one is not even a close call: measured, none
	// of the four colors a line as it is typed, so there is no disagreement
	// for a dialect field to record. UnclosedQuote is the highlighter this
	// package ships and the front end chooses its color.
	Highlighter Highlighter

	// PromptProviders contribute text to every prompt this session draws, in
	// order, before the prompt parameter's own text. Nil is a session whose
	// prompt is the prompt parameter and nothing else, which is what a front
	// end that has not said gets.
	//
	// Before rather than after, and that is the whole of the placement rule.
	// The prompt parameter is the person's own text and the cursor sits
	// immediately after it, so anything inserted between the two would move
	// the thing they are typing away from the thing they set. A provider that
	// wants a space before the prompt writes one.
	//
	// Not part of a dialect, for the reason Completers is not: a provider is
	// a code path and a dialect is a table of values. What a dialect says
	// about a prompt stays in PromptStyle, which is a table of what its codes
	// mean.
	//
	// Drawn once per command rather than once per keystroke, which is what
	// makes this a plausible plugin role later where completion is not. See
	// promptprovider.go for the cost and failure rules.
	PromptProviders []PromptProvider

	// Completers are what answers Tab before this shell's own completion
	// does, in order. Nil is a session that completes the way the substrate
	// does, which is what a front end that has not said gets.
	//
	// Consulted in order, and **the first one with anything to say is the
	// whole answer** — the lists are not merged, and this shell's own
	// completer is last. See completers, where the rule is measured against
	// both shells that have a completion system.
	//
	// Not part of a dialect, and that is the substrate rule rather than a
	// preference. A dialect is a table of values, re-read as execution
	// proceeds; a completer is a code path, and a dialect that carried one
	// would be a dialect that could not be written as data. What a dialect
	// *does* say about completion stays where it is and stays a value —
	// EditorStyle.CompletionMatchesHiddenFiles is the shape that question has
	// to take. This field is the front end's, for the same reason `-plugin`
	// is cmd/sh's: it is a program's composition and not a shell's grammar.
	//
	// Called on the keystroke path. Read the note at the top of completer.go
	// before writing one that does I/O.
	Completers []Completer

	// HistoryRecorders are told every line this session records, besides the
	// history file, which is always told too. Nil is a session whose history
	// is the file and nothing else.
	//
	// An addition rather than a replacement, and every one of them is told —
	// a recorder is not answering a question, so there is no winner to pick.
	// See historyseam.go, which carries the reasoning and the rules a line
	// has already passed by the time a recorder sees it.
	HistoryRecorders []HistoryRecorder

	// HistorySources supply lines this session can recall, before the history
	// file's. Nil is a session that recalls the file and nothing else.
	//
	// Asked once, in the order given, before the first prompt; their lines
	// are the older half of the walk. See historyseam.go.
	HistorySources []HistorySource

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

	// hooks is what this session has already said about a hook it will not
	// fire, so it says it once. Set by Run, beside counts and for the same
	// reason: a Shell is copied by value and a notice given by a copy has to
	// count for the session.
	hooks *hookState

	// capture is where the Runner's output goes when this session keeps
	// blocks — the same value the loops are handed, held here because one
	// caller is not in the block path at all. A hook prints through the
	// Runner's streams, which is the far end of a conduit a goroutine
	// forwards, so between a hook and the prompt drawn after it there is
	// something to wait for. See settled.
	capture *outputCapture

	// Name is what the shell calls itself, for a prompt that draws it —
	// bash's `\s`. Empty draws nothing, which is what a caller that has not
	// said gets.
	Name string
}

// Run reads, evaluates and prints until the input ends.
//
// inFile is the session's input as a descriptor, or nil where it is not one.
//
// Every question this package asks about the terminal goes through here, and
// each of the three already has an answer for nil: IsTerminal says no,
// terminalWidth says it does not know, and lookupTerminal has no name for it.
// So a reader that is not a file takes runPlain, which is the same route a
// pipe takes and the one this package has always had for it.
func (s Shell) inFile() *os.File {
	f, _ := s.In.(*os.File)
	return f
}

// It returns the status of the last command, which is what the shell exits
// with — the same thing a script's last command decides.
func (s Shell) Run(ctx context.Context) (int, error) {
	// What earlier sessions typed. Loaded before the two loops part company,
	// because both of them number their prompts from where it left off:
	// measured, bash given `-i` on a pipe with three lines in HISTFILE draws
	// `!4 #1` at its first prompt, editor or no editor.
	hist := s.historyFile()
	earlier := s.recalled(ctx, hist)
	s.counts = &counts{history: len(earlier)}
	s.hooks = &hookState{reported: map[string]bool{}}
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
	s.capture = capture
	// Closed on the way out, so that whatever is still in the conduit reaches
	// the terminal before the process does anything else with it.
	defer capture.close()
	if !IsTerminal(s.inFile()) {
		// A prompt without a terminal is not a mistake to refuse: every shell
		// in the panel, given `-i` on a pipe, still prints a prompt and runs
		// the lines — it only says that job control is off. The *editor* is
		// what needs a terminal, and it is the editor that goes away.
		return s.runPlain(ctx, store, capture)
	}
	state, err := makeRaw(s.inFile())
	if err != nil {
		return 0, err
	}
	// Deferred rather than restored at each return: a panic here would
	// otherwise leave the terminal with echo off, which is a broken terminal
	// and not merely a crash.
	defer func() { _ = state.restore() }()
	// Raw mode took the kernel's newline translation with it, so this loop
	// does it — see crlf. On the copy of the Shell this loop runs on, which is
	// what keeps it off the piped loop above, off the Runner's own streams
	// (those are written with the terminal in its own discipline, where the
	// kernel is still translating) and off the shell the caller handed in.
	s.Out, s.Err = translating(s.Out), translating(s.Err)

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
	// And the interpreter asks the same handler what it heard, because a ^C
	// that arrives while the shell is running a loop of its own arrives *here*
	// — the shell holds the terminal, so the signal is this process's — and
	// interp installs no handlers of its own. Cleared on the way out: the
	// Runner is the caller's, and a hook left pointing at this session's
	// handler would outlive the handler.
	s.Runner.TakeInterrupt = sig.take
	defer func() { s.Runner.TakeInterrupt = nil }()

	ed := s.newEditor(ctx, state)
	ed.history = earlier
	// What this session will write, which is not the same list as what it can
	// recall. The two used to be one — the new tail of the editor's history —
	// and they cannot be, because a dialect exists in which an ignored line is
	// still recallable and still not written. Kept here, so that the only
	// thing the file's contents depend on is what went into this slice.
	var added []string
	defer func() {
		if err := hist.save(ctx, added); err != nil {
			s.errf("%v\n", err)
		}
	}()
	record := s.recording(ed, &added)
	var pending strings.Builder
	for {
		if pending.Len() == 0 {
			// The last thing done about the previous command's output, and
			// done before the prompt hooks rather than after them — measured;
			// see markUnfinished. A continuation prompt marks nothing: the
			// newline the terminal echoed already ended the row.
			ed.markUnfinished()
		}
		drawn := s.beforeReading(ctx, state, &pending)
		if s.Runner.Exited() {
			// A prompt hook called `exit`. Measured, zsh's session ends
			// there and draws no prompt, so this one does not read a line.
			return s.status(), nil
		}
		line, err := ed.readLine(drawn)
		switch {
		case errors.Is(err, ErrInterrupted):
			// ^C abandons whatever was half-typed, including the earlier
			// lines of an unfinished construct — which is the whole point of
			// it at a continuation prompt.
			pending.Reset()
			continue
		case errors.Is(err, io.EOF):
			if s.heldForJobsAtExit(state) {
				// The end of input is a request to leave, and a shell with a
				// stopped job answers it the way it answers `exit`: it says so
				// and stays. The session goes on rather than returning, so a
				// second ^D is what actually ends it.
				continue
			}
			stmts, text, perr := s.endOfInput(&pending)
			switch {
			case perr != nil:
				s.errf("%s", s.report(perr))
				s.refused(perr)
			case len(stmts) > 0:
				b := s.beginBlock(text)
				s.run(ctx, state, text, stmts)
				s.closeBlock(ctx, store, capture, b)
			}
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
		stmts, text, perr, ready := s.take(&pending, record, line)
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
			//
			// The status is the refusal's, though, and that is the half this
			// loop used to leave out — see refused.
			s.errf("%s", s.report(perr))
			s.refused(perr)
			continue
		}
		b := s.beginBlock(text)
		done := s.run(ctx, state, text, stmts)
		s.closeBlock(ctx, store, capture, b)
		if s.interrupted(sig) {
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
func (s Shell) run(ctx context.Context, state *terminalState, typed string, stmts []*syntax.File) bool {
	var done bool
	s.inLineDiscipline(state, func() { done = s.runStmts(ctx, typed, stmts) })
	return done
}

// inLineDiscipline runs f with the terminal in its own line discipline, and
// puts raw mode back afterwards.
//
// Three things need it and they need exactly the same thing: a command, the
// interpreter's warning about stopped jobs, and a hook — each is the shell
// speaking to the person or handing the terminal to something that will,
// rather than the editor drawing a line. Raw mode has OPOST off, so a newline
// written under it is a line feed and nothing else, and the next thing drawn
// starts wherever the last one ended.
//
// One helper rather than three copies, because the copies were the bug: the
// second one existed before this and the third would have been written without
// the restore, which is precisely the failure it prevents.
//
// A nil state is a session with no terminal to hand back — runPlain's — and
// there f simply runs.
func (s Shell) inLineDiscipline(state *terminalState, f func()) {
	if state == nil {
		f()
		return
	}
	if err := state.restore(); err != nil {
		s.errf("%v\n", err)
	}
	defer func() {
		// **Nothing may still be on its way to the terminal when OPOST goes
		// off again.** A command's output does not always go straight to the
		// terminal: under a block store it goes through a pseudo-terminal
		// whose own output discipline is off, and a goroutine copies that to
		// the real one. So the command returning is not the same event as its
		// last bytes arriving, and re-entering raw mode between the two sends
		// the tail out with the newline translation already gone — a bare
		// line feed, and the next prompt drawn wherever the output ended.
		//
		// That is the failure this helper exists to prevent, arriving by the
		// one path the restore alone did not cover. The wait is here rather
		// than in a fourth copy of the helper because the other two callers
		// already do it inside their own f — a hook's output is waited for
		// by settled, before the terminal is handed back — and the command's
		// own output was the one nobody waited for. It was waited for, in
		// closeBlock, a few instructions after this defer had already put
		// raw mode back.
		//
		// It waits on the conduit's in-band mark rather than for quiet, so
		// it is proof and not a pause, and it keeps what was captured: the
		// block that records the output is read after this, and draining by
		// emptying would lose every command's body from the store.
		s.drained()
		if _, err := makeRaw(s.inFile()); err != nil {
			s.errf("%v\n", err)
		}
	}()
	f()
}

// interrupted reports whether the line that just ran was ended by a ^C, which
// is what the next prompt has to start on a line of its own for.
//
// Two questions rather than one, and the second is the load-bearing half. The
// shell only *hears* the signal while it still holds the terminal — a builtin,
// a loop of its own. An external command runs in a process group the ^C goes
// to instead, and this shell is not in it, so there the interrupt is visible
// only in what the command died of. Asking the first question alone put the
// prompt on top of the `^C` for every real command from the moment job control
// started handing the terminal over.
func (s Shell) interrupted(sig *interrupts) bool {
	return sig.took() || s.Runner.LastCommandWasInterrupted()
}

// heldForJobsAtExit asks the interpreter whether the end of input should end
// the session, with the terminal in its own line discipline while it answers.
//
// The discipline is the whole reason this is not the call written inline. The
// warning is the interpreter's to word and to write, and it writes it to the
// session's error stream — which here is a terminal in raw mode, where OPOST
// is off and a newline is a line feed and nothing else. The next prompt then
// starts wherever the message ended, twenty-three columns in. Restoring for
// the length of it is the same thing running a command does, and for the same
// reason: this is the shell speaking to the person rather than drawing a line.
func (s Shell) heldForJobsAtExit(state *terminalState) bool {
	var held bool
	s.inLineDiscipline(state, func() { held = s.Runner.HoldsExitForJobs() })
	return held
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
func (s Shell) runStmts(ctx context.Context, typed string, stmts []*syntax.File) (done bool) {
	// The command hook fires here: after the line was read and before any of
	// it runs, which is what it is for, and inside run's restore so that what
	// it prints reaches a terminal in its own line discipline. Nothing fires
	// for a line with nothing in it — measured, an empty line draws a prompt
	// and runs no `preexec` — and nothing fires for a line the parser
	// refused, which never reaches here at all.
	if len(stmts) > 0 {
		s.fireBeforeCommand(ctx, typed, stmts)
		s.settled()
		if s.Runner.Exited() {
			return true
		}
	}
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
//
// **A fatal error in what was typed costs the line and not the session.** A
// prompt is a boundary in the same sense a file is: the text a person just
// gave the shell is one unit of input, and an error in it ends *that* and asks
// for the next line. Measured through a pseudo-terminal, one keystroke at a
// time, with `set -u` and `echo X${NOPE}`: bash 5.3, zsh 5.9.2, ksh93u+ **and
// dash** all print the diagnostic and draw the next prompt, and so does every
// other fatal expansion asked — `${x?word}`, `${x:?word}`, a division by
// zero, a substitution that will not read, a readonly reassignment where the
// dialect calls one fatal. This shell ended the session for all of them, in
// all four dialects, so one mistyped variable name under `set -u` closed the
// terminal (#1124).
//
// The **third site** of the boundary #1104 built, and the same mechanism:
// `.` and `eval` are the first — see Semantics.FatalErrorEndsBorrowedTextOnly
// — and a startup file the second, in driver.startup. Not a new one, which is
// what made this a call rather than a design: interp.Runner.GiveUpTheLine.
//
// The axis it answers differently is `${x?word}`. Two dialects document that
// operand as ending the shell and do end it in a script; at a prompt all four
// draw the next prompt, so this site asks nothing where the other two ask
// (#1122). Whether the *borrowed text* boundary caught it first is beside the
// point here: an error that reached this line ends the line whichever
// dialect it is.
//
// It deliberately catches only an *error*. `exit` is not one: `exit`, `exit 7`
// and `eval 'exit 7'` all end the session in every shell measured, and so does
// errexit firing — `set -e` then `false` is a session gone in all four —
// which is exactly what these calls do not catch.
func (s Shell) runEach(ctx context.Context, stmts []*syntax.File) bool {
	for _, st := range stmts {
		if err := s.Runner.RunPart(ctx, st); err != nil {
			// Refused rather than silently skipped, the same way the script
			// driver does it.
			s.errf("%v\n", err)
			return false
		}
		if s.Runner.GiveUpTheLine() {
			// An error the line gave up over. The status it left behind is
			// the line's status, exactly as a command's own failure would
			// be, and the statements after it in the same line are not run:
			// what a person typed is the unit, and the unit is over.
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
//
// A drawnPrompt rather than the text, and here rather than in the editor,
// because both loops write it: the markers a dialect's non-printing codes
// leave behind have to come out whether or not there is a terminal to draw on,
// and a second place that turned a rendered prompt into bytes would be a
// second place to forget.
//
// PS2 is read exactly as PS1 is, which is measured rather than assumed: with
// PS2 set to a prompt of codes, bash 5.3.15 and 3.2.57 drew the user, the
// directory, the privilege character and a bracketed color at the
// continuation prompt, zsh 5.9.2 drew the same from its own language, and both
// re-ran a command substitution in it.
func (s Shell) beforeReading(ctx context.Context, state *terminalState, pending *strings.Builder) drawnPrompt {
	continuing := pending.Len() > 0
	// Before anything else, and not on a continuation: what this clears is
	// kept for the length of a line, and a construct still asking for more
	// text is the same line.
	if !continuing && s.StartLine != nil {
		s.StartLine()
	}
	// Before anything is drawn, because the prompt itself may be written
	// against $COLUMNS — a right-aligned segment is the usual reason a
	// startup file asks for this at all — and a prompt drawn from last
	// window's width is the one visible symptom the option exists to stop.
	s.trackWindowSize()
	s.reportFinishedJobs(continuing)
	// After the notices and before the prompt is expanded, which is the
	// order measured — see hooks.go. In the terminal's own line discipline,
	// for the reason heldForJobsAtExit is: a hook is a shell function and
	// its output goes to the Runner's streams, which nothing here translates,
	// so a newline written in raw mode would leave the next line where the
	// last one ended.
	s.inLineDiscipline(state, func() {
		s.reportUnfiredHooks()
		s.runElapsed(ctx)
		s.fireBeforePrompt(ctx, continuing)
		s.settled()
	})
	// The contributions and the parameter are rendered and then measured
	// together, in one drawPrompt, because the markers that say "none of this
	// is a column" have to be taken out of both and the count has to cover
	// both. Two drawnPrompts added together would be a text of two halves and
	// a width of one.
	if continuing {
		return drawPrompt(s.contributed(true) + s.prompt("PS2", or(s.Style.DefaultContinued, "> ")))
	}
	return drawPrompt(s.contributed(false) + s.prompt("PS1", or(s.Style.Default, "$ ")))
}

// trackWindowSize puts the terminal's size in $LINES and $COLUMNS, where the
// shell has said it wants that.
//
// bash's `checkwinsize`, and only the naming of it is bash's: the shell holds
// the permission (interp.Runner.TracksWindowSize) and this holds the ioctl,
// because a Runner has no terminal and a script has no window. zsh reaches the
// same behavior with no option name at all, which is the other reason the
// switch is not in either dialect.
//
// Once per prompt, which is what the option promises — the size is checked
// after each command — and it is also what makes the *first* prompt right:
// bash 5.3.15 driven through a pseudo-terminal reports `COLUMNS=80 LINES=24`
// before anything has been typed, so a session that only updated on a change
// would start with both unset and a startup file reading `$COLUMNS` would read
// nothing.
//
// Nothing is written for a size the terminal will not give: a pipe, a closed
// terminal or a kernel that declines all answer zero, and assigning `0` would
// be worse than assigning nothing — a prompt that divides by it, or wraps at
// it, is broken in a way an unset variable is not. Measured the same way:
// bash with `-i` on a pipe leaves both unset.
//
// Set rather than exported. bash does not export either name — `export -p`
// shows no COLUMNS in a session where `$COLUMNS` is 80 — so a child gets the
// terminal's size from the terminal, exactly as this shell does.
func (s Shell) trackWindowSize() {
	if s.Runner == nil || !s.Runner.TracksWindowSize() {
		return
	}
	rows, cols := terminalSize(s.inFile())
	if cols > 0 {
		s.Runner.SetVar("COLUMNS", strconv.Itoa(cols))
	}
	if rows > 0 {
		s.Runner.SetVar("LINES", strconv.Itoa(rows))
	}
}

// contributed is what this session's prompt providers add, in order.
//
// Each behind the panic guard a typed line already runs behind, and separately
// rather than all of them together, so that one provider with a bug costs its
// own segment and not everybody else's. The reason to guard at all is the
// reason the line is guarded: the process is the session, and a prompt that
// took a shell down with it would end a session that has been open for hours
// over a segment somebody wanted for decoration.
//
// A guarded panic leaves the segment empty and the diagnostic goes where every
// other one does, so the person sees which shell complained and the prompt
// still arrives.
func (s Shell) contributed(continuing bool) string {
	if len(s.PromptProviders) == 0 {
		return ""
	}
	info := s.promptInfo(continuing)
	guard := s.guard()
	var b strings.Builder
	for _, p := range s.PromptProviders {
		if p == nil {
			continue
		}
		var text string
		if guard.Do(func() { text = p.Prompt(info) }) {
			// It panicked. The report has already been written; the segment
			// is whatever it managed before it did, which is nothing.
			continue
		}
		b.WriteString(text)
	}
	return b.String()
}

// promptInfo is what every provider on one prompt is told.
//
// Built once and shared, rather than per provider, so that two providers on
// the same prompt cannot disagree about what the last command was — which they
// could, since a job finishing between them changes the count.
func (s Shell) promptInfo(continuing bool) PromptInfo {
	last := s.counted().last
	return PromptInfo{
		Continued: continuing,
		Dir:       s.workingDir(),
		Command:   last.command,
		Status:    s.exitStatus(),
		Duration:  last.duration,
		Jobs:      s.liveJobs(),
	}
}

// exitStatus is `$?`, and zero for a shell with no Runner.
func (s Shell) exitStatus() int {
	if s.Runner == nil {
		return 0
	}
	return s.Runner.ExitStatus()
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
func (s Shell) runPlain(ctx context.Context, store *blocks.Store, capture *outputCapture) (int, error) {
	in := bufio.NewReader(s.In)
	var pending strings.Builder
	for {
		drawn := s.beforeReading(ctx, nil, &pending)
		if s.Runner.Exited() {
			// A prompt hook called `exit`, exactly as in the other loop:
			// measured, zsh's session ends there and draws no prompt, so
			// the prompt this had ready is not written either.
			return s.status(), nil
		}
		s.errf("%s", drawn.text)

		line, err := in.ReadString('\n')
		if line == "" && err != nil {
			// End of input ends the session, exactly as ^D does at a
			// terminal. A final line without a newline is still a line,
			// which is why this asks about the text and not only the error.
			if s.Runner.HoldsExitForJobs() {
				// And it is held back for a job that would be abandoned
				// exactly as ^D is —
				// the same call in both loops, so they cannot disagree about
				// it. Once: the hold records that it said so, so the next
				// read, which ends at once, leaves.
				continue
			}
			stmts, text, perr := s.endOfInput(&pending)
			switch {
			case perr != nil:
				s.errf("%s", s.report(perr))
				s.refused(perr)
			case len(stmts) > 0:
				b := s.beginBlock(text)
				s.runStmts(ctx, text, stmts)
				s.closeBlock(ctx, store, capture, b)
			}
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
			s.refused(perr)
			continue
		}
		b := s.beginBlock(text)
		done := s.runStmts(ctx, text, stmts)
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
	first := s.countLine(pending.Len() == 0)
	pending.WriteString(line)
	pending.WriteString("\n")
	text := pending.String()

	p := syntax.NewParserAt(text, s.Dialect, first)
	// The hook goes on unconditionally, unlike the script path: every shell
	// in the panel expands aliases at a prompt, and the dialect's answer is
	// only about a *non-interactive* one. This is the place that knows there
	// is a person at the keyboard, and the front end has already told the
	// runner so. Through ExpandingAlias rather than the table directly, so
	// that `shopt -u expand_aliases` typed at the prompt is honored.
	if s.Runner != nil {
		p.Aliases = s.Runner.ExpandingAlias
		p.GlobalAliases = s.Runner.ExpandingGlobalAlias
		p.SuffixAliases = s.Runner.ExpandingSuffixAlias
	}
	stmts, err := collect(p)
	// Incomplete rather than incomplete-and-failed: input can be unfinished
	// without being wrong, and a here-document with no delimiter yet is
	// exactly that — the parser takes what it has and says there may be
	// more, which is an error to a script and a question to a prompt.
	//
	// And unfinished is not the only thing input can be. A construct the
	// parser has *refused* is asked about first: it is unfinished as well, and
	// no line that follows it can mend it, so a prompt that asked for one
	// would be asking for something it is going to throw away.
	if !s.refusedForGood(err) && (p.Incomplete() || endsWithContinuation(text)) {
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

// countLine records that one more line has been read and answers the line
// number the text now accumulating begins at.
//
// The count runs whatever the dialect says, so that turning the numbering on
// is one branch here rather than a second accounting nothing keeps in step;
// what the dialect decides is only whether the parser is told about it. See
// Shell.CountSessionLines.
func (s Shell) countLine(startsPending bool) int {
	c := s.counted()
	if startsPending {
		c.pendingLine = c.line + 1
	}
	c.line++
	return s.pendingLine()
}

// pendingLine is the session line the text now accumulating began on, or 1
// where the dialect numbers every construct from its own first line.
func (s Shell) pendingLine() int {
	if !s.CountSessionLines {
		return 1
	}
	if n := s.counted().pendingLine; n > 0 {
		return n
	}
	return 1
}

// endOfInput is what becomes of a half-typed construct when there is no more
// input coming.
//
// At a prompt an unfinished construct asks for another line; at end of input
// there is no other line, so the text is read as the whole of it — the same
// reading a script file's last line gets. It is then either a command or a
// failure, and both halves are measured and unanimous across the panel:
//
//	echo one \        every shell runs it and prints `one`
//	cat <<EOT / body  every shell runs it and prints `body`
//	x='never closed   every shell says what it was waiting for
//	if true; then     every shell says what it was waiting for
//
// We did neither. The pending text was dropped without a word, so a person who
// typed a quote by accident and pressed ^D was told nothing at all about why
// their line had vanished (#1467).
//
// What happens *next* is where the panel divides — bash ends the session and
// the other three prompt again — and that is deliberately not answered here:
// this loop was already leaving at end of input, so it keeps doing so.
func (s Shell) endOfInput(pending *strings.Builder) ([]*syntax.File, string, error) {
	text := pending.String()
	pending.Reset()
	if strings.TrimSpace(text) == "" {
		return nil, "", nil
	}
	p := syntax.NewParserAt(text, s.Dialect, s.pendingLine())
	// The same alias table the accepted lines were parsed with: a construct
	// half of which was typed through an alias must not finish differently
	// for having been finished here.
	if s.Runner != nil {
		p.Aliases = s.Runner.ExpandingAlias
		p.GlobalAliases = s.Runner.ExpandingGlobalAlias
		p.SuffixAliases = s.Runner.ExpandingSuffixAlias
	}
	stmts, err := collect(p)
	// Before the failure and whether or not there is one, which is the order
	// the script route already writes them in and is measured: a here
	// document with neither its delimiter nor its enclosing `}` produces both,
	// the warning first.
	s.sayRemarks(p.Remarks())
	if err != nil {
		return nil, "", err
	}
	return stmts, strings.TrimSuffix(text, "\n"), nil
}

// sayRemarks writes what the parser had to say about input it accepted anyway.
//
// Only from the end of the input, which is the only place a prompt can reach
// one: the single remark the panel has is a here-document delimited by the end
// of the input, and while a session is still running there is always another
// line, so the parser is waiting for the delimiter rather than remarking on
// its absence. A call beside accept would be a line no test could tell from a
// line that was never there.
func (s Shell) sayRemarks(rs []syntax.Remark) {
	if s.Remark == nil {
		return
	}
	for _, rk := range rs {
		if line := s.Remark(rk); line != "" {
			s.errf("%s", line)
		}
	}
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
		if line.Refused != nil {
			// A construct the reader gave up on, which ends the line and not
			// the shell — see syntax.File.Refused. A prompt has no next line
			// of its own to go on to: the person types one. So the refusal
			// is handed back as the failure of this input, which is what the
			// caller reports and what keeps a line nobody can run from
			// running silently.
			return nil, line.Refused
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
	// Three passes: the dialect's table of codes, the expansion, and the
	// character that stands for the history number. The first two are
	// interp.RenderPromptValue, which runs them in the order the *dialect*
	// draws them — see PromptStyle.ExpandBeforeEscapes, which is measured in
	// both directions and is what decides whether an escape a parameter
	// produced is one the table ever sees. The same call answers `PS4` in
	// front of a trace line and bash's `${v@P}`, so the three readers of a
	// prompt value cannot drift apart.
	//
	// The refusal this drawer never sees is dropped rather than reported: its
	// resolvers answer every code, so ok is always true here.
	value, _, _ = interp.RenderPromptValue(s.Style, s.Runner, value, s.promptField, s.promptQuantity)
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
	h.bound = boundary.Boundary{Gate: s.Gate, Events: s.Events, Session: s.Session}
	// How this shell's file spells an entry, which is the dialect's answer —
	// see HistoryStyle, and decodeEntries for what is done with it.
	h.encoding = historyEncoding{
		continuesOnABackslash: s.History.EntriesContinueOnABackslash,
		mayCarryATimestamp:    s.History.EntriesMayCarryATimestampHeader,
	}
	return h
}

// completer answers Tab from this shell: whatever the caller contributed, and
// then this shell's own builtins, functions, PATH and files.
//
// The shell's own goes last, which is the whole of what "extending completion"
// means here — a caller that answers takes the word, and one that does not
// leaves the substrate's answer standing. Nothing is merged; see completers.
//
// Built once per session rather than per keystroke for the names, which do not
// move; PATH and the directory are read each time because `cd` and an
// assignment both change them under it.
func (s Shell) completer(ctx context.Context) Completer {
	var all completers
	all = append(all, s.Completers...)
	if s.Runner != nil {
		all = append(all, runnerCompleter{
			r: s.Runner, hidden: s.Editor.CompletionMatchesHiddenFiles,
			// The session's boundary and the session's context, so a
			// directory listed to answer Tab is asked about the same way one
			// listed by a glob is. The history file above is handed the same
			// pair for the same reason.
			bound: boundary.Boundary{Gate: s.Gate, Events: s.Events, Session: s.Session},
			ctx:   ctx,
		})
	}
	if len(all) == 0 {
		// A nil interface rather than an empty list, because the editor's
		// check is against nil and a typed nil would pass it.
		return nil
	}
	return all
}

// runnerCompleter reads the shell's state at the moment Tab is pressed.
type runnerCompleter struct {
	r *interp.Runner
	// hidden is the dialect's answer and not the shell's, so it is settled
	// once when the session starts rather than read again per keystroke.
	hidden bool

	// bound and ctx are the session's gate, sink and context. Held here
	// because the Completer seam carries neither and cannot be widened to;
	// shellCompleter.ctx has the argument.
	bound boundary.Boundary
	ctx   context.Context
}

func (c runnerCompleter) names() []string {
	names := append(c.r.BuiltinNames(), c.r.FuncNames()...)
	// The reserved words are commands too — `if` and `while` are what a line
	// most often starts with, and a completer that offered every builtin but
	// not those would feel broken.
	return append(names, reservedWords...)
}

func (c runnerCompleter) shell() shellCompleter {
	path, _ := c.r.GetVar("PATH")
	// HOME from the shell's own variables rather than from the process's
	// environment, for the same reason the directory is the Runner's: a
	// Runner holds both, and a line that says `HOME=/tmp` means it.
	home, _ := c.r.GetVar("HOME")
	return shellCompleter{
		names: c.names(), path: path, dir: c.r.Dir,
		home: home, hidden: c.hidden,
		// Asked per keystroke rather than settled with hidden, because this
		// one moves during a session: `shopt -s no_empty_cmd_completion` is a
		// line a person types, and a completer built once at startup would
		// take effect on the next shell.
		emptyWordOffersNothing: !c.r.CompletesEmptyCommandWord(),
		// The pair behind `dirspell` and `direxpand`, read here for the same
		// reason: both are lines a person types at the prompt. The corrector
		// is handed over as a method value rather than as a bool because it
		// reads its own option — see interp.Runner.CorrectedDirectory — so
		// the switch is consulted at the keystroke and in one place.
		correctDir:       c.r.CorrectedDirectory,
		expandsDirectory: c.r.ExpandsCompletedDirectory(),
		// Handed over as a method value for the same reason the corrector is:
		// what a parameter expands to is the shell's own state, and it moves
		// between keystrokes.
		expandParams: c.r.ExpandParametersOnly,
		bound:        c.bound, ctx: c.ctx,
	}
}

// Complete answers the word this shell would answer with, which is a command
// where a command belongs and a filename everywhere else.
//
// The branch is on the request rather than on the line, because where a word
// sits is the editor's rule and this is one of two callers of it. See
// Completion.Command.
func (c runnerCompleter) Complete(req Completion) []string { return c.shell().Complete(req) }

// reservedWords is the grammar's own vocabulary, which no builtin table holds.
var reservedWords = []string{
	"case", "do", "done", "elif", "else", "esac", "fi", "for",
	"function", "if", "in", "select", "then", "until", "while",
}

// workingDir is the shell's directory, as a completer is told it.
//
// Nil-safe and a method rather than a closure over the Runner, so a Shell
// without one still builds an editor: the editor asks per keystroke and a
// session with nothing to ask gets the empty answer rather than a panic.
func (s Shell) workingDir() string {
	if s.Runner == nil {
		return ""
	}
	return s.Runner.Dir
}

func (s Shell) status() int { return s.Runner.ExitStatus() }

// refused sets the status a line the parser would not read leaves behind.
//
// Both loops call it, immediately after the complaint and in place of running
// anything, because a refused line is a failure the shell has to be able to
// report and the complaint alone is not that: a status left unchanged is
// undetectable to everything downstream of it — `$?` on the next line, the
// prompt's failure indicator, and the hooks the prompt hands the status to,
// which is where this was found (#1299).
//
// Nothing else about the line moves. It is not remembered as a block and it is
// not a command, so nothing here begins one; only the number changes.
func (s Shell) refused(err error) {
	if s.ParseFailureStatus == nil {
		return
	}
	s.Runner.SetExitStatus(s.ParseFailureStatus(err))
}

// refusedForGood reports whether a parse failure is one no further line could
// mend, so that a prompt refuses it where it stands.
//
// `if; then` is the shape. The `if` has been opened and not closed, so the
// parser says the input is incomplete and says so truthfully — but the `;`
// after the keyword is a token the grammar does not want, and typing more will
// not change that. "Unfinished" and "wrong" are two answers, and a prompt is
// where the difference shows.
//
// The distinction is the error's own: this is syntax.ErrUnexpected at the `;`
// rather than syntax.ErrUnterminated at the `if`, and the parser had told them
// apart all along. What was missing is that the prompt asked only whether more
// input was possible.
//
// Measured 2026-09-11, `printf 'echo one\nif; then\necho three\n'` into each
// shell under `-i` with PS1 and PS2 set:
//
//	bash 5.3.15  refuses at once, no continuation prompt, then runs `echo three`
//	ksh93u+      the same
//	dash         the same
//	zsh 5.9.2    draws PS2 and waits — and the typed `echo three` is swallowed
//
// So three of the four refuse *before* the next line is read, and the fourth
// really does continue: interp.Semantics.PromptAsksAgainAfterARefusedToken,
// carried here as a field. False — the zero value, and what a caller without a
// dialect gets — refuses at once, which is the majority answer and the one
// that does not eat a typed command (#1893).
func (s Shell) refusedForGood(err error) bool {
	if s.AskAgainAfterARefusedToken {
		return false
	}
	var se *syntax.Error
	return errors.As(err, &se) && se.Kind == syntax.ErrUnexpected
}

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
//
// The rest of what it does is the panel's own knobs, and they part company at
// exactly one point: a line the *pattern* knob rejected is left out of the
// file by both shells, and bash also drops it from the list where zsh keeps
// it. A line rejected for a leading blank or for repeating the one before it
// is gone from the list in both, which is measured rather than assumed — see
// HistoryStyle.PatternIgnoredStaysInSession. Which answer this session gives
// is the rules' to say, and the credential rule above takes zsh's answer for
// its own reasons — so the two paths look alike here and are decided
// separately, which is why they are written separately rather than folded
// together.
func (s Shell) recording(ed *editor, added *[]string) func(string) {
	if ed == nil {
		return nil
	}
	return func(line string) {
		if strings.TrimSpace(line) == "" {
			// A bare newline at the prompt is nothing happening, which every
			// shell in the panel agrees about. Checked here as well as in
			// remember, because these are two lists now: remember drops it
			// from what can be recalled, and this drops it from what is
			// written. It was one list once, and splitting them put a blank
			// line in the file for every time somebody pressed return —
			// invisible from inside the session, because load skips blank
			// lines on the way back in, and the file grew anyway.
			return
		}
		rules := s.historyRules()
		if rule, found := secret.Default().Match(line); found {
			s.errf("%s: history: not saving this line (matched %s)\n", or(s.Name, "sh"), rule)
			ed.remember(line)
			return
		}
		if ignored, recallable := rules.ignored(line, ed.newest()); ignored {
			if recallable {
				ed.remember(line)
			}
			return
		}
		ed.remember(line)
		// Past every rule the file's contents depend on, so this is exactly
		// what the session will write — which is what makes a recorder
		// protected by the credential check above rather than obliged to
		// repeat it.
		s.recorded(sessionRecorder{added: added}, line)
	}
}

// sessionRecorder is the substrate's own: what this session will write to its
// history file.
//
// It is the history seam's first implementation and it is production code
// rather than a test's, which is the point of it being written this way. The
// file's contents are what this collects and nothing else, so a recorder added
// beside it cannot change them and cannot be forgotten by them.
type sessionRecorder struct{ added *[]string }

// Record adds the line to what the session will write.
func (r sessionRecorder) Record(e HistoryEntry) { *r.added = append(*r.added, e.Command) }

// recorded tells the file and then every recorder the front end contributed.
//
// The file first, so the thing that is always there is never behind a
// contributed recorder that misbehaves. Each behind the panic guard, and
// separately: a recorder is not answering a question, so one that fails costs
// its own record and nobody else's — the same shape a prompt provider has, for
// the same reason. Guarded at all because this runs between commands, where a
// diagnostic has somewhere to go; the seams that run while a line is being
// drawn are not, because there a complaint lands on top of the line.
func (s Shell) recorded(own HistoryRecorder, line string) {
	entry := HistoryEntry{
		Command: line,
		Dir:     s.workingDir(),
		At:      s.now(),
		Session: s.Session,
	}
	own.Record(entry)
	if len(s.HistoryRecorders) == 0 {
		return
	}
	guard := s.guard()
	for _, r := range s.HistoryRecorders {
		if r == nil {
			continue
		}
		guard.Do(func() { r.Record(entry) })
	}
}

// recalled is the list this session can walk: what the front end's sources
// supply, and then what the history file holds.
//
// The file last, so it is the recent end of the walk — the up arrow reaches
// this machine's own tail before it reaches a tool's longer memory. Each
// source behind the panic guard, for the reason a recorder is: this happens
// before the first prompt, where there is somewhere to put a complaint, and a
// session that would not start because a history tool has a bug is worse than
// a session that starts without it.
func (s Shell) recalled(ctx context.Context, hist historyFile) []string {
	if len(s.HistorySources) == 0 {
		return hist.Lines(ctx)
	}
	guard := s.guard()
	var lines []string
	for _, src := range s.HistorySources {
		if src == nil {
			continue
		}
		var got []string
		if guard.Do(func() { got = src.Lines(ctx) }) {
			continue
		}
		lines = append(lines, got...)
	}
	return append(lines, hist.Lines(ctx)...)
}

// historyRules is what this session was told to leave out, read from the
// variables this dialect keeps it in.
//
// Read per accepted line rather than once at startup, for the reason HISTFILE
// is read from the shell's own variables and not the process environment: a
// person who types `HISTCONTROL=ignorespace` at the prompt means it from the
// next line on, and a setting that only takes hold in the next session is one
// they will believe is broken. Two variable lookups and a split is nothing
// beside running the command that was typed.
func (s Shell) historyRules() historyRules {
	if s.Runner == nil {
		return historyRules{}
	}
	// Three seams into the same shell, because the settings live in three
	// places: a variable, an option, and the pattern rules a `case` uses. A
	// session that answered any of them itself would be a second shell
	// disagreeing with the first about what it was told.
	return historyRulesFrom(s.History, s.Runner.GetVar, s.Runner.DialectOption, s.Runner.MatchPattern)
}

// dialectOption answers whether a named option is on in this session, and
// false for a dialect that named none. See optionOn, which is the same
// question asked for the history rules.
func (s Shell) dialectOption(name string) bool {
	if s.Runner == nil {
		return false
	}
	return optionOn(s.Runner.DialectOption, name)
}

// markIfAsked is what to write where unfinished output stopped, and empty
// where this session is not to mark it.
//
// Two options have to be on: the marking's own, and the return it is built on
// — measured, `setopt nopromptcr` stops the mark being written though the
// marking option is still set. The return is checked beside this rather than
// here, because it also stands alone.
func (s Shell) markIfAsked() string {
	if !s.dialectOption(s.Editor.MarkUnfinishedOutputOption) {
		return ""
	}
	return s.Editor.UnfinishedOutputMark
}

// newEditor is the line editor this shell types into.
//
// A method rather than a literal in the loop so that what a dialect says
// reaches the editor is something a test can look at: the editor itself is
// only built where there is a terminal, and a dialect's answer dropped on the
// way looks exactly like a dialect that did not answer.
//
// The context is the session's and is here for one reason: completion reads
// directories, those reads go through the gate, and the seam a completer
// answers through carries no context to consult it with. See
// shellCompleter.ctx.
func (s Shell) newEditor(ctx context.Context, state *terminalState) *editor {
	return &editor{
		in: s.In, out: s.Out, comp: s.completer(ctx),
		// The shell's directory, not the process's, and asked fresh: a
		// completer is handed it in every Completion and `cd` moves it
		// between one keystroke and the next.
		workingDir: s.workingDir,
		// What colors the line while it is typed, which is nothing unless the
		// front end said otherwise.
		highlighter: s.Highlighter,
		// What this dialect marks an abandoned line with, which is `^C` in
		// two of the four and nothing in the other two.
		interrupt:       s.Editor.Interrupt,
		listQuery:       s.Editor.ListQuery,
		listQueryEchoes: s.Editor.ListQueryEchoesTheKey,
		selfInsert:      s.Editor.SelfInsertWidget,
		// What to do about output that never ended its line. Read through the
		// options rather than taken as values, because both are options a
		// person turns off — and the return is the outer of the two, so a
		// dialect whose person cleared it gets neither. See freshRow.
		unfinishedMark:  s.markIfAsked(),
		returnsFirst:    s.dialectOption(s.Editor.ReturnBeforeThePromptOption),
		clearBefore:     s.Editor.ClearBeforeThePrompt,
		listQueryStrict: s.Editor.ListQueryAcceptsOnlyYesOrNo,
		// What this dialect calls a word, and what its kills do with one.
		wordChars:                  s.Editor.WordCharacters,
		wholeLineKill:              s.Editor.KillToStartOfLineTakesTheWholeLine,
		killBeforeCursorUsesWords:  s.Editor.KillWordBeforeCursorUsesWordCharacters,
		forwardWordStopsBeforeNext: s.Editor.ForwardWordStopsBeforeTheNextWord,
		transposeAtStart:           s.Editor.TransposeAtTheStartSwapsTheFirstTwo,
		// And what a reverse search looks like, which the two shells with a
		// line editor disagree about in wording and in placement alike.
		searchPrompt: s.History.SearchPrompt,
		searchFailed: s.History.SearchFailedPrompt,
		searchBelow:  s.History.SearchBelowTheLine,
		// How much of the line one `^_` takes back, where it leaves the
		// cursor, and what `M-.` does past the oldest line it can reach.
		undoPerKeystroke:     s.Editor.UndoTakesBackOneKeystrokeAtATime,
		undoRestoresCursor:   s.Editor.UndoRestoresTheCursorToWhereItWas,
		lastArgStaysOnOldest: s.Editor.LastArgumentStaysOnTheOldestLine,
		// And what a person rebound, asked fresh for every key.
		bindings: s.KeyBindings,
		vi:       s.ViEditing,

		viInsertSkipsBlanks: s.Editor.ViInsertAtStartOfLineSkipsLeadingBlanks,
		// And how one of the shell's own actions is run, with this session's
		// context closed over.
		runFunc: s.shellWidgets(ctx),
		// What the shell wants waited on beside the terminal, how it answers
		// a descriptor that woke, and which descriptor a key arrives on. All
		// three nil-or-negative in a session with nothing armed, which is
		// what makes the wait skipped entirely — see watchfd.go.
		watch:           s.watchedDescriptors(),
		descriptorReady: s.descriptorHandlers(ctx, state),
		inFd: func() int {
			f := s.inFile()
			if f == nil {
				return -1
			}
			return int(f.Fd())
		},
		// The width comes from the input, which is the terminal; the output
		// may be a file the session was started with, and its size is not the
		// screen's.
		width: func() int { return terminalWidth(s.inFile()) },
	}
}
