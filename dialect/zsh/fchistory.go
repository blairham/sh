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
//	fc -R file     the file's lines, appended to the list.
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

// registerFcHistory replaces the core `fc` with one that knows the file
// letters, delegating everything else back to it.
//
// Replacing rather than reimplementing: `-l`, `-n`, `-r`, `-s` and `-e` are
// the core's and stay the core's, and a dialect that copied them would be a
// second answer to maintain. [interp.Runner.Register] is documented to let a
// registration win over the built-in table for exactly this.
func registerFcHistory(r *interp.Runner) {
	core, ok := r.Builtin("fc")
	if !ok {
		return
	}
	r.Register("fc", func(r *interp.Runner, ctx context.Context, args []string) int {
		if letter, rest, found := fcFileLetter(args); found {
			return fcFile(r, letter, rest)
		}
		if entries := fcEntries(r); len(entries) > 0 && fcIsBareList(args) {
			return fcList(r, entries)
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

// fcIsBareList reports `fc -l` with no other letter, which is the one reading
// this file answers for that the core cannot: the core has no list.
func fcIsBareList(args []string) bool {
	return len(args) == 1 && args[0] == "-l"
}

// fcList writes the list the way zsh writes it, `%5d  %s` and numbered from
// one — the same shape bash's `history` uses, measured on both.
func fcList(r *interp.Runner, entries []string) int {
	for i, entry := range entries {
		_, _ = fmt.Fprintf(r.Out(), "%5d  %s\n", i+1, entry)
	}
	return 0
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
		r.SetArray(fcHistoryStore, append(fcEntries(r), lines...))
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

// fcRemember appends one line to the list. `print -s` is the caller, which is
// how a script puts something in this shell's history at all.
func fcRemember(r *interp.Runner, line string) {
	r.SetArray(fcHistoryStore, append(fcEntries(r), line))
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

// fcReadFile reads a history file's lines, having asked whether the script
// may read that path.
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
