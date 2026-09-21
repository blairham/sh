// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `fc`'s three file letters, and the history list they move in and out of.
//
// Measured 2026-09-12 against zsh 5.9.2 with a scratch `HOME` and no startup
// files.
//
// # The interactive condition, which is the whole shape of this
//
// zsh writes a history file **only when the shell is interactive**, and that
// is not a statement about the list being empty:
//
//	zsh -f -c    'HISTFILE=z; SAVEHIST=10; fc -W'              → no file
//	zsh -f -c    'fc -R seed; fc -l; HISTFILE=z; fc -W'        → no file,
//	                                                             and `fc -l`
//	                                                             printed the
//	                                                             entry
//	zsh -f -i -c 'HISTFILE=i; SAVEHIST=10; print -s x; fc -W'  → the file
//
// The second line is what settles it: the list is there, `fc -l` proves it,
// and nothing is written. bash is the opposite — `history -w` writes from a
// plain script, which is why that row of `make sandbox` was reachable and
// this one was not until the route learned to ask for `-i` (#2283).
//
// So the condition is modeled rather than approximated, and the test for it
// is a pair: the same script under `-i` and without it, one writing and one
// not. A shell that ignored interactivity would pass every other case here.
//
// # What each letter does
//
//	fc -W [file]   the whole list, truncating. No operand means $HISTFILE.
//	fc -A [file]   the whole list, appended — so twice over a one-entry
//	               list leaves that entry twice, measured.
//	fc -R file     the file's entries, appended to the list — entries and not
//	               lines, because this dialect's file joins a continued line
//	               and may put a timestamp in front of a command. See
//	               fcLoadLines.
//
// Three more measurements decide the edges, and each is a case below:
//
//   - An **empty list** writes nothing and creates no file. A shell that
//     created an empty one would look right and would put a file where zsh
//     puts none.
//   - `SAVEHIST` of zero, or unset, writes nothing — but the *value* does
//     not trim what `-W` writes: `SAVEHIST=1` with three entries writes all
//     three. So it is a switch here and not a bound, however it reads.
//   - `HISTFILE` unset is **silent at status 0**, which is the opposite of
//     bash's `history -w` — that one names the variable and fails. Two
//     shells, two answers, and neither is the other's.
//
// # The list is the dialect's
//
// Under a name no script can reach, the way `zstyle` and `zmodload` keep
// theirs and the way `history` keeps bash's (#2271). `repl` owns the
// interactive session's list and `repl` imports `interp`, so a builtin cannot
// reach it from here; joining the two is a separate question. `print -s` is
// what fills this one, which is the same route zsh's own is filled by from a
// script.

// fcHistoryStore is the list, an indexed array oldest-first.
const fcHistoryStore = ".zsh.history"

const (
	// fcHistoryDropped is how many entries the size has taken off the front
	// of the list since the shell started. The numbers an entry is given
	// here are **absolute** and never change, so this is a running total
	// rather than a figure an assignment overwrites — see fcTrimToSize.
	fcHistoryDropped = ".zsh.history.dropped"
	// fcHistorySizeInForce is the size the list is capped at, which outlives
	// the variable: measured, `HISTSIZE=2; unset HISTSIZE` leaves the list
	// held at two, where an `unset` before any assignment leaves the default
	// of thirty. So the cap is state beside the list and not a reading of
	// the parameter.
	fcHistorySizeInForce = ".zsh.history.size"
)

// fcHistorySizeDefault is the size a list has before anything sets one.
// Measured 2026-09-21 on zsh 5.9.2, `zsh -f -c`: `typeset -p HISTSIZE` writes
// `typeset -i10 HISTSIZE=30` with nothing having referred to it, and forty
// `print -s` lines into an untouched shell leave thirty entries.
const fcHistorySizeDefault = 30

// registerFcHistory replaces the core `fc` with one that knows the file
// letters, delegating everything else back to it.
//
// Replacing rather than reimplementing: `-l`, `-n`, `-r`, `-s` and `-e` are
// the core's and stay the core's, and a dialect that copied them would be a
// second answer to maintain. [interp.Runner.Register] is documented to let a
// registration win over the built-in table for exactly this.
//
// It did copy one of them for a while. `-l` was answered here, because the
// core had no list to read; the copy knew nothing of ranges, of `-n` or of
// `-r`, so `fc -l 1 2` reached the core and the core printed nothing. The
// list is handed over instead now, and the two things that really are this
// shell's about a listing — the `%5d  %s` shape and the `-n` shape, which
// drops the whitespace bash keeps — are handed over with it. #4009.
func registerFcHistory(r *interp.Runner) {
	core, ok := r.Builtin("fc")
	if !ok {
		return
	}
	// The reader is given nothing: this list is filled by `print -s` and by
	// `fc -R`, which is what zsh's own script-level list is filled by, and a
	// front end that recorded into it would be modeling bash's answer.
	r.SetHistoryStore(fcEntries, nil)
	r.SetHistoryListingLayout("%5d  %s\n", "%s\n")
	// The numbers, which are this dialect's own answer and not the core's
	// default of one: an entry keeps the number it was given, so the oldest
	// the list still holds is numbered by everything the size has dropped.
	// See fcTrimToSize.
	r.SetHistoryNumbering(fcFirst)
	// And the parameter that bounds the list, which trims it where it
	// stands. See fcTrimToSize for the rows, and
	// interp.Runner.SetAssignmentAction for why a stored name needs a seam
	// rather than a producer's writer.
	r.SetAssignmentAction("HISTSIZE", fcSizeAssigned)
	// And the parameter itself, which is this shell's own rather than
	// something a script has to set: an integer with a default. See
	// fcStartSize, and note that the action above is registered first so
	// that what this lays down is the first size in force.
	fcStartSize(r)
	r.Register("fc", func(r *interp.Runner, ctx context.Context, args []string) int {
		if letter, rest, found := fcFileLetter(args); found {
			return fcFile(r, letter, rest)
		}
		return core(r, ctx, args)
	})
}

// fcFileLetter finds `-W`, `-A` or `-R` among the leading option words.
//
// Only those three are claimed here. Anything else — including a word that
// mixes one of them with a letter this does not know — is left to the core
// builtin, so a letter arriving later does not have to be added in two
// places.
func fcFileLetter(args []string) (letter byte, rest []string, found bool) {
	for i, word := range args {
		if !strings.HasPrefix(word, "-") || len(word) < 2 {
			break
		}
		if word == "--" {
			break
		}
		for j := 1; j < len(word); j++ {
			switch c := word[j]; c {
			case 'W', 'A', 'R':
				return c, args[i+1:], true
			case 'I':
				// zsh's modifier on the three, which narrows what is written
				// to what this session added. Not claimed, so a word
				// carrying it falls through to the core rather than being
				// silently treated as the plain letter.
				return 0, nil, false
			}
		}
	}
	return 0, nil, false
}

// fcFile is `-W`, `-A` and `-R`.
func fcFile(r *interp.Runner, letter byte, rest []string) int {
	name := ""
	if len(rest) > 0 {
		name = rest[0]
	} else if value, ok := r.GetVar("HISTFILE"); ok {
		name = value
	}
	if letter == 'R' {
		if name == "" {
			return 1
		}
		lines, ok := fcReadFile(r, name)
		if !ok {
			return 1
		}
		fcLoadLines(r, lines)
		return 0
	}
	// A write that zsh would not perform is not an error and says nothing —
	// measured at status 0 for every one of the three conditions, including
	// `HISTFILE` unset. Reporting any of them would put a diagnostic in a
	// script that zsh runs silently, and the most likely caller is an rc file
	// that saves history unconditionally.
	if name == "" || !r.Interactive || !fcSaveHistOn(r) {
		return 0
	}
	entries := fcEntries(r)
	if len(entries) == 0 {
		return 0
	}
	return fcWriteFile(r, name, entries, letter == 'A')
}

// fcSaveHistOn reports whether `SAVEHIST` permits a save at all.
//
// A switch and not a bound: `SAVEHIST=1` with three entries writes all three,
// measured, so the value says only whether anything is written. Unset and `0`
// both mean no, and a value that is not a number is treated as no for the
// same reason — there is nothing to save *to* a count nobody can read.
func fcSaveHistOn(r *interp.Runner) bool {
	value, ok := r.GetVar("SAVEHIST")
	if !ok {
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return err == nil && n != 0
}

func fcEntries(r *interp.Runner) []string {
	entries, ok := r.GetArray(fcHistoryStore)
	if !ok {
		return nil
	}
	return append([]string(nil), entries...)
}

// fcLoadLines is a file's physical lines becoming entries, which is not one
// for one in this dialect and is one for one in the substrate's default.
//
// zsh states two encodings about its own history file and `fc -R` used to
// apply neither, because it split the text on newlines and put the pieces
// straight in the list. So a `for` loop stored across backslash-continued
// lines came back as four entries, and an `EXTENDED_HISTORY` header came back
// as the front of the command (#4028).
//
// Both answers already existed: `EntriesContinueOnABackslash` and
// `EntriesMayCarryATimestampHeader` are set in HistoryStyle, measured under a
// pty in #2452, and `repl.HistoryEntries` is the decoder that reads them —
// the same call the **session's** reader makes, against the same style. That
// is the whole of the fix, and spelling the rules out here instead would have
// been the third copy of a decoder for one file: the session's, the one
// `dialect/bash` reaches through `historyLoadLines`, and this.
//
// Measured 2026-09-21, zsh 5.9.2, `env -i` with a scratch `HOME`, each file
// read by `fc -R` and listed with `fc -l 1` — and read again by the route
// `$HISTFILE` names, under a pty, which answers every shape identically:
//
//	echo one / for i in 1 2\ / do\ / echo $i\ / done / echo two
//	                                     three entries, the loop one of them
//	: 1700000000:0;echo a / : 1700000001:0;echo b
//	                                     echo a, echo b
//	echo a / <blank> / echo b            three entries, the blank kept
//	: 1700000000:0;echo a / : 1700000001:0;
//	                                     echo a, and an empty entry
//
// The last one is why the order inside the decoder matters rather than being
// an implementation detail: a header with nothing after it leaves an empty
// entry, which this dialect keeps because `EmptyLinesAreEntries` says so
// (#4024). A decoder that dropped empties before the headers came off would
// have kept that line whole instead.
//
// Every route a file reaches this list by goes through here, which today is
// `-R` alone: `print -s` is a line the script typed rather than a file, and
// this dialect deliberately has no startup read into a script's list.
func fcLoadLines(r *interp.Runner, lines []string) {
	r.SetArray(fcHistoryStore, append(fcEntries(r), repl.HistoryEntries(HistoryStyle(), lines)...))
	fcTrimToSize(r, fcHistorySize(r))
}

// fcRemember appends one line to the list. `print -s` is the caller, which is
// how a script puts something in this shell's history at all.
func fcRemember(r *interp.Runner, line string) {
	r.SetArray(fcHistoryStore, append(fcEntries(r), line))
	fcTrimToSize(r, fcHistorySize(r))
}

// The size of the list, and the trim an assignment to it does.
//
// Both were missing entirely: nothing here read HISTSIZE, so the list grew
// without bound and an assignment to it did nothing at all (#4043).
//
// Measured 2026-09-21 on zsh 5.9.2, `env -i` with a scratch HOME, over lists
// built with `print -s` and read with `fc -l`:
//
//	three adds, no assignment            1 a · 2 b · 3 c
//	three adds, then HISTSIZE=2          2 b · 3 c
//	HISTSIZE=2 first, then three adds    2 b · 3 c
//	three adds, then HISTSIZE=3          1 a · 2 b · 3 c
//	three adds, then HISTSIZE=0          3 c
//	three adds, then HISTSIZE=abc        3 c
//	three adds, then HISTSIZE=-1         3 c
//	five adds, HISTSIZE=2, then a sixth  5 e · 6 f
//	HISTSIZE=2, unset HISTSIZE, 3 adds   5 e · 6 f
//	HISTSIZE=2, =9, then a fourth add    2 b · 3 c · 4 d
//	HISTSIZE=2, then `fc -R` of 4 lines  6 z · 7 w
//
// # The numbers are the entries' own
//
// **This is where the dialect parts company with bash**, and it is the half a
// fix that took bash's rule across would get wrong. bash renumbers what a
// trim keeps from the count it dropped, so the same three entries under
// `HISTSIZE=2` list as `1 b`, `2 c` there and `2 b`, `3 c` here. An entry's
// number is settled when it joins the list and nothing moves it afterwards,
// which is one running total rather than a figure each assignment rewrites.
//
// The rule is stated here rather than as a field on repl.HistoryStyle for the
// reason the bash side gives: there is no second reader of it, and a named
// field nothing else consults is a switch without a disagreement behind it.
//
// # A value that is not a count is not "keep everything"
//
// The other half of the same measurement: `abc`, `-1` and `0` all leave
// **one** entry, where bash leaves the list alone for the first two and
// empties it for the third. The parameter is an integer with a floor rather
// than a word this shell reads three ways — `typeset -p HISTSIZE` after
// `HISTSIZE=abc` writes `typeset -i10 HISTSIZE=1`, after `HISTSIZE=" 2 "`
// writes `2`, and `HISTSIZE=1+1` is two — so the value is arithmetic and the
// floor is one. What the *parameter* reads back as is not modeled here; this
// is the list's size, and `echo $HISTSIZE` still answers what was written.

// fcSizeAssigned is the HISTSIZE assignment seam: it records the size now in
// force and trims a list already longer than it.
func fcSizeAssigned(r *interp.Runner, value string) {
	n := fcCountFloored(value)
	r.SetVar(fcHistorySizeInForce, strconv.Itoa(n))
	fcTrimToSize(r, n)
	// And the floor written back into the parameter, because zsh's HISTSIZE
	// *is* the floored value and not merely bounded by it: measured,
	// `HISTSIZE=0` then `echo $HISTSIZE` answers `1`, as do `-1` and a word.
	//
	// The arithmetic above it is the **integer attribute's** and not this
	// function's — fcStartSize declares the name, so `1+1` arrives here as
	// `2` and `2x` never arrives at all, having been the bad-math error that
	// ends the shell. One reader of one rule, and it is the core's.
	//
	// The guard is what ends the recursion rather than a flag beside the
	// store: writing the name sends this same message again, and the second
	// time the value already is the floor, so it stops. A flag would have to
	// be right about re-entry from a nested assignment as well.
	if text := strconv.Itoa(n); value != text {
		r.SetVar("HISTSIZE", text)
	}
}

// fcCountFloored is a HISTSIZE as a count of entries, with zsh's floor of one.
//
// The value arrives already evaluated — see fcSizeAssigned — so this is a
// decimal integer in every reachable case; the scan is what answers for a
// name something put a value on before the attribute was declared.
func fcCountFloored(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fcImportedSize(value)
	}
	return max(n, 1)
}

// fcStartSize lays HISTSIZE down as the parameter zsh carries rather than a
// name a script has to invent.
//
// Measured 2026-09-21 on zsh 5.9.2 under `zsh -f -c`, in a shell that has
// never mentioned the name: `typeset -p HISTSIZE` writes `typeset -i10
// HISTSIZE=30` and `echo "[${HISTSIZE-U}]"` writes `[30]`. So it is set, it
// is an integer, and thirty is its value.
//
// The **attribute is what does the arithmetic**, which is why it is declared
// here rather than the reading being written out twice: with it, `HISTSIZE=1+1`
// stores two, `HISTSIZE=" 2 "` stores two, `HISTSIZE=abc` stores nothing at
// all and `HISTSIZE=2x` is `bad math expression` and the end of a
// non-interactive shell — every row of that measured identical to zsh's
// before this, because the core already had the attribute (#4093).
//
// The **order is load-bearing**: an inherited value is scanned *before* the
// attribute goes on. zsh reads what the environment handed it rather than
// evaluating it — `HISTSIZE=2x` in the environment is two and no complaint,
// where the same text assigned is the error above — and declaring the
// attribute first would make a shell that dies at startup over a variable
// somebody exported years ago.
func fcStartSize(r *interp.Runner) {
	start := fcHistorySizeDefault
	if value, ok := r.GetVar("HISTSIZE"); ok {
		start = fcImportedSize(value)
	}
	r.SetIntegerParameter("HISTSIZE", 10)
	r.SetVar("HISTSIZE", strconv.Itoa(start))
}

// fcImportedSize is a HISTSIZE the shell was **handed**, which is a different
// reading of the same parameter and not a second copy of the one above.
//
// Measured 2026-09-21, zsh 5.9.2, `env -i HISTSIZE=<value> zsh -f -c` over
// three entries, as the list the shell ends up holding:
//
//	2x, " 2 ", 0x2          two — a leading integer, base prefix and all
//	1+1                     one: the scan stops at the operator
//	abc, -1, 0, empty       one, the floor
//	99                      ninety-nine
//
// So an inherited value is scanned and an assigned one is evaluated, and
// `1+1` is the row that says so: two ways in, two readings, and a shell that
// used one for both would either refuse `2x` at startup — which zsh does not
// — or let `1+1` through as one.
//
// The scan is also what the **parameter** ends up holding, because zsh
// rewrites it: `echo $HISTSIZE` answers `2` for an inherited `2x`. See
// fcStartSize, which stores what this answered before the attribute that
// would have evaluated it goes on.
func fcImportedSize(value string) int {
	text := strings.TrimSpace(value)
	for end := len(text); end > 0; end-- {
		if n, err := strconv.ParseInt(text[:end], 0, 64); err == nil {
			return max(int(n), 1)
		}
	}
	return 1
}

// fcHistorySize is the size the list is held at.
//
// The parameter, which fcStartSize keeps as a floored integer, so there is
// nothing to work out here. Only where `unset` has taken it away does this
// fall back to the size that was last in force — measured, `HISTSIZE=2;
// unset HISTSIZE` leaves the list held at two — and to the default where
// nothing ever set one.
func fcHistorySize(r *interp.Runner) int {
	if value, ok := r.GetVar("HISTSIZE"); ok {
		return fcCountFloored(value)
	}
	if held, ok := r.GetVar(fcHistorySizeInForce); ok {
		if n, err := strconv.Atoi(held); err == nil {
			return max(n, 1)
		}
	}
	return fcHistorySizeDefault
}

// fcTrimToSize drops the oldest entries over the size, counting them so that
// the ones left keep the numbers they had.
func fcTrimToSize(r *interp.Runner, keep int) {
	entries := fcEntries(r)
	if len(entries) <= keep {
		return
	}
	fcSetDropped(r, fcDropped(r)+len(entries)-keep)
	r.SetArray(fcHistoryStore, entries[len(entries)-keep:])
}

// fcFirst is the history number of the oldest entry the list holds.
func fcFirst(r *interp.Runner) int { return fcDropped(r) + 1 }

func fcDropped(r *interp.Runner) int {
	value, _ := r.GetVar(fcHistoryDropped)
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func fcSetDropped(r *interp.Runner, n int) {
	r.SetVar(fcHistoryDropped, strconv.Itoa(max(n, 0)))
}

// Every system call this file makes about a path a script named, with the
// gate asked first. Nothing above reaches the `os` package by another route —
// the same discipline `filesgate.go` and `mapfile.go` state, and the reason
// `internal/boundary`'s guard has one name for each direction.

// fcWriteFile puts the list down, having asked whether the script may modify
// that name.
//
// `0600`, which is the mode zsh leaves behind and is narrower than the umask
// would give: a history file holds what somebody typed. Measured, and the
// same answer bash's `history -w` gives.
//
// A refusal is silent here because AllowModify has already reported it, in
// the words a refused redirection gets.
func fcWriteFile(r *interp.Runner, name string, entries []string, appendTo bool) int {
	path := shellPath(r, name)
	if !r.AllowModify(r.ShellContext(), path) {
		return 1
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if appendTo {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		r.Diagnosef("fc: %s: %s\n", name, err)
		return 1
	}
	defer func() { _ = f.Close() }()
	for _, entry := range entries {
		if _, err := fmt.Fprintf(f, "%s\n", entry); err != nil {
			r.Diagnosef("fc: %s: %s\n", name, err)
			return 1
		}
	}
	return 0
}

// fcReadFile reads a history file's **physical lines**, having asked whether
// the script may read that path.
//
// Lines and not entries: what a line means is the dialect's encoding, which
// fcLoadLines asks the one decoder about. Keeping the two apart is what stops
// the gate and the encoding from having to be right in the same place.
//
// A refused read is reported by AllowReadPath the way a refused redirection's
// is — reading a file's contents is an open, and answering with an empty list
// and saying nothing would leave a script believing the file was empty.
func fcReadFile(r *interp.Runner, name string) ([]string, bool) {
	path := shellPath(r, name)
	if !r.AllowReadPath(r.ShellContext(), path) {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, true
	}
	return strings.Split(text, "\n"), true
}
