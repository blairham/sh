// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// The compatibility level, in its two spellings.
//
// `BASH_COMPAT` names a release this shell is asked to behave as, and
// `shopt -s compat44` names the same thing with a letter. They are one state
// and not two, which is the whole of this file: interp.Runner.compatLevel
// reads the parameter, every compatibility row asks it, and the letters have
// to reach it or a script that sets one is answered by a shell still at its
// own release.
//
// Measured 2026-09-22 on `/opt/homebrew/bin/bash` 5.3.20 under `LC_ALL=C`
// with standard input on /dev/null. Four facts, each of which shaped a line
// below:
//
//   - **The range is numeric and the complaint is the only thing out of range
//     costs.** `BASH_COMPAT=abc` writes `BASH_COMPAT: abc: compatibility value
//     out of range`, **stores the value anyway** — `echo "$BASH_COMPAT"` is
//     `abc` — and leaves the status at 0, so this is an assignment action and
//     not a refused store. `0`, `30`, `54`, `99`, `044`, `4.10`, `.44`, `44.`
//     and ` 44` are each out of range; `31` through `53` and `3.1` through
//     `5.3` are each a level. The ends are the oldest release with a
//     compatibility reading and the shell's **own** release, which is why the
//     ceiling is here and interp.CompatibilityLevel holds only the floor.
//   - **Every assignment spelling complains**, because every one of them is a
//     store: `export`, `declare`, `local`, `+=`, a command's prefix, and the
//     environment the shell was started with, which complains before the
//     first line and with no line number. An empty value is silent and takes
//     the level back to this shell's own.
//   - **`shopt -s compatNN` sets the level to NN**, and `BASH_COMPAT` reads
//     `44` afterwards — plain digits, whatever spelling was there before.
//   - **`shopt -u compatNN` writes the level back whether or not it moved.**
//     It resets to the default only when the level *is* NN: at 42, `shopt -u
//     compat44` leaves 42 — but it writes it, so `BASH_COMPAT=4.4; shopt -u
//     compat42` re-spells the parameter as `44`, and `shopt -u compat44` with
//     the parameter unset writes `53` where nothing had moved at all. A
//     model that wrote only on a change gets both of those wrong.
//
// The letters stop at `compat44` — 5.3.20 has no `shopt compat51`, and says
// `invalid shell option name` for it — so the letter door reaches only levels
// at or below 44 while the parameter reaches every level in the range. That
// asymmetry is bash's and not an omission here.
const compatVariable = "BASH_COMPAT"

// compatCeiling is the newest level this shell will take, which is its own
// release: being compatible with a release that has not happened is what
// `out of range` is about at the top end.
//
// Derived from the version the prelude hands `BASH_VERSION` rather than
// written out again, so a dialect that moves its release moves the range with
// it. interp.CompatibilityFloor is the other end.
const compatCeiling = major*10 + minor

// compatLevel is the level in force now: the parameter's, where it holds one
// this shell recognizes, and this shell's own release otherwise.
//
// "Otherwise" covers unset, empty and out of range alike, and that is
// measured rather than convenient — `BASH_COMPAT=abc; shopt compat44` reports
// `off`, which is the same answer an unset parameter gives.
func compatLevel(r *interp.Runner) int {
	value, ok := r.GetVar(compatVariable)
	if !ok {
		return compatCeiling
	}
	n, ok := interp.CompatibilityLevel(value)
	if !ok || n > compatCeiling {
		return compatCeiling
	}
	return n
}

// compatSwitch is one `shopt compatNN` letter, wired to the parameter at both
// ends: it reads `on` when the level is NN, and setting or unsetting it
// writes the level back into `BASH_COMPAT`.
//
// Written through interp.Runner.SetVar, which is the ordinary store, so the
// name keeps whatever it had: measured, `export BASH_COMPAT=44; shopt -u
// compat44` lists `declare -x BASH_COMPAT="53"` rather than losing the export.
func compatSwitch(level int) shoptSwitch {
	return shoptSwitch{
		get: func(r *interp.Runner) bool { return compatLevel(r) == level },
		set: func(r *interp.Runner, on bool) {
			now := compatLevel(r)
			switch {
			case on:
				now = level
			case now == level:
				// Unsetting the level in force takes this shell back to its
				// own release; unsetting any other letter moves nothing, and
				// still writes what is there.
				now = compatCeiling
			}
			r.SetVar(compatVariable, strconv.Itoa(now))
		},
	}
}

// registerCompatibilityLevel puts the out-of-range complaint on the
// parameter's assignment rather than on the first row that reads it.
//
// Which is where bash puts it, and the difference is visible: a script that
// sets a bad level and never reaches a compatibility row is complained about
// all the same, at its own line, and a script that sets one and then runs a
// hundred rows is complained about once.
func registerCompatibilityLevel(r *interp.Runner) {
	r.SetAssignmentAction(compatVariable, func(rr *interp.Runner, value string) {
		if value == "" {
			return
		}
		if n, ok := interp.CompatibilityLevel(value); ok && n <= compatCeiling {
			return
		}
		rr.Diagnosef("%s: %s: compatibility value out of range\n", compatVariable, value)
	})
}
