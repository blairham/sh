// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A command word that is exactly `-`, and the one dialect that has it.
//
// `- echo hi` prints `hi` at 0 in zsh 5.9.2 and is `command not found` at 127
// in bash 5.3.20, bash 3.2, ksh93u+, dash 0.5.12 and BusyBox ash — every
// column but one, so this is a dialect's answer rather than an axis the panel
// has two sides of. The dialect spells it by naming the word a precommand
// modifier, which is where the family lives: see PrecommandDash for what it
// does and for the rows it was measured from.
//
// It was read here as a word to **throw away** until #5018, and every probe
// that reached the command through `echo` agreed with that reading, because
// `echo hi` prints `hi` under either one. What tells them apart is a command
// that reports its own argv[0], and that is one line.
//
// It is **not** Semantics.LoneDashIsAnOption, which that shell also answers
// yes: that one is about a dash-word handed to a *builtin*, where `echo - x`
// prints `x` because the builtin's option reader ate the word. This one is
// about the word standing where the command name goes, and nothing has been
// looked up yet when it is asked.
//
// Why the two are worth keeping apart: the readings are hard to tell from the
// outside, and the wrong one survives a lot of probing. `eval - echo hi`
// prints `hi` in that shell, which reads as "`eval` ate the `-`" — but
// `eval "- echo hi"`, one argument, prints `hi` too, and `eval - -- echo hi`
// reports `--` as the command. Both are explained by the text `- echo hi`
// running with the modifier read off the front, and neither is explained by
// an option (#3236).

// dashed puts the modifier's dash on the name a command will find in argv[0].
//
// The name rather than the path, which is measured and is the whole of the
// difference between the two: `- sh -c '…'` hands over `-sh` and `- /bin/sh
// -c '…'` hands over `-/bin/sh`, so the dash goes on the word as written and
// not on what PATH resolved it to. It is spent here for the same reason the
// `-l` letter's copy of it is spent in Runner.execArgv — one dash, however
// many `-` words were written.
func (r *Runner) dashed(name string) string {
	if !r.dashPrecommand {
		return name
	}
	return "-" + name
}
