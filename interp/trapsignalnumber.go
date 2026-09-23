// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// TrapSignalNumber is the number of the condition whose trap body is running,
// and whether one is running at all.
//
// The second answer is the reason this is a pair rather than a number: the
// parameter one dialect publishes from it is *unset* outside a trap, which is
// a difference a script can see — `${p-word}` takes its default there and not
// inside. See Runner.SetDynamicPresence.
//
// The numbering is the host's signal table extended, and it is measured rather
// than assumed. bash 5.3.20 on macOS, whose table ends at 31, answers 0 for
// EXIT, 30 for a `kill -USR1`, and 32, 33 and 34 for DEBUG, ERR and RETURN —
// so the three conditions that are not signals are numbered from one past the
// last signal the host names, in that order. Derived from the table rather
// than written out, so a host with a longer one numbers them where that host's
// shell does instead of where this machine's does.
//
// A condition the table does not hold answers 0, which is EXIT's number too.
// That is the honest answer rather than a placeholder: there is no number for
// a name no signal has, and a dialect that publishes this has nothing else to
// say about one.
func (r *Runner) TrapSignalNumber() (number int, running bool) {
	if !r.inTrapBody {
		return 0, false
	}
	switch r.trapBodyCond {
	case "EXIT":
		return 0, true
	case "DEBUG":
		return lastKnownSignal() + 1, true
	case "ERR":
		return lastKnownSignal() + 2, true
	case "RETURN":
		return lastKnownSignal() + 3, true
	}
	if entry, ok := r.signalNamed(r.trapBodyCond); ok {
		return int(entry.Sig), true
	}
	return 0, true
}
