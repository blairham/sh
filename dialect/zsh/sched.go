// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/interp"
)

// The `sched` builtin: a command line put aside until a time.
//
// Measured 2026-09-07 against zsh 5.9.2 in the oracle environment, with a
// scratch HOME and no startup files, and through a pseudo-terminal for the
// half only a session has — when an elapsed entry actually runs.
//
// It arrived with `zle` because one plugin manager's scheduler wants both, and
// it is otherwise unrelated to the line editor: the table is a table of
// commands and a clock, and nothing about it touches a key. What it does share
// with `zle` is the shape of the split — **the moment is repl's and the table
// is this file's.** A shell can only notice that a time has passed at a
// boundary between commands, a script has no such boundary, and a prompt is
// made of them; so repl gained one seam, repl.Shell.RunScheduled, which is
// called before every prompt and is told nothing and hands back nothing.
// Everything a shell says about it — that it is spelled `sched`, that `+5` is
// five seconds, that the listing is numbered from 1 — is here.
//
// ## The time specifier, which is three grammars and not one
//
//   - **`+N` is N seconds.** Measured at 17:47:39: `+5` schedules 17:47:44 and
//     `+100` schedules 17:49:19. This is the one worth stating plainly,
//     because the manual's `hh:mm` invites reading a bare number as minutes,
//     and a scheduler that fires a hundred times later than it was asked to is
//     a scheduler nobody can see is broken.
//   - **`+H:MM[:SS]` is that much time from now.** `+0:05` is five minutes,
//     `+1:00` is an hour, `+1:00:30` is an hour and half a minute.
//   - **`H:MM[:SS]` without the plus is a time of day**, and it is arithmetic
//     from *today's midnight* rather than a clock face: `99:99` at 17:47 on
//     Monday 7 September schedules Friday 11 September at 4:39:00, which is
//     midnight plus 99 hours plus 99 minutes exactly. A time already past
//     rolls forward one day — `0:00` schedules tomorrow's — and one still to
//     come does not: `23:59` on Monday evening stays on Monday.
//   - **A bare number with no colon and no plus is seconds since the epoch.**
//     `sched 5 x` schedules 1970-01-01 00:00:05 UTC, which the listing writes
//     in local time. Recorded rather than admired; it costs one branch and a
//     shell that refused it would be stricter than the one being modeled.
//   - `bogus`, `now` and `+5x` are all `bad time specifier` at status 1.
//
// ## The table, and what the listing writes
//
//   - **`sched` with no arguments lists**, and an empty table prints nothing at
//     all at status 0.
//   - **Sorted by time, and numbered from 1 in that order.** Scheduling `+5`
//     then `+3` lists the `+3` entry as number 1, so the number is a position
//     in the sorted listing rather than an identity — which is what makes
//     `sched -1` mean "the next one".
//   - **The line is `%3d ` then the time then the command**: `  1 Mon Sep  7
//     17:47:23 echo hi`, and ` 10 ` at two digits. The time is the day name,
//     the month, the day of the month space-padded to two, and then the clock
//     with its *hour* space-padded to two as well — ` 4:39:00`, not
//     `04:39:00`.
//   - **The command is the words, joined by one space**, and the quoting that
//     produced them is gone: `sched +5 echo one     three` lists `echo one
//     three` because the shell split the line before `sched` saw it, while
//     `sched +5 "echo   a"` lists `echo   a` because that was one word. And a
//     first word starting with `-` is written after a `--`, so the line reads
//     back as what it was: `sched +5 -x` lists `-- -x`.
//   - **`sched -N` deletes entry N**, at status 0, and takes the rest of the
//     line with it — `sched -1 +5 echo hi` deletes and schedules nothing. A
//     number past the end of the table is `not that many entries` at 1, which
//     is also what `sched -1` says with an empty table; `sched -0` is `usage
//     for delete: sched -<item#>.`
//   - `sched +5` with no command is `not enough arguments`, and `sched -x` is
//     `bad option: -x` — this builtin's wording, which is `bindkey`'s and
//     `zle`'s rather than `zstyle`'s.
//
// ## When an entry runs, and the one difference this shell has
//
// Measured through a pseudo-terminal: `sched +2` at an idle prompt fires about
// two seconds later **with nobody typing**, and the entry is gone from the
// table afterwards. So zsh's is not a check at the next prompt — it is a
// timeout on the read that waits for a key.
//
// **Here it runs at the next prompt.** For a person who is typing that is the
// same moment or near enough; for an idle terminal it is later, and for an
// idle terminal that is never touched again it is never. That is a real
// difference and it is written down rather than papered over: firing on time
// needs this shell's read loop to wait on more than the terminal, which is the
// same seam `zle -F` needs and the same reason both are named here instead of
// half-built. What is not different is the order and the discipline — before
// the prompt hook, behind the panic guard, and with the status put back
// afterwards, all of which repl already does for a hook.

// schedStore is the table: a flat array of pairs, the time as seconds since
// the epoch and then the command line.
//
// In the Runner's own tables under a name no script can spell, the way
// bindkey.go and zle.go keep theirs, which is also what gives a subshell its
// own copy.
const schedStore = ".zsh.sched"

// registerSched installs the builtin and the parameter beside it, which are
// the two features `zmodload -lF zsh/sched` names in the real shell.
func registerSched(r *interp.Runner) {
	r.Register("sched", schedBuiltin)
	// `$zsh_scheduled_events` is the same table the listing walks, in the
	// same order, one element per entry as `<epoch>:<options>:<command>` —
	// measured, with the middle field empty for every spelling of `sched`
	// that can be written here, since the options it would hold are `-o` on
	// an entry and this builtin has no `-o`.
	r.SetDynamicArray("zsh_scheduled_events", func(rr *interp.Runner) []string {
		entries := readSchedule(rr)
		out := make([]string, 0, len(entries))
		for _, entry := range entries {
			out = append(out, strconv.FormatInt(entry.at.Unix(), 10)+"::"+entry.command)
		}
		return out
	})
	// Readonly and hidden, which is what `${(t)zsh_scheduled_events}` says:
	// `array-readonly-hide-hideval-special`. Readonly is the half that is
	// not decoration — a produced parameter a script can assign to is
	// shadowed by the assignment from then on, so it would stop tracking the
	// table and never say so, which is the reason datetime.go gives for the
	// same call.
	r.MarkReadonly("zsh_scheduled_events")
	hideModuleParameter(r, "zsh_scheduled_events")
}

// schedEntry is one row of the table.
type schedEntry struct {
	at      time.Time
	command string
}

func schedBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		return listSchedule(r)
	}
	if item, delete := schedDeletion(args[0]); delete {
		return deleteScheduled(r, args[0], item)
	}
	if strings.HasPrefix(args[0], "-") {
		r.Diagnosef("bad option: -%c\n", []rune(args[0][1:] + " ")[0])
		return 1
	}
	if args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			r.Diagnosef("not enough arguments\n")
			return 1
		}
	}
	at, ok := schedTime(r.Now(), args[0])
	if !ok {
		r.Diagnosef("bad time specifier\n")
		return 1
	}
	if len(args) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	writeSchedule(r, append(readSchedule(r), schedEntry{at: at, command: schedCommand(args[1:])}))
	return 0
}

// schedDeletion reads `-N`, the one spelling of this builtin that is an option
// rather than a time. `-0` is a usage complaint of its own and not a bad
// option, measured.
func schedDeletion(arg string) (int, bool) {
	if !strings.HasPrefix(arg, "-") || len(arg) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(arg[1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

func deleteScheduled(r *interp.Runner, arg string, item int) int {
	if item < 1 {
		r.Diagnosef("usage for delete: sched -<item#>.\n")
		return 1
	}
	entries := readSchedule(r)
	if item > len(entries) {
		r.Diagnosef("not that many entries\n")
		return 1
	}
	writeSchedule(r, append(entries[:item-1:item-1], entries[item:]...))
	return 0
}

// schedCommand is the words joined by one space, with a `--` in front where
// the first of them would otherwise read as an option. Measured.
func schedCommand(words []string) string {
	line := strings.Join(words, " ")
	if strings.HasPrefix(words[0], "-") {
		return "-- " + line
	}
	return line
}

// schedTime reads the three grammars in the file comment, and reports whether
// what it was given was one of them.
func schedTime(now time.Time, spec string) (time.Time, bool) {
	relative := strings.HasPrefix(spec, "+")
	digits := strings.TrimPrefix(spec, "+")
	if digits == "" {
		return time.Time{}, false
	}
	if !strings.Contains(digits, ":") {
		n, err := strconv.Atoi(digits)
		if err != nil {
			return time.Time{}, false
		}
		if relative {
			// `+N` is N seconds from now.
			return now.Add(time.Duration(n) * time.Second), true
		}
		// And a bare number is seconds since the epoch.
		return time.Unix(int64(n), 0), true
	}
	span, ok := schedSpan(digits)
	if !ok {
		return time.Time{}, false
	}
	if relative {
		return now.Add(span), true
	}
	// A time of day is arithmetic from today's midnight, which is what lets
	// `99:99` mean four days out; one already past rolls forward a day.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := midnight.Add(span)
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}

// schedSpan reads `H:MM` or `H:MM:SS` as a duration, without folding either
// field into the next: 99 hours and 99 minutes is four days and change, and
// that is the answer zsh gives.
func schedSpan(digits string) (time.Duration, bool) {
	fields := strings.Split(digits, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return 0, false
	}
	units := []time.Duration{time.Hour, time.Minute, time.Second}
	var span time.Duration
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return 0, false
		}
		span += time.Duration(n) * units[i]
	}
	return span, true
}

// listSchedule is `sched` with no arguments.
//
// The times are written in the zone of the shell's own clock rather than the
// process's, which is the same rule Runner.Now is there for: an embedder that
// pins the clock pins what a listing says, and a test that pins one does not
// then read the machine's zone back out.
func listSchedule(r *interp.Runner) int {
	zone := r.Now().Location()
	for i, entry := range readSchedule(r) {
		_, _ = fmt.Fprintf(r.Out(), "%3d %s %s\n", i+1, schedClock(entry.at.In(zone)), entry.command)
	}
	return 0
}

// schedClock is the listing's time: the day name, the month, the day of the
// month and the clock, with both the day and the *hour* space-padded to two.
//
// Written out rather than handed to a layout string, because the hour is the
// one field Go's reference time cannot ask to be space-padded.
func schedClock(at time.Time) string {
	return fmt.Sprintf("%s %s %2d %2d:%02d:%02d",
		at.Format("Mon"), at.Format("Jan"), at.Day(), at.Hour(), at.Minute(), at.Second())
}

// RunScheduled runs every entry whose time has passed, oldest first, and takes
// each out of the table before it runs.
//
// The dialect's answer to driver.Shell.RunScheduled. Taken out first, and that
// is the order rather than a tidy-up: a scheduled command that schedules
// another one — which is exactly what a plugin manager's scheduler does, since
// that is how it keeps a poll going — must not have its successor removed by
// the same pass that ran it.
//
// Through the `eval` builtin, because a table row is a command *line* and
// turning a line into something that runs is the core's job and not this
// file's.
func RunScheduled(r *interp.Runner, ctx context.Context) {
	eval, ok := r.Builtin("eval")
	if !ok {
		return
	}
	now := r.Now()
	for {
		entries := readSchedule(r)
		if len(entries) == 0 || entries[0].at.After(now) {
			return
		}
		due := entries[0]
		writeSchedule(r, entries[1:])
		eval(r, ctx, []string{due.command})
		if r.Exited() {
			return
		}
	}
}

// readSchedule is the table, sorted by time, which is the order everything
// about this builtin is expressed in — the listing's numbering and which entry
// runs next alike.
func readSchedule(r *interp.Runner) []schedEntry {
	flat, _ := r.GetArray(schedStore)
	out := make([]schedEntry, 0, len(flat)/2)
	for i := 0; i+2 <= len(flat); i += 2 {
		at, err := strconv.ParseInt(flat[i], 10, 64)
		if err != nil {
			continue
		}
		out = append(out, schedEntry{at: time.Unix(at, 0), command: flat[i+1]})
	}
	// Stable, so two entries at the same second keep the order they were
	// scheduled in and the numbers a listing gave them do not shuffle.
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out
}

func writeSchedule(r *interp.Runner, entries []schedEntry) {
	flat := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		flat = append(flat, strconv.FormatInt(entry.at.Unix(), 10), entry.command)
	}
	r.SetArray(schedStore, flat)
}
