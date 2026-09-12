// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `history`, which is bash's list of what was typed and the two files it
// keeps that list in.
//
// Measured 2026-09-12 against bash 5.3.15 under plain `bash -c`, with a
// scratch directory and no startup files.
//
// # Why a non-interactive shell has one at all
//
// It is the first surprise here and the reason this row is reachable where
// zsh's `fc -W` is not. bash maintains a history *list* in a script, and
// writes a history *file* from it, with no terminal anywhere:
//
//	$ bash -c 'history -w out; echo $?; wc -c < out'
//	0
//	       0
//
// The file is created even though the list is empty. zsh's `fc -W` writes
// nothing at all unless the shell is interactive — measured, including with
// a list loaded by `fc -R`, so it is not emptiness that stops it. So
// `history -w` is a write to a path a script names in a shell nobody is
// sitting at, which is exactly the shape a policy is for, and
// `make sandbox` graded the row `inert` with the note "not a builtin yet —
// falls through to a refused exec" until this landed.
//
// # Where the list is kept
//
// In the dialect, under a name no script can reach — the idiom `zstyle`,
// `emulate` and `zmodload` already use, and it is not only for hiding: a
// store held this way is cloned into a subshell, so `(history -s x)` cannot
// reach the parent's list, which is what bash does.
//
// It is deliberately *not* the interactive session's history. `repl` keeps
// that, `repl` imports `interp`, and a builtin cannot reach across that way
// round. Joining the two is a real question and is not this one: bash's own
// non-interactive list starts empty and is filled by `-s` and `-r`, which is
// what a script can observe and all this has to model.

const (
	// historyStore is the list itself, an indexed array oldest-first.
	historyStore = ".bash.history"
	// historyAppended is how many entries `-a` has already written, which is
	// the whole of what makes it an *append* rather than a second write.
	//
	// Measured: two `-a` to the same file write each entry once, and a `-w`
	// in between does not reset it — `history -s one; history -w k; history
	// -s two; history -a k` leaves `one` twice and `two` once, because the
	// write put the whole list down and the append then added everything
	// since the last *append*, which was none.
	historyAppended = ".bash.history.appended"
	// historyReadAt is how far `-n` has read each file, keyed by path.
	//
	// Per file rather than one counter, because that is what the letter
	// means: `-n` reads the lines it has not read *from that file*. Measured
	// both ways — a second `-n` on an unchanged file adds nothing, and a
	// second `-n` after the file grew adds only the new line. `-r` tracks
	// nothing at all and re-reads the whole file every time, which is the
	// difference between the two letters and is measured as `-r` twice
	// leaving two copies.
	historyReadAt = ".bash.history.readat"
)

// historyUsage is the line a refused option prints after the complaint, in
// bash's own words and with its own spacing.
const historyUsage = "history: usage: history [-c] [-d offset] [n] or " +
	"history -anrw [filename] or history -ps arg [arg...]"

func registerHistory(r *interp.Runner) {
	r.Register("history", historyBuiltin)
}

// historyFlags is the letters one call carried.
type historyFlags struct {
	clear  bool
	delete bool
	offset string
	append bool
	unread bool
	read   bool
	write  bool
	print  bool
	store  bool
}

// fileLetter reports whether one of the four letters that name a file was
// given, and refuses the combination bash refuses by taking the last.
func (f historyFlags) fileLetter() (byte, bool) {
	switch {
	case f.append:
		return 'a', true
	case f.unread:
		return 'n', true
	case f.read:
		return 'r', true
	case f.write:
		return 'w', true
	}
	return 0, false
}

func historyBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	flags, rest, code := historyOptions(r, args)
	if code != 0 {
		return code
	}

	// `-p` and `-s` take every remaining operand and answer on their own,
	// which is why bash's usage line prints them as a third form rather than
	// alongside the others.
	if flags.print {
		return historyPrint(r, rest)
	}
	if flags.store {
		historyAdd(r, strings.Join(rest, " "))
		return 0
	}

	if flags.clear {
		r.SetArray(historyStore, nil)
		// The append mark goes with it. A cleared list has written nothing,
		// and leaving the mark behind would make the next `-a` skip entries
		// that no longer have anything to do with the ones it counted.
		r.SetVar(historyAppended, "0")
	}
	if flags.delete {
		if code := historyDelete(r, flags.offset); code != 0 {
			return code
		}
	}

	if letter, ok := flags.fileLetter(); ok {
		return historyFile(r, letter, rest)
	}
	if flags.clear || flags.delete {
		// Both of those are complete on their own; bash prints nothing after
		// them and ignores a trailing operand — `history -c extra` is 0.
		return 0
	}
	return historyList(r, rest)
}

// historyOptions reads the leading option words.
//
// A letter bash does not have is `history: -Z: invalid option`, the usage
// line, and status 2 — which is two things worth being exact about: the
// status is 2 where nearly every other refusal here is 1, and the usage goes
// to standard error under the complaint rather than replacing it.
func historyOptions(r *interp.Runner, args []string) (flags historyFlags, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			switch letter := word[i]; letter {
			case 'c':
				flags.clear = true
			case 'a':
				flags.append = true
			case 'n':
				flags.unread = true
			case 'r':
				flags.read = true
			case 'w':
				flags.write = true
			case 'p':
				flags.print = true
			case 's':
				flags.store = true
			case 'd':
				value, ok := historyLetterValue(word, &rest, i)
				if !ok {
					r.Diagnosef("history: -d: option requires an argument\n")
					historyWriteUsage(r)
					return flags, nil, 2
				}
				flags.delete, flags.offset = true, value
				i = len(word)
			default:
				r.Diagnosef("history: -%c: invalid option\n", letter)
				historyWriteUsage(r)
				return flags, nil, 2
			}
		}
	}
	return flags, rest, 0
}

func historyWriteUsage(r *interp.Runner) {
	_, _ = fmt.Fprintf(r.Err(), "%s\n", historyUsage)
}

// historyLetterValue is the argument of `-d`: the rest of the word it is in,
// or the word after it.
func historyLetterValue(word string, rest *[]string, i int) (string, bool) {
	if i+1 < len(word) {
		return word[i+1:], true
	}
	if len(*rest) == 0 {
		return "", false
	}
	value := (*rest)[0]
	*rest = (*rest)[1:]
	return value, true
}

// historyList writes the list the way bash writes it: `%5d  %s`, oldest
// first, numbered from one.
//
// An operand is a count of the most recent entries, and the numbers do not
// restart — `history 1` on a two-entry list writes `    2  b`, so the number
// is the entry's position in the whole list rather than in what was printed.
func historyList(r *interp.Runner, rest []string) int {
	entries := historyEntries(r)
	from := 0
	if len(rest) > 0 {
		n, err := strconv.Atoi(rest[0])
		if err != nil {
			r.Diagnosef("history: %s: numeric argument required\n", rest[0])
			historyWriteUsage(r)
			return 2
		}
		if n < len(entries) {
			from = len(entries) - n
		}
	}
	for i := from; i < len(entries); i++ {
		_, _ = fmt.Fprintf(r.Out(), "%5d  %s\n", i+1, entries[i])
	}
	return 0
}

// historyDelete removes one entry, counting from one.
//
// Out of range is `history: 9: history position out of range` at 1, and zero
// is out of range too — the list is one-based at both ends, measured.
func historyDelete(r *interp.Runner, offset string) int {
	entries := historyEntries(r)
	n, err := strconv.Atoi(offset)
	if err != nil {
		r.Diagnosef("history: %s: numeric argument required\n", offset)
		historyWriteUsage(r)
		return 2
	}
	if n < 1 || n > len(entries) {
		r.Diagnosef("history: %d: history position out of range\n", n)
		return 1
	}
	r.SetArray(historyStore, append(entries[:n-1:n-1], entries[n:]...))
	return 0
}

// historyPrint is `-p`: each operand after history expansion, one per line.
//
// This shell has no history expansion, and that is the whole of the answer
// rather than a gap in it. An operand with no `!` in it expands to itself and
// is written back — measured, `history -p foo bar` writes two lines at 0. One
// carrying a `!` has nothing to expand against, and bash's own answer in a
// shell that cannot expand it is `history: !!: history expansion failed` at
// 1, which is the same sentence for the same reason.
func historyPrint(r *interp.Runner, rest []string) int {
	for _, arg := range rest {
		if strings.Contains(arg, "!") {
			r.Diagnosef("history: %s: history expansion failed\n", arg)
			return 1
		}
	}
	for _, arg := range rest {
		_, _ = fmt.Fprintf(r.Out(), "%s\n", arg)
	}
	return 0
}

// historyFile is the four letters that name a file: `-w`, `-a`, `-r`, `-n`.
//
// The path is the operand, or `$HISTFILE` when there is none. Neither is
// `history: HISTFILE: parameter null or not set` at 1 — bash names the
// *variable* rather than saying an operand is missing, which is the right way
// round: the letter has a default and the default is unset.
func historyFile(r *interp.Runner, letter byte, rest []string) int {
	name := ""
	if len(rest) > 0 {
		name = rest[0]
	} else if value, ok := r.GetVar("HISTFILE"); ok && value != "" {
		name = value
	}
	if name == "" {
		r.Diagnosef("history: HISTFILE: parameter null or not set\n")
		return 1
	}
	switch letter {
	case 'w':
		return historyWriteFile(r, name, historyEntries(r), false)
	case 'a':
		entries := historyEntries(r)
		from := historyMark(r)
		if from > len(entries) {
			from = len(entries)
		}
		if code := historyWriteFile(r, name, entries[from:], true); code != 0 {
			return code
		}
		r.SetVar(historyAppended, strconv.Itoa(len(entries)))
		return 0
	case 'r':
		lines, ok := historyReadFile(r, name)
		if !ok {
			return 1
		}
		for _, line := range lines {
			historyAdd(r, line)
		}
		return 0
	default: // 'n'
		lines, ok := historyReadFile(r, name)
		if !ok {
			return 1
		}
		at := historyReadMark(r, name)
		if at > len(lines) {
			at = len(lines)
		}
		for _, line := range lines[at:] {
			historyAdd(r, line)
		}
		r.SetAssocElement(historyReadAt, shellPath(r, name), strconv.Itoa(len(lines)))
		return 0
	}
}

// The list, as the store holds it.

func historyEntries(r *interp.Runner) []string {
	entries, ok := r.GetArray(historyStore)
	if !ok {
		return nil
	}
	return append([]string(nil), entries...)
}

func historyAdd(r *interp.Runner, line string) {
	r.SetArray(historyStore, append(historyEntries(r), line))
}

func historyMark(r *interp.Runner) int {
	value, ok := r.GetVar(historyAppended)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func historyReadMark(r *interp.Runner, name string) int {
	table, ok := r.GetAssoc(historyReadAt)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(table[shellPath(r, name)])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Every system call this builtin makes about a path the script named, with
// the gate asked first. Nothing above reaches the `os` package by another
// route.
//
// The same discipline `dialect/zsh/filesgate.go` opens with, and the reason
// it is two functions rather than four call sites: the property is checkable
// by reading them, and `internal/boundary`'s guard has one name each to hold.

// historyWriteFile puts entries down, having asked whether the script may
// modify that name.
//
// `0600` is the mode bash leaves behind, which is not an ordinary default —
// a history file holds what somebody typed, so the shell that writes it
// narrows the mode rather than taking the umask's answer. Measured.
//
// A refusal is silent here because AllowModify has already reported it, in
// the same words a refused redirection gets.
func historyWriteFile(r *interp.Runner, name string, entries []string, appendTo bool) int {
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
		r.Diagnosef("history: %s: %s\n", name, historyReason(err))
		return 1
	}
	defer func() { _ = f.Close() }()
	for _, entry := range entries {
		if _, err := fmt.Fprintf(f, "%s\n", entry); err != nil {
			r.Diagnosef("history: %s: %s\n", name, historyReason(err))
			return 1
		}
	}
	return 0
}

// historyReadFile reads a history file's lines, having asked whether the
// script may read that path.
//
// A refused read is reported the way a refused redirection's is — reading a
// file's contents is an open, and a builtin that answered with an empty list
// and said nothing would leave a script believing the file was empty.
func historyReadFile(r *interp.Runner, name string) ([]string, bool) {
	path := shellPath(r, name)
	if !r.AllowReadPath(r.ShellContext(), path) {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		r.Diagnosef("history: %s: %s\n", name, historyReason(err))
		return nil, false
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, true
	}
	return strings.Split(text, "\n"), true
}

// historyReason is the part of an error a shell says, without the operation
// and path Go's own wrapper repeats in front of it.
func historyReason(err error) string {
	var perr *os.PathError
	if errors.As(err, &perr) {
		return perr.Err.Error()
	}
	return err.Error()
}

// shellPath resolves a name against the *shell's* directory rather than the
// process's, which is the difference between the file the script meant and
// whatever that name happens to be beside the program that embedded this
// shell. The same rule dialect/zsh/shellpath.go states at length.
func shellPath(r *interp.Runner, name string) string {
	if name == "" || filepath.IsAbs(name) || r.Dir == "" {
		return name
	}
	return filepath.Join(r.Dir, name)
}
