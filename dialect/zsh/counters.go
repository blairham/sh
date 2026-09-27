// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// The three counters this shell keeps and names, and had no parameter for.
//
// Split out of #4866's ledger as #4904: the group whose value is **state the
// engine would have to keep**, which is what separates them from the twelve
// constants #4912 supplied and from the identity values beside them. All
// three answered `${+NAME}` of 0, so `print "cmd $HISTCMD"` wrote a line with
// a gap in it and `(( ZSH_SUBSHELL ))` read an empty word.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `-f` from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME — and again in a
// digest-pinned `debian:bookworm-slim` against that distribution's zsh, which
// answers all three identically:
//
//	name          ${(t)}                     fresh value
//	HISTCMD       integer-readonly-special    0
//	ZSH_SUBSHELL  integer-readonly-special    0
//	TTYIDLE       integer-readonly-special   -1
//
// All three are **frozen produced** names, so the seam is the one `$ARGC`,
// `$LINENO`, `$PPID` and `$status` use: [interp.Runner.SetDynamic] plus
// [interp.Runner.MarkReadonly] plus a declaration carrying the integer
// letter, base ten and `Silent`. Measured in the same run, all four listing
// forms agree with those four names — a bare `readonly` and a bare `typeset`
// write the row, `typeset -p NAME` and `readonly -p` write nothing at 0.
//
// # `$ZSH_SUBSHELL` is the one the apparatus can measure wrongly
//
// A probe that reads it inside `$( … )` answers **1**, because the command
// substitution *is* a subshell, and a probe at the top level answers **0**.
// Both are correct and only the second is the startup value, so a sweep that
// wrapped every cell in a substitution — the obvious way to keep going past a
// name that might refuse — would record the wrong number for this name and
// for nothing else in the sweep. Every figure above was taken at the top
// level of a script file with no substitution around it.
//
// # And the pipeline is where the count parts from a boundary
//
// Measured rather than assumed, because this engine's answer could have
// differed from the shell's for a reason nothing else would surface: the
// reference runs the **last** stage of a pipeline in the shell itself, so
// `true | print $ZSH_SUBSHELL` is **0** while `print $ZSH_SUBSHELL | cat` is
// **1**. This engine clones for a stage that is not the last and runs the
// last one on the shell's own runner, so [interp.Runner.SubshellDepth]
// already answers both cells the way the reference does and there is nothing
// here to correct. That agreement is a finding and is in the test, not a
// property to assume of the next thing that clones.
func registerTheCounters(r *interp.Runner) {
	// The current history event — the same number `%h` draws, from the same
	// place, so the prompt escape and the parameter cannot come to disagree.
	// Zero with nothing in the list, which is what a script sees.
	r.SetDynamic("HISTCMD", func(rr *interp.Runner) string {
		return strconv.Itoa(fcCurrentEvent(rr))
	})
	// How many subshell boundaries lie between here and the shell that was
	// started. Produced rather than stored for the reason `$ARGC` is: a
	// number written once would be right until the first `( … )` and quietly
	// wrong after it, and a stale depth still reads as a number.
	r.SetDynamic("ZSH_SUBSHELL", func(rr *interp.Runner) string {
		return strconv.Itoa(rr.SubshellDepth())
	})
	// Seconds since the terminal was last read from, and **-1 where there is
	// no terminal** — which is the value a script in a pipeline sees and the
	// one every case in this tree sees, since nothing here has a controlling
	// terminal. The word for the absent case is the dialect's: the core hands
	// back a duration and a second result saying whether there was one to
	// measure, because zero is a real answer.
	r.SetDynamic("TTYIDLE", func(rr *interp.Runner) string {
		idle, held := rr.TerminalIdle()
		if !held {
			return "-1"
		}
		return strconv.Itoa(int(idle.Seconds()))
	})
	for _, name := range [...]string{"HISTCMD", "ZSH_SUBSHELL", "TTYIDLE"} {
		r.MarkReadonly(name)
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
	}
}
