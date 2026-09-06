// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"time"
)

// The public history seam: how something other than this package keeps a
// session's history.
//
// Two halves, because they are two jobs and a tool may want one of them. A
// session *records* what was typed, and it *recalls* a list — the list the up
// arrow walks and `C-r` searches. `repl/history.go` does both against a file
// of lines, and until this there was no way to do either from outside.
//
// **Both are additions and neither is a replacement, and that is the same
// answer #495 already gave.** The plain-line HISTFILE was deliberately kept
// beside the JSONL block index because the two answer different questions —
// the line file is the recall list and the index is the record of what
// happened — and either is disposable without harming the other. A seam that
// switched the file off the moment somebody attached a tool would make
// attaching a tool a decision about losing the file. A front end that
// genuinely wants only its own turns the file off the way a person does, with
// an empty HISTFILE, which is the one setting that already means exactly that.
//
// Both run **between** commands rather than while one is being typed: a
// recorder is told about a line that has been accepted, and a source is asked
// once, before the first prompt. That is what makes them guarded — see
// Shell.recorded and Shell.recalled. The rule across this package's four seams
// is one rule: a seam that runs between lines is run behind the panic guard,
// because there is somewhere for the diagnostic to go and a person to read it;
// a seam that runs while a line is being drawn is not, because a complaint
// there lands on top of what is being typed.

// HistoryEntry is one line a session recorded.
type HistoryEntry struct {
	// Command is the line as it was typed, before expansion — the same text
	// the history file keeps and a block record keeps, and for the same
	// reason: it is what the person wrote. A construct typed over four lines
	// is one entry holding all four, which is what the file holds too.
	Command string

	// Dir is the shell's working directory when the line was accepted. Here
	// because a history tool that scopes recall to a directory cannot get it
	// any other way: `cd` moves the Runner's directory and not the process's.
	Dir string

	// At is when the line was accepted, from the session's own clock.
	At time.Time

	// Session identifies the run, and is the same string the Runner carries
	// and every event and every block of this run is stamped with. Empty when
	// the front end gave the run no identity.
	//
	// It is here so that a tool's record of a command and this repository's
	// own two records of the same command can be joined. Two identities made
	// independently would leave them looking joinable and not being, which is
	// the mistake the block store already made once.
	Session string
}

// HistoryRecorder is told what a session accepted.
//
// Told the same lines the history file is told, after the same rules and at
// the same moment — which is the load-bearing half. A blank line is not one. A
// line carrying a credential is refused, and refused *before* this: the check
// is on the way in, so a recorder is protected by it rather than having to
// repeat it, and "a line the session refused to remember reached a recorder"
// is not a thing that can happen. A line the session was told to ignore —
// HISTCONTROL, HISTIGNORE — is left out of every history and not only out of
// the file.
//
// Called between commands, on the loop's goroutine, behind the panic guard. It
// is not asked to answer anything, so nothing waits on what it returns; a
// recorder that blocks still blocks the next prompt, and that is stated rather
// than defended against, for the reasons written down in completer.go.
type HistoryRecorder interface {
	Record(entry HistoryEntry)
}

// HistoryRecorderFunc adapts a function to HistoryRecorder.
type HistoryRecorderFunc func(HistoryEntry)

// Record calls f.
func (f HistoryRecorderFunc) Record(entry HistoryEntry) { f(entry) }

// HistorySource supplies lines a session can recall.
//
// Asked once, before the first prompt, for the reason the file is read once:
// the list the arrows walk is fixed for the session and grows only by what the
// session itself types. A tool whose database changed underneath would be a
// list that changed under a person's fingers mid-walk.
//
// Its lines come *before* the history file's, so they are the older half of
// the walk. That is the order they are in: a tool's database is the long
// memory and the file is this machine's recent tail, and the up arrow reaches
// the recent first.
//
// A source bounds its own answer. HISTSIZE trims the *file*, because that is
// what the variable is about; a tool that wants to hand over a hundred
// thousand lines is entitled to, and the search will walk them.
type HistorySource interface {
	Lines(ctx context.Context) []string
}

// HistorySourceFunc adapts a function to HistorySource.
type HistorySourceFunc func(context.Context) []string

// Lines calls f.
func (f HistorySourceFunc) Lines(ctx context.Context) []string { return f(ctx) }
