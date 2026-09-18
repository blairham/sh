// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A command word that is exactly `-`, and the one dialect that throws it away.
//
// `- echo hi` prints `hi` at 0 in zsh 5.9.2 and is `command not found` at 127
// in bash 5.3.20, bash 3.2, ksh93u+, dash 0.5.12 and BusyBox ash — every
// column but one, so this is a dialect's answer rather than an axis the panel
// has two sides of. See Semantics.LoneDashInCommandPositionIsDiscarded.
//
// It is **not** Semantics.LoneDashIsAnOption, which that shell also answers
// yes: that one is about a dash-word handed to a *builtin*, where `echo - x`
// prints `x` because the builtin's option reader ate the word. This one is
// about the word standing where the command name goes, and nothing has been
// looked up yet when it is asked.
//
// Why it is worth writing down: the two readings are hard to tell apart from
// the outside, and the wrong one survives a lot of probing. `eval - echo hi`
// prints `hi` in that shell, which reads as "`eval` ate the `-`" — but
// `eval "- echo hi"`, one argument, prints `hi` too, and `eval - -- echo hi`
// reports `--` as the command. Both are explained by the text `- echo hi`
// running with the `-` discarded, and neither is explained by an option
// (#3236).

// discardLoneDashCommandWords takes the leading `-` words off a command,
// reporting whether it took any.
//
// Measured 2026-09-18 on zsh 5.9.2 from script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and no startup files:
//
//   - echo hi           hi, 0
//   - - - echo hi       hi, 0          — as many as are written
//     '-' echo hi         hi, 0          — quoting does not protect it
//     v=- ; $v echo hi    hi, 0          — nor does arriving by expansion
//     -- echo three       command not found: --, 127
//   - v=1 echo hi       command not found: v=1, 127
//   - ;                 0
//     v=1 -;              0, and `v` is 1 afterwards
//   - false             1
//
// Three of those are what make it a discard rather than a modifier. `--` is
// an ordinary command name, so it is the word being exactly one dash and not
// a leading one. The word after it is a **command word** and not a prefix, so
// `- v=1 echo hi` looks `v=1` up as a command. And with nothing left, what is
// left is the assignments, which persist exactly as they do for `v=1` written
// on its own.
//
// The quoted and expanded rows are why this is asked after expansion: it is
// the word the command was going to run, not the text that was typed.
func (r *Runner) discardLoneDashCommandWords(argv *[]string) bool {
	if len(*argv) == 0 || (*argv)[0] != "-" {
		// The common path, and it is a string comparison rather than an
		// axis read: a shell whose commands are not named `-` never asks
		// the dialect anything.
		return false
	}
	if !r.ask(r.sem().LoneDashInCommandPositionIsDiscarded,
		"a command word that is exactly `-`") {
		return false
	}
	words := *argv
	for len(words) > 0 && words[0] == "-" {
		words = words[1:]
	}
	*argv = words
	return true
}
