// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// `$EPOCHSECONDS` and `$EPOCHREALTIME`: this shell's own clock parameters,
// which it has had since 5.0 and which are not zsh's under another name.
//
// The distinction is the whole reason this is a file rather than two lines
// beside `RANDOM`. `zsh/datetime` supplies parameters spelled the same way
// (dialect/zsh/datetime.go), and copying that registration across would have
// been wrong on every row but the name — measured 2026-09-12 with `env -i`
// and a scratch HOME, over a script file, against bash 5.3.15 and zsh 5.9.2:
//
//	probe                                  bash 5.3        zsh 5.9.2
//	`echo ${#EPOCHREALTIME#*.}` places      6               10
//	`EPOCHSECONDS=5; echo $EPOCHSECONDS`    the clock, st 0 fatal: read-only
//	`declare -p EPOCHSECONDS`               declare -- …="1789252077"
//	                                                        typeset -ir, no value
//	`$epochtime`, `strftime`                absent          present
//
// So: six places rather than ten, assignable-but-ignored rather than
// readonly, and listed as an ordinary scalar carrying its value rather than
// as a hidden integer carrying none. zsh's are `MarkReadonly` **and**
// `MarkHidden`; neither belongs here, and a mark either way is what the two
// tests in epoch_test.go watch for.
//
// There is no `$epochtime` and no `strftime` builtin, because this shell has
// neither: its formatting is `printf '%(fmt)T'`, which the core already has.
//
// **The divergence this file recorded is closed, and it was never this
// parameter's.** Real bash's `unset EPOCHSECONDS` removes the parameter for
// good — a later `EPOCHSECONDS=7` makes an ordinary variable holding `7`, and
// the clock never comes back — where the producer used to return on the next
// assignment. Measured the same day, `unset RANDOM; RANDOM=9; echo $RANDOM
// $RANDOM` answers `9 9` in bash 5.3, bash 3.2, ksh93 and BusyBox ash and two
// different numbers in zsh 5.9.2, so `RANDOM` and `SECONDS` had carried the
// same divergence since long before this file. It is one axis over every
// produced parameter rather than a special case here, which is why it was
// filed rather than patched in behind one parameter's back:
// Semantics.AssignmentRestoresAnUnsetProducedParameter (#2450).
//
// bash 3.2.57 has neither parameter, which is a version fact and not an axis:
// the panel holds both builds and this dialect models 5.3.

// registerEpochClock installs the two parameters.
//
// Produced rather than stored, for the reason `SECONDS` is: a clock read once
// is wrong from the instant afterwards, and silently — the caller still gets
// a number.
func registerEpochClock(r *interp.Runner) {
	r.SetDynamic("EPOCHSECONDS", func(rr *interp.Runner) string {
		return strconv.FormatInt(rr.Now().Unix(), 10)
	})
	r.SetDynamic("EPOCHREALTIME", func(rr *interp.Runner) string {
		t := rr.Now()
		return strconv.FormatInt(t.Unix(), 10) + "." +
			epochFraction(t.Nanosecond(), epochRealtimeDigits)
	})
	// An assignment is taken and thrown away, which is a writer that does
	// nothing rather than the absence of one. Without it the assignment is
	// still ignored — the producer answers ahead of `Assigned` — but the
	// silence would be an accident of the read path instead of this shell's
	// measured answer, and [interp.Runner.SetDynamicWriter] asks for the
	// writer precisely so that the two cannot be told apart by reading the
	// code. Marking them readonly instead would be zsh's answer: `st=0` and
	// no diagnostic is what bash gives, against zsh's fatal `read-only
	// variable: EPOCHSECONDS`.
	for _, name := range []string{"EPOCHSECONDS", "EPOCHREALTIME"} {
		r.SetDynamicWriter(name, func(*interp.Runner, string) {})
		// And listed as an ordinary scalar carrying its value —
		// `declare -- EPOCHSECONDS="1789252077"`, measured — where zsh's
		// pair of the same name list as `typeset -ir` with no value at all.
		// The row in the table above was unreachable until a produced
		// parameter could say how it lists (#2451).
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{})
	}
}

// epochRealtimeDigits is how many places `$EPOCHREALTIME` carries here: six,
// which is a microsecond, against zsh's ten.
const epochRealtimeDigits = 6

// epochFraction is the leading width digits of a nanosecond count, truncated
// rather than rounded.
//
// Truncated because the parameter is a clock read and not a measurement being
// reported: rounding 999999999ns up would carry into a second the integer
// half does not have, so `$EPOCHREALTIME` could read one second ahead of
// `$EPOCHSECONDS` taken in the same breath. Real bash reads the two halves of
// one `gettimeofday` and cannot disagree with itself; truncating is how this
// keeps that property.
//
// Built by hand rather than through FormatFloat: a float64 of a nanosecond
// epoch has no bits left for the sixth place, so `%.6f` of it is rounding
// noise in the last digit or two. zsh's ten places are that noise and are
// kept there because zsh prints it; six places here are exact.
func epochFraction(nsec, width int) string {
	div := 1
	for i := width; i < 9; i++ {
		div *= 10
	}
	s := strconv.Itoa(nsec / div)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
