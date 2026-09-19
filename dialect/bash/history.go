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
	"github.com/blairham/sh/repl"
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
	// historyUnwritten is how many of the newest entries this session added
	// and no `-a` has written yet — a **count** from the end of the list and
	// not a position in it, which is the whole of what makes `-a` an
	// *append* rather than a second write.
	//
	// Measured: two `-a` to the same file write each entry once, and a `-w`
	// in between does not reset it — `history -s one; history -w k; history
	// -s two; history -a k` leaves `one` twice and `two` once, because the
	// write put the whole list down and the append then added everything
	// since the last *append*, which was none.
	//
	// A count rather than a position, measured 2026-09-16 on bash 5.3.20 by
	// what the file holds when the shell ends, which appends the same count
	// `-a` does:
	//
	//   - a line the reader records and an entry `-s` adds each count one, and
	//     an entry `-r` or `-n` reads from a file counts nothing — `history -r`
	//     then `history` leaves the file with `gamma three` and `history`,
	//     the newest two entries, because two lines were recorded and the
	//     three read are older than neither;
	//   - `-d` takes one off, whichever entry it removed: three `history -d 1`
	//     after `echo q` leave only the last `history -d 1` appended;
	//   - the builtin's own line that `-p` and `-s` drop takes one off;
	//   - `-c` puts it back to nothing.
	historyUnwritten = ".bash.history.unwritten"
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
	// historyOwnLine says the line the reader handed over last joined the
	// list, which is the entry `-p` and `-s` drop as their own. Measured
	// 2026-09-16 on bash 5.3.20: with `HISTIGNORE='history*'`, `history -p
	// x` and `history -s z` are not recorded and the entries before them
	// survive both calls, so a builtin whose line was left out has nothing of
	// its own to drop.
	historyOwnLine = ".bash.history.ownline"
	// historyDropped is how many entries HISTSIZE has taken off the front of
	// the list, which is what the numbers go on from: measured, `HISTSIZE=2`
	// after `echo a`, `echo b`, `echo c` lists `3  HISTSIZE=2` and
	// `4  history`, and `!1` is then an event the list does not hold.
	historyDropped = ".bash.history.dropped"
)

// historyUsage is the line a refused option prints after the complaint, in
// bash's own words and with its own spacing.
const historyUsage = "history: usage: history [-c] [-d offset] [n] or " +
	"history -anrw [filename] or history -ps arg [arg...]"

func registerHistory(r *interp.Runner) {
	r.Register("history", historyBuiltin)
	// And the same list to the front end, which is what fills it when bash
	// reads a *script* with `set -o history` written: every command it runs
	// joins the list the designators index. Functions taking a runner rather
	// than closures over this one — see interp.Runner.SetHistoryStore, where
	// the subshell reason is written down.
	r.SetHistoryStore(historyEntries, historyRecord)
	r.SetHistoryFile(historyStartFile, historyFinishFile)
	r.SetHistoryNumbering(historyFirst)
	// And the parameter whose assignment truncates the file where it
	// stands, which is the moment nothing else could reach: see
	// historyTruncateFile for the nine shapes it was measured over, and
	// interp.Runner.SetAssignmentAction for why a stored name needs a seam
	// of its own rather than a producer's.
	r.SetAssignmentAction("HISTFILESIZE", func(rr *interp.Runner, _ string) {
		historyTruncateFile(rr)
	})
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
	// Both of these drop the builtin's **own** line from the list first.
	// `-s` is documented to — "the last command in the history list is
	// removed before the args are added" — and `-p` is measured to: after
	// `history -p "!!"`, a later `history` does not list the `-p` line.
	//
	// Only where the front end put that line there. Measured, `history -s a`
	// followed by `history -s b` in a shell with no list leaves both, because
	// neither line was ever recorded and there is nothing of the builtin's
	// own to drop.
	if flags.print || flags.store {
		historyDropOwnLine(r)
	}
	if flags.print {
		return historyPrint(r, rest)
	}
	if flags.store {
		historyAdd(r, strings.Join(rest, " "))
		return 0
	}

	if flags.clear {
		r.SetArray(historyStore, nil)
		// The count goes with it: a cleared list holds nothing for `-a` to
		// write, and measured, `echo 1; history -c; echo 2` leaves only
		// `echo 2` appended when the shell ends.
		historySetUnwritten(r, 0)
		historySetDropped(r, 0)
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

// historyTooManyOperands is the refusal for a second operand, which reads one
// count and has nowhere to put another. It was silent at 0 here (#3468).
//
// Measured 2026-09-18 on bash 5.3.20 from a script file, `; echo a=$?` behind
// the call and `echo b=$?` on the line after it:
//
//	history 1 2       history: too many arguments   no a=, then b=2
//	history 1 x       history: too many arguments   no a=, then b=2
//	history -- 1 2    history: too many arguments   no a=, then b=2
//	history x 1       history: x: numeric …         a=2, then b=0
//	history -c 1 2    nothing at all, 0
//
// So the count is checked **after** the first operand has been read as a
// number, and only where no letter took the operands for itself — `-c`, `-d`,
// `-p`, `-s` and the file letters each ignore what is left, which was already
// measured and is why this is in historyList rather than in the builtin.
//
// It is the one refusal in this builtin that costs the rest of the command:
// the `; echo a=$?` never runs and the next line reports 2. Every other
// refusal here — the invalid option, the missing `-d` argument, the operand
// that is not a number — leaves the line to finish. See
// interp.Runner.GiveUpTheCommandAt, which is the door this and the core's own
// give-ups share, and where the `-c` route's answer of 1 is written down.
func historyTooManyOperands(r *interp.Runner) int {
	r.Diagnosef("history: too many arguments\n")
	return r.GiveUpTheCommandAt(2)
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
		count, err := strconv.Atoi(rest[0])
		if err != nil {
			// No usage block under this one, which is the difference
			// between a complaint about an *operand* and a complaint about
			// how the builtin was called. Measured 2026-09-18: `history x`
			// is the sentence alone at 2, where `history -q` and
			// `history -d` each print the usage line under theirs.
			r.Diagnosef("history: %s: numeric argument required\n", rest[0])
			return 2
		}
		if len(rest) > 1 {
			return historyTooManyOperands(r)
		}
		if count < len(entries) {
			from = len(entries) - count
		}
	}
	for i := from; i < len(entries); i++ {
		_, _ = fmt.Fprintf(r.Out(), "%5d  %s\n", historyFirst(r)+i, entries[i])
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
	// A history number, so counted from wherever HISTSIZE left the front of
	// the list: measured, after `HISTSIZE=3` has dropped the first entry,
	// `history -d 1` is out of range.
	i := n - historyFirst(r)
	if i < 0 || i >= len(entries) {
		r.Diagnosef("history: %d: history position out of range\n", n)
		return 1
	}
	r.SetArray(historyStore, append(entries[:i:i], entries[i+1:]...))
	historySetUnwritten(r, historyUnwrittenCount(r)-1)
	return 0
}

// historyPrint is `-p`: each operand after history expansion, one per line.
//
// Against the same list everything else here keeps, and **without** asking
// whether `set -H` is on: measured, `set -o history; echo one two three;
// history -p "!!"` writes `echo one two three` with the letter never written.
// The letter decides whether the shell expands what it *reads*; this operand
// was handed to the builtin on purpose.
//
// An operand with no reference in it expands to itself and is written back —
// measured, `history -p foo bar` writes two lines at 0. A reference the list
// does not hold is `history: !!: history expansion failed` at 1, which is
// also bash's answer in a shell whose list is empty, and none of the operands
// is written when one of them fails. Nothing is added to the list: `-p` is
// the way to look at what a reference resolves to without committing to it.
func historyPrint(r *interp.Runner, rest []string) int {
	out := make([]string, 0, len(rest))
	entries := historyEntries(r)
	for _, arg := range rest {
		res, err := r.ExpandHistoryAlways(arg, entries, historyFirst(r))
		if err != nil {
			r.Diagnosef("history: %s: history expansion failed\n", arg)
			return 1
		}
		out = append(out, res.Line)
	}
	for _, arg := range out {
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
		return historyWriteFile(r, name, historyEntries(r), false, false)
	case 'a':
		if code := historyWriteFile(r, name, historyNewest(r), true, false); code != 0 {
			return code
		}
		historySetUnwritten(r, 0)
		return 0
	case 'r':
		lines, ok := historyReadFile(r, name, false)
		if !ok {
			return 1
		}
		for _, line := range lines {
			historyLoad(r, line)
		}
		return 0
	default: // 'n'
		lines, ok := historyReadFile(r, name, false)
		if !ok {
			return 1
		}
		at := historyReadMark(r, name)
		if at > len(lines) {
			at = len(lines)
		}
		for _, line := range lines[at:] {
			historyLoad(r, line)
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

// historyDropOwnLine removes the line the builtin was written on, which the
// front end reading the program has already recorded.
func historyDropOwnLine(r *interp.Runner) {
	if !r.HistoryListFilledByTheReader() {
		return
	}
	if own, _ := r.GetVar(historyOwnLine); own != "1" {
		return
	}
	entries := historyEntries(r)
	if len(entries) == 0 {
		return
	}
	r.SetArray(historyStore, entries[:len(entries)-1])
	historySetUnwritten(r, historyUnwrittenCount(r)-1)
}

// historyRecord is a command the reader hands over, which joins the list unless
// HISTCONTROL or HISTIGNORE leaves it out — the same rules a prompt reads, see
// repl.HistoryIgnores.
//
// `erasedups` is the one word of HISTCONTROL a prompt does not read here, and
// a script's list does: measured, `echo a`, `echo b`, `echo a` under
// `HISTCONTROL=erasedups` lists `echo b` and then `echo a`, the earlier copy
// gone and the new one kept at the end.
func historyRecord(r *interp.Runner, line string) {
	entries := historyEntries(r)
	previous := ""
	if len(entries) > 0 {
		previous = entries[len(entries)-1]
	}
	if repl.HistoryIgnores(HistoryStyle(), r, line, previous) {
		r.SetVar(historyOwnLine, "0")
		return
	}
	if historyErasesDups(r) {
		kept := entries[:0]
		for _, entry := range entries {
			if entry != line {
				kept = append(kept, entry)
			}
		}
		if len(kept) != len(entries) {
			r.SetArray(historyStore, kept)
		}
	}
	historyAdd(r, line)
	r.SetVar(historyOwnLine, "1")
}

// historyErasesDups reports `erasedups` in HISTCONTROL's colon-separated words.
func historyErasesDups(r *interp.Runner) bool {
	v, _ := r.GetVar("HISTCONTROL")
	for _, word := range strings.Split(v, ":") {
		if word == "erasedups" {
			return true
		}
	}
	return false
}

// historyAdd is an entry this session made — a line the reader recorded or
// one `-s` stored — which a later `-a`, or the shell ending, will write.
func historyAdd(r *interp.Runner, line string) {
	historyAppend(r, line, true)
	historySetUnwritten(r, historyUnwrittenCount(r)+1)
}

// historyLoad is an entry read from a file, which is already written and is
// not counted.
func historyLoad(r *interp.Runner, line string) {
	historyAppend(r, line, false)
}

// historyAppend puts one entry at the end of the list, keeping no more than
// HISTSIZE of them, and moves the numbering on for what fell off the front
// where numbered is set.
//
// The numbers are the part worth being exact about, and they were measured on
// bash 5.3.20 one shape at a time rather than derived:
//
//   - `echo a`, `echo b`, `echo c`, `HISTSIZE=3`, `echo d`, `echo e`,
//     `history` lists `4 echo d`, `5 echo e`, `6 history` — every entry
//     pushed off a full list moves the numbers on by one;
//   - the same with `HISTSIZE=2` and then `history` lists `3 HISTSIZE=2`,
//     `4 history` — a list already longer than the new size loses the
//     excess at once, and the numbers move on by one fewer than it lost;
//   - `HISTSIZE=1` before a two-line HISTFILE is read and then `history`
//     lists `2 history` — an entry read from a file moves nothing.
func historyAppend(r *interp.Runner, line string, numbered bool) {
	entries := historyEntries(r)
	keep, bounded := historySize(r)
	dropped := historyDroppedCount(r)
	if bounded && len(entries) > keep {
		if numbered {
			dropped += len(entries) - keep - 1
		}
		entries = entries[len(entries)-keep:]
	}
	entries = append(entries, line)
	if bounded && len(entries) > keep {
		if numbered {
			dropped += len(entries) - keep
		}
		entries = entries[len(entries)-keep:]
	}
	historySetDropped(r, dropped)
	r.SetArray(historyStore, entries)
}

// historySize is HISTSIZE as a count of entries the list keeps, or false where
// it keeps them all. Measured on bash 5.3.20: `HISTSIZE=2` after three
// commands lists only the newest two, `HISTSIZE=0` keeps nothing — `history`
// then lists nothing at all, its own line included — and a negative value
// keeps everything.
func historySize(r *interp.Runner) (int, bool) {
	value, ok := r.GetVar("HISTSIZE")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// historyFirst is the history number of the oldest entry the list holds.
func historyFirst(r *interp.Runner) int { return historyDroppedCount(r) + 1 }

func historyDroppedCount(r *interp.Runner) int {
	value, _ := r.GetVar(historyDropped)
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func historySetDropped(r *interp.Runner, n int) {
	r.SetVar(historyDropped, strconv.Itoa(max(n, 0)))
}

func historyUnwrittenCount(r *interp.Runner) int {
	value, ok := r.GetVar(historyUnwritten)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// historySetUnwritten stores the count, which never goes below nothing.
func historySetUnwritten(r *interp.Runner, n int) {
	r.SetVar(historyUnwritten, strconv.Itoa(max(n, 0)))
}

// historyNewest is the entries the count covers: the newest ones, or the
// whole list where it has lost some since they were counted.
func historyNewest(r *interp.Runner) []string {
	entries := historyEntries(r)
	n := min(historyUnwrittenCount(r), len(entries))
	return entries[len(entries)-n:]
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
func historyWriteFile(r *interp.Runner, name string, entries []string, appendTo, quiet bool) int {
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
		historyFileComplaint(r, quiet, name, err)
		return 1
	}
	defer func() { _ = f.Close() }()
	for _, entry := range entries {
		if _, err := fmt.Fprintf(f, "%s\n", entry); err != nil {
			historyFileComplaint(r, quiet, name, err)
			return 1
		}
	}
	return 0
}

// historyFileComplaint says why a history file could not be read or written,
// unless the caller is the shell reading or writing one on its own account —
// the list being turned on, or the shell ending — which says nothing:
// measured, a HISTFILE in a missing directory is silent at both. A refusal by
// the gate is not this, and is reported the way AllowModify and AllowReadPath
// report every one.
func historyFileComplaint(r *interp.Runner, quiet bool, name string, err error) {
	if !quiet {
		r.Diagnosef("history: %s: %s\n", name, historyReason(err))
	}
}

// historyReadFile reads a history file's lines, having asked whether the
// script may read that path.
//
// A refused read is reported the way a refused redirection's is — reading a
// file's contents is an open, and a builtin that answered with an empty list
// and said nothing would leave a script believing the file was empty.
func historyReadFile(r *interp.Runner, name string, quiet bool) ([]string, bool) {
	path := shellPath(r, name)
	if !r.AllowReadPath(r.ShellContext(), path) {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		historyFileComplaint(r, quiet, name, err)
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
