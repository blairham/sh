// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	//   - `-d` takes off what it removed, whichever entries those were: three
	//     `history -d 1` after `echo q` leave only the last `history -d 1`
	//     appended, and `history -d 2-3` over four entries takes three off
	//     rather than one;
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
	// historyInForce is the size the list is capped at, which is **not** what
	// HISTSIZE says. A value that is not a count leaves the last one that was
	// in force — see historySize, where the six shapes are written down — so
	// the cap is state beside the list rather than a reading of the variable.
	//
	// A decimal count, or `-1` for a list with no cap at all. Absent until
	// something sets the size or the list is turned on, which is what lets
	// the value inherited from the environment be the first one in force.
	historyInForce = ".bash.history.inforce"
	// historyNoCap is what historyInForce holds for a list keeping
	// everything, spelled as the value a script writes to say the same thing.
	historyNoCap = "-1"
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
	// And whether the list already holds the line the builtin now running is
	// written on, which three builtins need and used to be read by one:
	// `history -s` and `history -p` drop it, and the core's `fc` both ends
	// its default range before it and replaces it with what `-s` re-runs.
	// One answer registered here rather than a second one in the core — see
	// interp.Runner.SetHistoryOwnLine.
	r.SetHistoryOwnLine(historyHasOwnLine, historyDropOwnLine)
	r.SetHistoryFile(historyStartFile, historyFinishFile)
	// And the seam for the one shell that route leaves out: an interactive
	// session, whose file the front end keeps. Its lines still belong in this
	// list, because `history`, `fc` and every `!` reference read this one.
	// See interp.Runner.SetHistorySeed.
	r.SetHistorySeed(historySeed)
	r.SetHistoryNumbering(historyFirst)
	// And the parameter whose assignment truncates the file where it
	// stands, which is the moment nothing else could reach: see
	// historyTruncateFile for the nine shapes it was measured over, and
	// interp.Runner.SetAssignmentAction for why a stored name needs a seam
	// of its own rather than a producer's.
	r.SetAssignmentAction("HISTFILESIZE", func(rr *interp.Runner, _ string) {
		historyTruncateFile(rr)
	})
	// And its sibling, which trims the *list* where it stands rather than
	// the file. The same seam and for the same reason: the cap on the way in
	// cannot see an assignment that lands on a list already longer than it,
	// and a lazy reading — trimming only when the next entry arrives —
	// answers the longer list to a `history` that looks first (#4031). See
	// historyStifle for the numbering, which is the half that makes this
	// more than dropping entries.
	r.SetAssignmentAction("HISTSIZE", func(rr *interp.Runner, value string) {
		historySizeAssigned(rr, value)
	})
	// And the removal, which is the other way the cap is lifted and cannot
	// be read back off the variable: the size in force outlives a HISTSIZE
	// that is no longer a count, so a shell that only heard assignments
	// could not tell an `unset` from a `HISTSIZE=abc` (#4054). See
	// interp.Runner.SetUnsetAction.
	r.SetUnsetAction("HISTSIZE", func(rr *interp.Runner) {
		rr.SetVar(historyInForce, historyNoCap)
	})
	// And the variable that decides whether an entry keeps the time it ran
	// at. A seam rather than a reading, because the answer outlives the
	// variable: an `unset` does not stop the recording, so a shell that only
	// ever read HISTTIMEFORMAT could not tell an entry made before it was
	// first assigned from one made after. See historytimes.go.
	r.SetAssignmentAction("HISTTIMEFORMAT", func(rr *interp.Runner, _ string) {
		historyTimesAssigned(rr)
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

// fileLetter reports which of the four letters that name a file was given.
//
// At most one can have been, because [historyFlags.tooManyFileLetters] has
// already refused the call where two were — so the order of the cases below
// decides nothing and is not a precedence.
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

// tooManyFileLetters reports the combination bash refuses: two *different*
// letters out of `-anrw` on one call.
//
// Different, not repeated — measured 2026-09-22 on bash 5.3.20, `history -a
// -a` is accepted and does the append once, while `-an`, `-na`, `-anrw` and
// `-c -a -r` are each `history: cannot use more than one of -anrw` at status
// 1. The refusal stands in front of the whole builtin and not only in front
// of the file work: the `-c` of that last one does not clear the list.
//
// No usage block under it, which is the difference between a complaint about
// a *combination* and one about a letter the builtin does not have: `-Z` is
// status 2 with the usage line, this is status 1 without it.
func (f historyFlags) tooManyFileLetters() bool {
	n := 0
	for _, given := range []bool{f.append, f.unread, f.read, f.write} {
		if given {
			n++
		}
	}
	return n > 1
}

func historyBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	flags, rest, code := historyOptions(r, args)
	if code != 0 {
		return code
	}
	if flags.tooManyFileLetters() {
		r.Diagnosef("history: cannot use more than one of -anrw\n")
		return 1
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
	//
	// The line is taken **once by `-s`** and every time by `-p`, which is
	// the difference #4072 was about and is not what either letter's wording
	// suggests. Measured 2026-09-21 on bash 5.3.20, list emptied by
	// `HISTIGNORE` so only the operands show:
	//
	//   - `true; history -s a; history -s b; history -s c` lists `a`, `b`,
	//     `c`. Only the first `-s` takes the line the three are written on;
	//     a shell taking it again would answer `c` alone, the later calls
	//     each eating the entry before them.
	//   - `history -s pre` then `true; history -p pp; history -s b` lists
	//     `b` alone, so the `-s` after a `-p` took `pre` — `-p` leaves the
	//     line marked as still there.
	//   - `history -s pre1; history -s pre2` then `true; history -p x;
	//     history -p y` lists `pre1` alone: two `-p` on one line take two
	//     entries.
	//   - `history -s pre` then `true; history -s a; history -p x` lists
	//     `pre` and `a`, so the `-s` cleared the mark for the `-p` too.
	//
	// And where the mark is set with nothing left to take, the builtin does
	// not fall through to its own work: `true; history -p x; history -p y`
	// prints `x` and then nothing at status 1, and `true; history -p x;
	// history -s b; history -s c` leaves the list empty at status 0 — the
	// operands of a `-s` that found nothing to replace are dropped, and the
	// mark it never took survives for the next one.
	if flags.print || flags.store {
		if len(rest) == 0 {
			// Nothing after the letter is nothing to do, and it is the
			// **operand count** that says so rather than emptiness: measured
			// 2026-09-21 on bash 5.3.20, `history -s ''` stores an empty
			// entry where `history -s` stores none, so the two are
			// distinguishable and the join of no words is not an entry.
			//
			// The builtin's own line survives it, which is the discriminating
			// case and is why this stands in front of historyTakeOwnLine
			// rather than inside the store below: with the reader recording,
			// `history -s` on its own line leaves that line in the list,
			// where `history -s a` takes it out. `history -p` answers the
			// same shape, and `history -s --` is no operands rather than one
			// empty one — the `--` is eaten by the option reader and nothing
			// is left behind it. Both at 0, with no diagnostic, and neither
			// reaches the mark the take above reads. #4068.
			return 0
		}
		if !historyTakeOwnLine(r, flags.store) {
			if flags.print {
				return 1
			}
			return 0
		}
	}
	if flags.print {
		return historyPrint(r, rest)
	}
	if flags.store {
		// Through the same gate the reader's lines pass rather than straight
		// into the list: HISTIGNORE and HISTCONTROL are consulted for an
		// operand too, which is #4057. See historyWanted for what each of
		// them was measured to do to one.
		if line := strings.Join(rest, " "); historyWanted(r, line) {
			historyAdd(r, line)
		}
		return 0
	}

	if flags.clear {
		historySetList(r, nil, nil)
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
	// The time in front of the command where this shell was told to draw
	// one, and the `??` an entry with none gets. See historytimes.go, where
	// the three rules HISTTIMEFORMAT carries are measured.
	format, timed := historyTimeFormat(r)
	times := historyTimesOf(r, len(entries))
	for i := from; i < len(entries); i++ {
		stamp := ""
		if timed {
			stamp = historyDrawnTime(format, times[i])
		}
		_, _ = fmt.Fprintf(r.Out(), "%5d  %s%s\n", historyFirst(r)+i, stamp, entries[i])
	}
	return 0
}

// historyDelete removes one entry, or the whole span of a range, counting
// from one.
//
// Measured 2026-09-21 against bash 5.3.20, `env -i` and no startup files,
// over a nine-entry list built with `history -s` (#4010). The operand is one
// of three shapes and each has its **own wording**, which is the whole of
// what made this row expensive: a refusal here costs two lines rather than
// one wherever this shell prints the usage block bash does not.
//
//   - a lone offset the list holds — `3`, `+3`, `007`, `-1` — deletes it;
//   - a lone offset it does not hold is `history: 0: history position out of
//     range` at 1, the number **as it was written**: `-0` and `0777` and
//     `+50` are each echoed back unchanged;
//   - a lone operand that is not a number at all is a third wording,
//     `history: @42: invalid number`, also at 1;
//   - and neither of the two carries the usage block. That block belongs to a
//     complaint about how the builtin was *called* — the invalid letter, the
//     missing `-d` argument — and not to one about an operand. See
//     historyList, which already draws the same line for `history x`.
//
// A **range** is the fourth shape and the reason for the rest of this: bash
// takes `history -d 2-4` and deletes all three. Out of range, it names the
// end that is out of range rather than the operand — `16-40` on a nine-entry
// list is `history: 16: …` and `1-200` is `history: 200: …`, the start being
// checked first — and where a side is not a number at all it falls back to
// naming the whole operand at the same wording: `5-0xaf` and `@42-3` and
// `2-4-6` are each `history: <operand>: history position out of range`.
func historyDelete(r *interp.Runner, offset string) int {
	if start, end, ok := historyRange(offset); ok {
		return historyDeleteRange(r, offset, start, end)
	}
	n, ok := historyNumber(offset)
	if !ok {
		// A fourth wording, and the one prefix that earns it: measured,
		// `0x9` and `0x` are `invalid hex number` where `0X9`, `+0x9`,
		// `0b101` and `9abc` are all plain `invalid number`.
		if strings.HasPrefix(strings.TrimSpace(offset), "0x") {
			r.Diagnosef("history: %s: invalid hex number\n", offset)
		} else {
			r.Diagnosef("history: %s: invalid number\n", offset)
		}
		return 1
	}
	// A history number, so counted from wherever HISTSIZE left the front of
	// the list: measured, after `HISTSIZE=3` has dropped the first entry,
	// `history -d 1` is out of range.
	first, last := historyBounds(r)
	num, inRange := historyPosition(n, first, last)
	if !inRange || num < first {
		r.Diagnosef("history: %s: history position out of range\n", offset)
		return 1
	}
	historyRemove(r, num-first, num-first)
	return 0
}

// historyDeleteRange is `-d N-M`, whose two ends are read the way one offset
// is and then bounded differently at the **low** end.
//
// A lone `0` is out of range; `0-3` is not, and deletes the first three.
// Measured, and the pair is the discriminator: a non-negative end below the
// first entry is clamped to it rather than refused, so `1-0` and `0-0` each
// take the oldest entry alone. A *negative* end is not clamped — it counts
// back from the newest, and one that counts back past the oldest is refused
// even where it lands on zero, so `-10-9` on a nine-entry list is
// `history: -10: history position out of range` while `-9-9` deletes the lot.
//
// A range whose start is after its end deletes nothing, says nothing, and is
// 1 — measured on `4-2` and on `3-0`, the second being a range whose end was
// clamped underneath its start.
func historyDeleteRange(r *interp.Runner, offset, start, end string) int {
	first, last := historyBounds(r)
	from, ok := historyNumber(start)
	if !ok {
		r.Diagnosef("history: %s: history position out of range\n", offset)
		return 1
	}
	to, ok := historyNumber(end)
	if !ok {
		r.Diagnosef("history: %s: history position out of range\n", offset)
		return 1
	}
	fromNum, inRange := historyPosition(from, first, last)
	if !inRange {
		r.Diagnosef("history: %s: history position out of range\n", start)
		return 1
	}
	toNum, inRange := historyPosition(to, first, last)
	if !inRange {
		r.Diagnosef("history: %s: history position out of range\n", end)
		return 1
	}
	i, j := max(fromNum-first, 0), max(toNum-first, 0)
	if i > j {
		return 1
	}
	historyRemove(r, i, j)
	return 0
}

// historyRange splits an operand at its range separator, which is the first
// `-` **after the first character** of the operand as written.
//
// A leading `-` is therefore a sign and never a separator, so `-1--1` is the
// newest entry alone and `--1` is a range whose start is the word `-`. The
// first character is the whole of the exception and nothing else is skipped:
// measured, a *space* in front of the sign is not, so ` -1` splits into ` `
// and `1` and is refused where `-1` is taken. Whitespace inside a side is
// fine — ` 16 - 40 ` is the range 16 to 40.
func historyRange(offset string) (start, end string, ok bool) {
	if len(offset) < 2 {
		return "", "", false
	}
	i := strings.IndexByte(offset[1:], '-')
	if i < 0 {
		return "", "", false
	}
	return offset[:i+1], offset[i+2:], true
}

// historyNumber reads one written offset. Surrounding whitespace is allowed
// and a leading zero is not an octal prefix: measured, `007` is seven, `08`
// is eight and `010` is ten.
func historyNumber(token string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(token))
	return n, err == nil
}

// historyBounds is the history numbers of the oldest and newest entries. An
// empty list leaves last below first, which refuses every offset.
func historyBounds(r *interp.Runner) (first, last int) {
	first = historyFirst(r)
	return first, first + len(historyEntries(r)) - 1
}

// historyPosition turns a written offset into a history number. A negative
// counts back from the newest, `-1` being the newest itself, and one that
// counts back past the oldest entry is out of range.
//
// A non-negative below the oldest is *not* refused here, because a range
// clamps it and a lone offset does not; historyDelete adds that check. An
// **empty** list is the exception at that end and refuses every offset,
// which is where the two disagree: measured, `0-3` takes the first three of
// a nine-entry list and is `history: 0: history position out of range`
// against no list at all.
func historyPosition(n, first, last int) (num int, inRange bool) {
	if n < 0 {
		num = last + 1 + n
		return num, num >= first
	}
	return n, n <= last && last >= first
}

// historyRemove takes entries i through j out of the list, inclusive, and
// takes the same number off the unwritten count — which is what `-d` does to
// it whichever entries it removed, and which never goes below nothing.
func historyRemove(r *interp.Runner, i, j int) {
	entries := historyEntries(r)
	times := historyTimesOf(r, len(entries))
	historySetList(r,
		append(entries[:i:i], entries[j+1:]...),
		append(times[:i:i], times[j+1:]...))
	historySetUnwritten(r, historyUnwrittenCount(r)-(j-i+1))
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
		entries := historyEntries(r)
		return historyWriteFile(r, name, entries, historyTimesOf(r, len(entries)), false, false)
	case 'a':
		if code := historyWriteFile(r, name, historyNewest(r), historyNewestTimes(r), true, false); code != 0 {
			return code
		}
		historySetUnwritten(r, 0)
		return 0
	case 'r':
		lines, ok := historyReadFile(r, name, false)
		if !ok {
			return 1
		}
		historyLoadLines(r, lines, true)
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
		historyLoadLines(r, lines[at:], true)
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

// historyTakeOwnLine removes the line the builtin was written on, which the
// front end reading the program has already recorded, and answers whether the
// builtin may go on to its own work.
//
// It may not where the list says the line is there and the list is empty:
// bash's `-p` and `-s` both stop rather than carry on against a list they
// could not take from — see the rows written down in historyBuiltin, where
// `history -s b` after the line has already gone stores nothing.
//
// take says the caller consumes the mark rather than merely reading it, which
// is `history -s` and not `history -p`: only the first `-s` on a line takes
// the line, while every `-p` on it takes an entry. Measured, not derived —
// the rows are in historyBuiltin.
func historyTakeOwnLine(r *interp.Runner, take bool) bool {
	if !historyHasOwnLine(r) {
		return true
	}
	entries := historyEntries(r)
	if len(entries) == 0 {
		return false
	}
	times := historyTimesOf(r, len(entries))
	historySetList(r, entries[:len(entries)-1], times[:len(times)-1])
	historySetUnwritten(r, historyUnwrittenCount(r)-1)
	if take {
		r.SetVar(historyOwnLine, "0")
	}
	return true
}

// historyDropOwnLine is the seam the core holds for `fc`, which reads the
// line rather than consuming it, exactly as `history -p` does.
func historyDropOwnLine(r *interp.Runner) {
	historyTakeOwnLine(r, false)
}

// historyHasOwnLine reports that the list holds the line the builtin now
// running is written on — the reader put it there and no HISTCONTROL or
// HISTIGNORE rule kept it out.
//
// Both halves matter and neither implies the other: a prompt records into the
// editor's history rather than into this list, so a planted entry would be
// dropped by a builtin whose own line nothing had pushed in front of it; and
// measured 2026-09-16 on bash 5.3.20 with `HISTIGNORE='history*'`, `history
// -p x` and `history -s z` are not recorded and the entries before them
// survive both calls.
func historyHasOwnLine(r *interp.Runner) bool {
	if !r.HistoryListFilledByTheReader() {
		return false
	}
	own, _ := r.GetVar(historyOwnLine)
	return own == "1"
}

// historyRecord is a command the reader hands over, which joins the list
// unless HISTCONTROL or HISTIGNORE leaves it out — and remembers whether it
// did, because that answer is what makes the builtin's own line droppable.
func historyRecord(r *interp.Runner, line string) {
	if historyWanted(r, line) {
		historyAdd(r, line)
		r.SetVar(historyOwnLine, "1")
		return
	}
	r.SetVar(historyOwnLine, "0")
}

// historyWanted asks whether an entry joins the list at all, and erases the
// earlier copies of it where it does. One question asked in one place,
// because **an entry offered to the list is an entry offered to the list
// however it arrived**.
//
// It arrives two ways: the reader hands over a command it has just run, and
// `history -s` is handed one nobody ran. Those were two answers until #4057,
// the newer route going straight to historyAdd — the shape this tree keeps
// rediscovering, a second helper missing the rule the first one carries.
//
// Measured on bash 5.3.20 with the reader silenced by `HISTIGNORE='history*'`
// set *before* `set -o history`, so that nothing the instrument itself runs
// is an entry — the trap #4031 fell into, and worth naming because the
// silencer is the feature under test:
//
//   - `HISTIGNORE='…:secret*'` then `history -s secretline`, `history -s
//     keepme` lists `1 keepme`, so the patterns reach `-s`; they are globs
//     matched against the whole entry, anchored — `HISTIGNORE=echo` drops
//     `echo` and keeps `echo one`;
//   - `HISTIGNORE='…:&'` over `-s x`, `-s x`, `-s y`, `-s y` lists `1 x`,
//     `2 y`, so `&` reads the entry the list already ends with, which for
//     `-s` is a well-defined thing even though no line was read;
//   - `HISTCONTROL=ignorespace` then `history -s " hidden"`, `history -s
//     kept` lists `1 kept`: a leading blank hides an operand exactly as it
//     hides a typed line, though `-s` is not a typed line;
//   - `HISTCONTROL=ignoredups` over `-s one`, `-s one`, `-s two` lists
//     `1 one`, `2 two`, and an unknown word in the value is simply not one
//     of the four — `bogus:ignoredups:alsobogus` still ignores dups;
//   - `HISTCONTROL=erasedups` over `-s a`, `-s b`, `-s a`, `-s c`, `-s a`
//     lists `1 b`, `2 c`, `3 a`: every earlier copy gone, and what survives
//     **renumbered from the front** rather than keeping the numbers it had.
//     Erasing is not HISTSIZE dropping entries off the front, and the
//     numbers say which happened: with `HISTSIZE=3` over `-s a`…`-s d` then
//     `-s b`, the list reads `2 c`, `3 d`, `4 b` — the one entry HISTSIZE
//     dropped still counts against the numbering and the copy erasedups
//     removed does not;
//   - and where both would reject, either alone is enough:
//     `HISTIGNORE='…:zap*'` with `HISTCONTROL=ignorespace` over `-s ' zapper'`,
//     `-s zapper`, `-s ' plain'`, `-s plain` lists `1 plain`.
//
// The duplicate rules read the entry the list already ends with, which for
// `-s` is what is left after the builtin's own line is dropped and not that
// line: measured, `history -s alpha`, `HISTCONTROL=ignoredups`, `history -s
// alpha` records the second `alpha`, because what stood before it was the
// assignment.
//
// What does **not** come through here is a file. Measured, `HISTIGNORE`
// matching a line of the file and `HISTCONTROL=ignoredups:erasedups` both
// leave `history -r` loading every line of it, so historyLoad sits beside
// this rather than under it: reading a file back is not the shell being
// offered a command.
func historyWanted(r *interp.Runner, line string) bool {
	entries := historyEntries(r)
	previous := ""
	if len(entries) > 0 {
		previous = entries[len(entries)-1]
	}
	if repl.HistoryIgnores(HistoryStyle(), r, line, previous) {
		return false
	}
	if historyErasesDups(r) {
		times := historyTimesOf(r, len(entries))
		kept, keptTimes := entries[:0], times[:0]
		for i, entry := range entries {
			if entry != line {
				kept = append(kept, entry)
				keptTimes = append(keptTimes, times[i])
			}
		}
		if len(kept) != len(entries) {
			historySetList(r, kept, keptTimes)
		}
	}
	return true
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
	historyAppend(r, line, historyTimeNow(r), true)
	historySetUnwritten(r, historyUnwrittenCount(r)+1)
}

// historyLoad is an entry read from a file, which is already written and so
// is never counted as unwritten. Whether it moves the *numbering* is the
// caller's to say — see historyLoadLines.
func historyLoad(r *interp.Runner, line, stamp string, numbered bool) {
	historyAppend(r, line, stamp, numbered)
}

// historyLoadLines is a file's physical lines becoming entries, which is not
// one for one: a `#<epoch>` line is a time bash wrote and not a command
// anybody typed, and it is left out of the list (#4013).
//
// The decoding is `repl.HistoryEntries` against this dialect's own
// HistoryStyle — the same call the session's reader makes, against the same
// style — rather than a rule spelled out here. The two readers were the thing
// to be careful about: a script's list and a prompt's come off one file, and
// a `#` rule that only one of them knew would be the next helper that omits
// the other's fix.
//
// Every route a file reaches the list by goes through here: `-r`, `-n`, and
// the read a script's first `set -o history` does. **They do not agree about
// the numbering**, which is what `numbered` carries: a line `-r` or `-n`
// pushes off a full list moves the numbers on exactly as an entry the script
// made does, and a line the startup read pushes off moves nothing.
//
// Measured 2026-09-21 on bash 5.3.20, `env -i`, a scratch HOME and HISTIGNORE
// keeping the reader's own lines out of the list:
//
//	HISTSIZE=2, `-s a`,`b`,`c`, then `-r` of x,y,z,w        6 z · 7 w
//	the same with `-n` (one more entry for the HISTFILE=)   7 z · 8 w
//	an empty list, `-r` of x,y,z,w into HISTSIZE=2          3 z · 4 w
//	that `-r` twice                                         7 z · 8 w
//	an unbounded list, `-r` of x,y,z,w                      1 x … 4 w
//	HISTSIZE=2 and a four-line HISTFILE read at startup     1 z · 2 w
//
// The last row is the one that pays for the flag. Four lines into a list held
// at two move the numbering by four when `-r` reads them and by nothing when
// the startup read does, so a single answer would be wrong for one of them
// (#4073). The row the comment above historyAppend was written from —
// `HISTSIZE=1` before a two-line HISTFILE reads `2 history` — is the startup
// read again, and the entry the reader adds afterwards is what moved it.
func historyLoadLines(r *interp.Runner, lines []string, numbered bool) {
	entries, times := repl.HistoryEntriesTimed(historyStyle(r), lines)
	for i, entry := range entries {
		historyLoad(r, entry, times[i], numbered)
	}
}

// historySeed is a previous session's lines joining the list, which is what
// an interactive front end hands over once, at the start.
//
// The same call `-r` makes and with the same `numbered` answer the startup
// read uses: these lines were read from a file, so nothing counts them as
// waiting to be written and the numbering does not move for the ones a size
// drops. No times come with them — the front end's decoder hands back the
// commands — so a listing under a format draws `??` for a line an earlier
// session wrote, which is what this shell knows about it.
func historySeed(r *interp.Runner, lines []string) {
	for _, line := range lines {
		historyLoad(r, line, "", false)
	}
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
//   - `HISTSIZE=1` before a two-line HISTFILE is read and then `history`
//     lists `2 history` — an entry read from a file moves nothing.
//
// **This is the half that moves the numbers on, and it is not the only
// route to a shorter list.** An assignment trims where it stands and numbers
// what it keeps from the count it dropped instead — see historyStifle, which
// is where the shape this used to carry (`HISTSIZE=2` over a longer list)
// was measured properly. The two are distinguishable: the same two entries
// reached this way keep their own numbers.
func historyAppend(r *interp.Runner, line, stamp string, numbered bool) {
	entries := historyEntries(r)
	times := historyTimesOf(r, len(entries))
	keep, bounded := historySize(r)
	dropped := historyDroppedCount(r)
	if bounded && len(entries) > keep {
		// Over-size with no assignment behind it, which is what is left once
		// the assignment trims: a `local HISTSIZE` whose restore shrinks the
		// size back as a function returns. bash trims at the restore and this
		// shell trims at the next entry (#4045), so the rule applied here is
		// the assignment's own — the count dropped, not a step.
		if numbered {
			dropped = len(entries) - keep - 1
		}
		entries, times = entries[len(entries)-keep:], times[len(times)-keep:]
	}
	entries, times = append(entries, line), append(times, stamp)
	if bounded && len(entries) > keep {
		if numbered {
			dropped += len(entries) - keep
		}
		entries, times = entries[len(entries)-keep:], times[len(times)-keep:]
	}
	historySetDropped(r, dropped)
	historySetList(r, entries, times)
}

// historySizeKind is what one HISTSIZE value says about the size of the list.
type historySizeKind int

const (
	// historySizeText is a value that is not a count at all, which says
	// nothing about the size: the one in force stays there.
	historySizeText historySizeKind = iota
	// historySizeLifted is a value that takes the cap off — an empty one or
	// a negative count.
	historySizeLifted
	// historySizeCount is a count of entries to keep.
	historySizeCount
)

// historyReadSize reads one HISTSIZE value the three ways bash reads it,
// measured on bash 5.3.20 over lists built with `history -s`.
//
// **Whitespace around the digits is not part of the value**, which is one
// question and not two: measured 2026-09-21 over three entries, `HISTSIZE=" 2
// "` and a leading tab each leave two, exactly as `HISTSIZE=2` does. So is a
// sign — `+2` keeps two and `-0` empties the list, where `-1` keeps
// everything.
//
// What is **not** a count is text after the digits or a value too large to
// hold: `2x`, `0x2` (bash does not read it as hex here) and
// `99999999999999999999`. None of those lifts the cap — see historySize.
//
// An **empty** value lifts it and a value that is only whitespace does not,
// which is the one place the trimming stops: measured 2026-09-21, `HISTSIZE=`
// over a list capped at two lets it grow, where `HISTSIZE=" "` leaves the cap
// of two exactly where it was. So the emptiness is tested before the trim and
// not after it.
func historyReadSize(value string) (int, historySizeKind) {
	if value == "" {
		return 0, historySizeLifted
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, historySizeText
	}
	if n < 0 {
		return 0, historySizeLifted
	}
	return n, historySizeCount
}

// historySize is the count of entries the list keeps, or false where it keeps
// them all. Measured on bash 5.3.20: `HISTSIZE=2` after three commands lists
// only the newest two, `HISTSIZE=0` keeps nothing — `history` then lists
// nothing at all, its own line included — and a negative value keeps
// everything.
//
// **It is not a reading of HISTSIZE.** A value that is not a count leaves the
// last size that *was* one in force, so the cap is state beside the list
// rather than something the variable can be asked for — `echo "[$HISTSIZE]"`
// answers `[abc]` while the list is still being held at two. Measured
// 2026-09-21 on bash 5.3.20, `env -i` with a scratch HOME and HISTIGNORE
// keeping the reader's own lines out of the way, after `HISTSIZE=2` and nine
// entries:
//
//	HISTSIZE=abc, then two more entries     10 j · 11 k — still two
//	HISTSIZE=2x, and 0x2, and an overflow   the same: the cap of two holds
//	HISTSIZE=abc then =2x, two more         the same again — it is not spent
//	HISTSIZE=-1, then two more              the list grows: the cap is off
//	HISTSIZE= (empty), and unset            the same, the cap is off
//	-1, then HISTSIZE=abc, two more         still off: a lift is remembered
//
// So the three ways of saying "no limit" are a **negative**, an **empty
// value** and **unset**, and "not a number at all" is not one of them: `abc`
// leaves the cap where it was and the numbering carries on unbroken, which is
// what says it is the earlier size still being enforced rather than a fresh
// default (#4054).
//
// Which size that is, before any assignment, is the value the list is turned
// on with: measured, `set -o history` with no HISTSIZE defaults it to 500 and
// `HISTSIZE=abc` afterwards holds the list at 500, where `unset HISTSIZE`
// first leaves it unbounded; and a HISTSIZE of 2 inherited from the
// environment is in force for a `HISTSIZE=abc` that never had an assignment
// of its own to remember. historyStartFile lays that first value down.
//
// One answer for both readers of the size, because they are one question:
// the cap the insert path enforces and the trim an assignment does — a
// `HISTSIZE=" 3 "` that trimmed but did not then bound the list, or the
// reverse, would be the same rule answered twice. The trim reaches this
// through historySizeAssigned, which has the value being written and so does
// not need to ask what is in force.
func historySize(r *interp.Runner) (int, bool) {
	value, ok := r.GetVar("HISTSIZE")
	if !ok {
		return 0, false
	}
	n, kind := historyReadSize(value)
	if kind != historySizeText {
		return n, kind == historySizeCount
	}
	// Not a count, so the variable says nothing and the size in force is
	// what the last count left. Absent only where nothing has set the size
	// and the list has never been turned on, which is a list with no cap.
	held, ok := r.GetVar(historyInForce)
	if !ok {
		return 0, false
	}
	n, kind = historyReadSize(held)
	return n, kind == historySizeCount
}

// historySizeAssigned is the HISTSIZE assignment seam: it records the size
// now in force and trims the list where a count was written.
//
// The three kinds part company here and nowhere else. A count is the new size
// and trims (historyStifle); a negative or empty value takes the cap off and
// is remembered as having done so, which is what makes a later `HISTSIZE=abc`
// leave the list unbounded rather than resurrect the count before it; and a
// value that is not a count changes nothing at all — it does not trim, which
// was already true here, and it does not lift the cap, which is #4054.
func historySizeAssigned(r *interp.Runner, value string) {
	n, kind := historyReadSize(value)
	switch kind {
	case historySizeText:
		return
	case historySizeLifted:
		r.SetVar(historyInForce, historyNoCap)
		return
	}
	r.SetVar(historyInForce, strconv.Itoa(n))
	historyStifle(r, n)
}

// historyStifle trims the list where HISTSIZE is assigned, which is a moment
// of its own rather than the cap historyAppend already keeps on the way in.
// Setting the size *before* a list is built needs nothing of this, which is
// why it is invisible to anything that sets HISTSIZE in an rc file (#4031).
//
// Measured 2026-09-21 on bash 5.3.20, `env -i` with a scratch HOME, over
// lists built with `history -s` and with HISTIGNORE keeping the reader's own
// lines out of the way. That last part is what made the rule legible: the
// line carrying the assignment is itself an entry, and its own insert moves
// the numbers again before a `history` on the next line can look.
//
//	nine entries, `HISTSIZE=5`               4 e · 5 f · 6 g · 7 h · 8 i
//	nine entries, `HISTSIZE=8`               1 b … 8 i
//	nine entries, `HISTSIZE=5` then `=2`     3 h · 4 i
//	three entries, `HISTSIZE=2`              1 b · 2 c
//	`HISTSIZE=3`, five entries, then `=2`    1 d · 2 e
//	two entries, `HISTSIZE=0`, `=9`, add c   2 c
//
// **The new numbering is absolute rather than a step**, and that is the half
// a trim that only drops entries gets wrong. The oldest entry the trim keeps
// is numbered by *how many it dropped*: dropping four of nine leaves the
// oldest at 4 and dropping seven leaves it at 7, whatever the numbers were
// beforehand — the fifth row above comes off a list already numbered 3, 4, 5
// and comes back numbered 1, 2. It is also what tells the two routes to a
// two-entry list apart, since the same pair reached by the insert path keeps
// the numbers it had.
//
// `HISTSIZE=0` is that rule at its limit rather than a case of its own — the
// list is emptied and the count still decides where the numbering resumes,
// which the last row shows. Worth pinning separately all the same, because an
// off-by-one in the trim reads as correct everywhere except zero.
//
// A list already **at** the new size, or under it, is left alone with its
// numbers untouched, and so is one whose HISTSIZE is not a count — which is
// why the count arrives as an argument rather than being read back: the
// caller is historySizeAssigned, which has already told the three kinds of
// value apart, and a value that is not a count never reaches here even
// though a cap is still in force for it (#4054).
//
// zsh trims on the assignment too and **keeps the numbers**: 5.9.2 with
// `print -s a`, `b`, `c` and `HISTSIZE=2` lists `2 b`, `3 c` where bash lists
// `1 b`, `2 c`. That is dialect/zsh's own rule now (#4043), stated where it
// was measured rather than as a field on repl.HistoryStyle that nothing would
// read; the session's own recall list is sized once when it starts and has no
// assignment to hear, so there is no second reader of this to keep in step.
//
// **ksh93 is not a third column of this, and the line that used to say so was
// wrong.** It does not trim: HISTSIZE narrows what `hist -l` shows and every
// entry is still there. Measured 2026-09-21 on ksh93u+ at `/bin/ksh` through
// `-i`, listing at each step, `true 1`, `true 2`, `true 3`, `HISTSIZE=2`,
// `hist -l`, `HISTSIZE=99`, `hist -l` — the first listing holds two and the
// second holds all seven, at their own numbers. The history is the file
// there, which two rows say: a HISTFILE in a directory that does not exist
// leaves ksh93 with no history at all, and after the assignment the file
// still holds every line. The numbers surviving, which the original reading
// leaned on, is a consequence of their being the file's offsets rather than a
// trimming rule that happens to keep them. #4092 carries the measurement.
func historyStifle(r *interp.Runner, keep int) {
	entries := historyEntries(r)
	if len(entries) <= keep {
		return
	}
	times := historyTimesOf(r, len(entries))
	historySetDropped(r, len(entries)-keep-1)
	historySetList(r, entries[len(entries)-keep:], times[len(times)-keep:])
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

// historyNewestTimes is the times of exactly those entries.
func historyNewestTimes(r *interp.Runner) []string {
	entries := historyEntries(r)
	times := historyTimesOf(r, len(entries))
	n := min(historyUnwrittenCount(r), len(entries))
	return times[len(times)-n:]
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
func historyWriteFile(r *interp.Runner, name string, entries, times []string, appendTo, quiet bool) int {
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
	// The encoder is `repl`'s, against this dialect's own HistoryStyle — the
	// mirror of the decoder historyLoadLines reads with, and the same call
	// the session's writer and `dialect/zsh`'s make. This dialect's file
	// states no continuation, so what it writes is an entry and a newline
	// exactly as it always was, and an entry holding a newline goes down as
	// two physical lines and reads back as two: that is the shell's own
	// answer and is measured, not a gap left here. Going through the one
	// encoder is what stops the next fact about the file from landing in one
	// writer and not the others (#4034).
	if _, err := io.WriteString(f, repl.HistoryTextTimed(historyStyle(r), entries, times)); err != nil {
		historyFileComplaint(r, quiet, name, err)
		return 1
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
