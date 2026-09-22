// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"
	"time"

	"github.com/blairham/sh/interp"
)

// When each line ran, which bash keeps beside the list and HISTTIMEFORMAT is
// the whole of the interface to.
//
// Measured 2026-09-22 on bash 5.3.20, from script files with no terminal,
// `env -i` and a scratch HOME, one shape at a time. Three separate rules came
// out of it and none of them is the others:
//
//   - **Recording is on while the variable is set, and latches on once it
//     has been.** An entry made before HISTTIMEFORMAT was ever set carries no
//     time; one made after does. `unset HISTTIMEFORMAT` does not put it back
//     — `HISTTIMEFORMAT=x; unset HISTTIMEFORMAT; history -s a` still times
//     `a`, and the same script without the first assignment does not. A value
//     the shell **inherited** counts as set, which is the half a seam on the
//     assignment cannot see: a `HISTTIMEFORMAT= shell` writes a timed file
//     without ever assigning it.
//   - **Reading a header takes its value**, with the variable set or unset. A
//     file of `#<epoch>` lines read with HISTTIMEFORMAT unset and listed with
//     it set shows the file's own times.
//   - **Writing asks the variable now.** `history -w` puts a `#<epoch>` line
//     in front of each entry that has one when HISTTIMEFORMAT is set at that
//     moment, and writes bare lines when it is not — whatever the entries
//     carry and however they were made.
//
// An entry with no time is not an error anywhere: it is written with no
// header, and a listing draws `??` where the time would have gone.
const (
	// historyTimes is the times, one per entry of historyStore and in the
	// same order — epoch seconds as the file spells them, and empty for an
	// entry with none.
	//
	// A second array rather than a field on the entry, because the list is a
	// shell array in the dialect's own namespace and every reader of it wants
	// the commands. The pairing is kept by [historySetList], which is the one
	// place either array is written, so an operation that drops entries
	// cannot drop the wrong times.
	historyTimes = ".bash.history.times"
	// historyRecordsTimes is the latch: set once HISTTIMEFORMAT has been
	// assigned, and never cleared. See the rule above, where the `unset` row
	// is what makes it a latch rather than a reading of the variable.
	historyRecordsTimes = ".bash.history.recordstimes"
	// historyNoTime is what a listing draws for an entry that has none. Two
	// question marks, measured: `HISTTIMEFORMAT='[%s] ' history` over an
	// untimed entry writes `??` and then the command, with no separator of
	// its own.
	historyNoTime = "??"
)

// historyTimesOf is the times beside the list, padded to the list's length so
// that a caller can index it with an entry's index whatever state the two
// arrays were left in.
func historyTimesOf(r *interp.Runner, entries int) []string {
	times, _ := r.GetArray(historyTimes)
	out := make([]string, entries)
	copy(out, times)
	return out
}

// historySetList puts the list and its times down together.
//
// The one writer of either array. Every operation on the list — an append, a
// trim from the front, a `-d` splice, an `erasedups` filter — is an operation
// on both, and doing it in two places is how the times would come to describe
// the wrong commands.
func historySetList(r *interp.Runner, entries, times []string) {
	r.SetArray(historyStore, entries)
	r.SetArray(historyTimes, times[:min(len(times), len(entries))])
}

// historyTimeNow is the time an entry joining the list takes, or empty where
// this shell is not recording them.
func historyTimeNow(r *interp.Runner) string {
	if !historyRecording(r) {
		return ""
	}
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// historyRecording reports whether an entry joining the list takes a time:
// the variable is set now, or it has been set at some point since the shell
// started.
func historyRecording(r *interp.Runner) bool {
	if _, ok := r.GetVar("HISTTIMEFORMAT"); ok {
		return true
	}
	v, ok := r.GetVar(historyRecordsTimes)
	return ok && v == "1"
}

// historyTimesAssigned is the HISTTIMEFORMAT assignment seam, and it only
// ever turns the latch on. See the rules above: the `unset` does not reach
// here and would not undo it if it did.
func historyTimesAssigned(r *interp.Runner) { r.SetVar(historyRecordsTimes, "1") }

// historyTimeFormat is HISTTIMEFORMAT where it is set, which is what decides
// whether a listing carries a time at all and what a written file looks like.
func historyTimeFormat(r *interp.Runner) (string, bool) { return r.GetVar("HISTTIMEFORMAT") }

// historyDrawnTime is what a listing puts in front of one entry: the format
// against that entry's own time, or `??` where it has none.
//
// The format is strftime's and is taken as it stands — measured, a format
// with no conversion in it is written literally. An **empty** one draws
// nothing at all, and that is the whole of it: an untimed entry under an
// empty format gets no `??` either, measured beside the same entry under `X`,
// which does. So the empty string is how a script asks for the spanning read
// of a history file without changing what a listing looks like.
func historyDrawnTime(format, stamp string) string {
	if format == "" {
		return ""
	}
	if stamp == "" {
		return historyNoTime
	}
	secs, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return historyNoTime
	}
	return interp.Strftime(format, time.Unix(secs, 0))
}
