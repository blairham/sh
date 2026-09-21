// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
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
	historyLoadLines(r, lines)
	r.SetAssocElement(historyReadAt, shellPath(r, name), strconv.Itoa(read))
}

// historyFinishFile runs as a script whose list is still on ends, and appends
// what `history -a` would: the entries this session made and no `-a` wrote.
//
// To the HISTFILE of the moment the shell ends, not the one the list was read
// from — measured, changing it part way through writes the new file — and not
// at all where it is unset or empty. interp keeps this from running in a
// subshell's ending, after a fatal signal, or with the list turned off again;
// `exec` never reaches it. A file that cannot be written is not a complaint
// either: a HISTFILE in a missing directory ends the run silently.
func historyFinishFile(r *interp.Runner) {
	name, ok := r.GetVar("HISTFILE")
	if !ok || name == "" {
		return
	}
	entries := historyNewest(r)
	if len(entries) == 0 {
		// Nothing to append, and the ending then does nothing at all:
		// measured 2026-09-18, `HISTFILE=missing; set -o history` leaves no
		// file behind, where an unconditional append had created an empty
		// one. The truncation below goes with it — see historyTruncateFile,
		// where a three-line file over HISTFILESIZE=1 keeps its three lines
		// when the shell ends with nothing to write.
		return
	}
	if historyWriteFile(r, name, entries, true, true) == 0 {
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
	if !ok || len(lines) <= keep {
		return
	}
	_ = historyWriteFile(r, name, lines[len(lines)-keep:], false, true)
}

// historyFileSize is HISTFILESIZE as a count of lines to keep, or false where
// it is not one — measured, `-1` and `abc` both keep everything.
func historyFileSize(r *interp.Runner) (int, bool) {
	value, ok := r.GetVar("HISTFILESIZE")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
