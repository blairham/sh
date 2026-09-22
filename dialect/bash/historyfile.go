// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The history file of a **script**: read the first time the list is turned
// on, and appended to when the shell ends.
//
// Measured 2026-09-16 on bash 5.3.20, from script files run with no terminal,
// no startup files and a scratch HOME. None of it is what a person at a prompt
// sees — repl keeps that file — and all of it was reachable from a script
// that says `set -o history`, where this shell read nothing and wrote nothing:
//
//	HISTFILE=f          # f holds `alpha one` and `beta two`
//	set -o history
//	history             # 1 alpha one / 2 beta two / 3 history
//
// and f ends the run with `history` appended to it.

// historySizeDefault is the value HISTSIZE takes when the list is first turned
// on and the script has not set it. Measured: `echo $HISTSIZE $HISTFILESIZE`
// is two empty words before `set -o history` and `500 500` after it, and a
// HISTSIZE the script set is what HISTFILESIZE takes instead — `HISTSIZE=7`
// gives `7 7`, where `HISTFILESIZE=7` alone gives `500 7`.
const historySizeDefault = "500"

// historyStartFile runs the first time a script turns the list on.
//
// Once per shell, which interp decides: a second `set -o history`, or one
// after `set +o history`, reads nothing and puts back no size the script
// unset in between. And a first one with no HISTFILE to read is still the
// first — naming the file afterwards and toggling the option reads nothing.
//
// The file's lines join the list as entries this session did not make, so
// neither `-a` nor the shell's ending writes them back; and they count as read
// for `-n`, which measured reads nothing from the file straight afterwards.
// A HISTFILESIZE that is a count keeps only the newest that many lines —
// `HISTFILESIZE=2` over a three-line file lists the second and third.
//
// A file that is not there is not a complaint: measured, a HISTFILE naming a
// missing file, or a file in a missing directory, reads nothing and says
// nothing.
func historyStartFile(r *interp.Runner) {
	size, sizeSet := r.GetVar("HISTSIZE")
	if !sizeSet {
		size = historySizeDefault
		r.SetVar("HISTSIZE", size)
	}
	if _, ok := r.GetVar("HISTFILESIZE"); !ok {
		r.SetVar("HISTFILESIZE", size)
	}
	// And the first size in force, which is not the same as the variable and
	// cannot be read back off it once a value that is not a count has been
	// written over it. Only where nothing has set it yet: an assignment the
	// script made before turning the list on has already said what the size
	// is, and measured 2026-09-21, `HISTSIZE=2; HISTSIZE=abc; set -o history`
	// holds the list at two rather than letting it grow. What this lays down
	// is the other two first values — the default above, so that
	// `HISTSIZE=abc` after `set -o history` holds the list at 500, and a
	// HISTSIZE inherited from the environment, which no assignment saw. See
	// historySize (#4054).
	if _, ok := r.GetVar(historyInForce); !ok {
		if n, kind := historyReadSize(size); kind == historySizeCount {
			r.SetVar(historyInForce, strconv.Itoa(n))
		} else {
			r.SetVar(historyInForce, historyNoCap)
		}
	}
	name, ok := r.GetVar("HISTFILE")
	if !ok || name == "" {
		return
	}
	lines, ok := historyReadFile(r, name, true)
	if !ok {
		return
	}
	read := len(lines)
	if keep, ok := historyFileSize(r); ok && keep < len(lines) {
		lines = lines[len(lines)-keep:]
	}
	// Not numbered: the startup read is the one route into the list that
	// moves the numbering by nothing, however many lines the cap drops. See
	// historyLoadLines, where the six rows are.
	historyLoadLines(r, lines, false)
	r.SetAssocElement(historyReadAt, shellPath(r, name), strconv.Itoa(read))
}

// historyFinishFile runs as a script whose list is still on ends, and writes
// what `history -a` would: the entries this session made and no `-a` wrote.
//
// To the HISTFILE of the moment the shell ends, not the one the list was read
// from — measured, changing it part way through writes the new file — and not
// at all where it is unset or empty. interp keeps this from running in a
// subshell's ending, after a fatal signal, or with the list turned off again;
// `exec` never reaches it. A file that cannot be written is not a complaint
// either: a HISTFILE in a missing directory ends the run silently.
//
// # It appends, except where unwritten entries were lost
//
// Normally an append, and that is what leaves an earlier `history -a`'s lines
// and another shell's lines alone. But once the list has been made **shorter
// than the count of entries waiting to be written** — which is what a
// HISTSIZE trim does, and what a `local HISTSIZE` restore does (#4045) — the
// ending stops appending and puts the list down as the whole file.
//
// Measured 2026-09-21 on bash 5.3.20, `env -i`, a scratch HOME, HISTIGNORE
// keeping the reader's own lines out, and the file shown as it stands when
// the shell ends. `p q r` is a file the session never read; `-a` after `a`
// writes it:
//
//	p q r, -s a, -a, -s b, -s c                        p q r a b c
//	p q r, -s a, -a, -s b, -s c, HISTSIZE=2            p q r a b c
//	p q r, -s a, -a, -s b, -s c, HISTSIZE=1            c
//	p q r, -s a, -s b, -s c, HISTSIZE=1                c
//	p q r read at startup, -s a,b,c, HISTSIZE=1        c
//	p q r read at startup, -s a,b,c, HISTSIZE=4        p q r a b c
//	p q r, -s a, -a, -s b, -s c, HISTSIZE=1, -a        p q r a c
//	p q r, -s a, -a, -s b, -s c, HISTSIZE=1, -s d      e — the list again
//	p q r, -s a, -s b, HISTSIZE=0                      empty
//	p q r, -s a, -a, HISTSIZE=0                        p q r a
//	p q r, HISTSIZE=0 with no entries                  p q r
//
// Row three is the one that names the rule, and rows one and two are the
// controls: a trim that drops only entries an `-a` had already written leaves
// the ending appending, and a trim that drops an **unwritten** one does not.
// Rows five and six say the same of a file the session read at startup, and
// row five is what rules a truncation out — `p q r` are lines the list still
// held a moment earlier and they are gone, so the ending wrote the list
// rather than keeping the file's newest lines.
//
// Row seven says the whole-file write belongs to the **ending** and not to
// the loss: a `history -a` straight after the trim appends the one entry the
// list still has and clears the count, and the ending then has nothing to do.
// Row eight is the same rule after two more entries. Rows nine to eleven are
// `HISTSIZE=0` at its limit — an empty list still writes, which empties the
// file, unless nothing was waiting to be written at all.
//
// The condition is the unwritten count exceeding the list, which is exactly
// the loss: entries are written oldest first, so a count larger than the list
// can supply is a count that was still counting something the list dropped.
func historyFinishFile(r *interp.Runner) {
	name, ok := r.GetVar("HISTFILE")
	if !ok || name == "" {
		return
	}
	entries := historyNewest(r)
	whole := historyUnwrittenCount(r) > len(historyEntries(r))
	if len(entries) == 0 && !whole {
		// Nothing to append, and the ending then does nothing at all:
		// measured 2026-09-18, `HISTFILE=missing; set -o history` leaves no
		// file behind, where an unconditional append had created an empty
		// one. The truncation below goes with it — see historyTruncateFile,
		// where a three-line file over HISTFILESIZE=1 keeps its three lines
		// when the shell ends with nothing to write.
		return
	}
	if historyWriteFile(r, name, entries, historyNewestTimes(r), !whole, true) == 0 {
		historySetUnwritten(r, 0)
		historyTruncateFile(r)
	}
}

// historyTruncateFile keeps only the newest HISTFILESIZE lines of the file
// HISTFILE names, which is the second thing a script's history file meets and
// was missing entirely (#3423).
//
// Two moments, measured 2026-09-18 on bash 5.3.20 from script files with a
// three-line file and no terminal, one shape at a time:
//
//	HISTFILESIZE=1                              the file is `HISTFILESIZE=1`
//	HISTFILESIZE=2                              `gamma`, `HISTFILESIZE=2`
//	HISTFILESIZE=0                              empty
//	HISTFILESIZE=1; history -c                  `gamma`
//	HISTFILESIZE=1; history -a; history -c      `gamma` and the two entries
//	HISTFILESIZE=1; history -w; history -c      all five, untouched
//	HISTFILESIZE=1; history -a; echo x          `echo x`
//	HISTFILESIZE=1; HISTFILESIZE=10             `gamma` and the two entries
//	HISTFILESIZE=abc, and -1                    nothing is truncated
//
// Two rules account for every row and neither alone accounts for any of them.
// **An assignment truncates where it stands** — `HISTFILESIZE=1; history -c`
// leaves `gamma` with no write anywhere near it, which only an assignment can
// have done — and **the ending truncates after it has appended**, but only
// when it appended: the `-a` row ends three lines over a size of one, and the
// `-c` row would be one line if the ending truncated unconditionally.
//
// `-w` is the row that says the two are the whole of it. It writes the list
// and truncates nothing, and it does not mark what it wrote as written
// either — which is why `HISTFILESIZE=1; history -w` ends at one line (the
// ending appended the same entries again and then truncated) while the same
// pair with a `history -c` after it ends at five.
//
// A value that is not a count truncates nothing, which historyFileSize
// already answers for the read at startup.
func historyTruncateFile(r *interp.Runner) {
	keep, ok := historyFileSize(r)
	if !ok {
		return
	}
	name, ok := r.GetVar("HISTFILE")
	if !ok || name == "" {
		return
	}
	lines, ok := historyReadFile(r, name, true)
	if !ok {
		return
	}
	// By entries rather than by lines where the file carries a time on a line
	// of its own — see repl.HistoryFileTail, where both readings were
	// measured against the shell.
	kept := repl.HistoryFileTail(historyStyle(r), lines, keep)
	if len(kept) == len(lines) {
		return
	}
	// The file's own physical lines, put back as they stand: a `#<epoch>`
	// line among them is one of the file's lines and is written as itself,
	// not re-derived from the list. So no times are handed over here — an
	// entry's time is already in the text.
	_ = historyWriteFile(r, name, kept, nil, false, true)
}

// historyFileSize is HISTFILESIZE as a count of lines to keep, or false where
// it is not one — measured, `-1` and `abc` both keep everything.
//
// **One reading for both sizes.** HISTSIZE and HISTFILESIZE are the same
// question asked of two things and bash reads their values the same way, so
// the value arrives through historyReadSize rather than through a second
// `strconv.Atoi` beside it. That second copy was the bug: the whitespace
// historyReadSize learned to ignore (#4060) was still part of the value here,
// and the two rows that differed were exactly the two it had gained (#4074).
//
// Measured 2026-09-21 on bash 5.3.20, from a script file with no terminal,
// `env -i`, a scratch HOME and a three-line HISTFILE, as the lines the file
// holds when the shell ends:
//
//	" 2 ", a leading tab, +2                  two — the digits are the value
//	-0                                        none
//	2x, 0x2, 99999999999999999999             five: not a count, and nothing
//	-1, abc                                   five, the same way
//
// Where the two part company is **retention**, which is why the kinds are
// folded here rather than handed on. A HISTSIZE that is not a count leaves
// the last size that was one in force (see historySize); a HISTFILESIZE that
// is not a count truncates nothing and leaves nothing behind. Measured beside
// the rows above: `HISTFILESIZE=3` over a three-line file, then
// `HISTFILESIZE=abc`, then three commands ends with every line on disk, where
// the same script without the `abc` ends at three. So a lifted value and a
// value that is not a count both mean "truncate nothing" here, and neither is
// remembered.
func historyFileSize(r *interp.Runner) (int, bool) {
	value, ok := r.GetVar("HISTFILESIZE")
	if !ok {
		return 0, false
	}
	n, kind := historyReadSize(value)
	return n, kind == historySizeCount
}
