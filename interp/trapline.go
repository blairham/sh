// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a diagnostic from inside a trap's body says it happened.
//
// A trap's text is parsed on its own, so it has lines of its own, and the
// panel splits three ways on which lines to name:
//
//	trap "echo a
//	nosuchcmd" INT      on line 2 of a six-line script, fired from line 5
//
//	bash   line 2   the second line of the body
//	dash   2        the same
//	ksh93  line 6   the body counted from where it fired: 5 + 2 - 1
//	zsh    5        where it fired, and the body's own lines are not counted
//
// Measured with a two-line body on purpose. A one-line body cannot tell
// "the body's first line" from "wherever it fired", which is how this
// looked like one answer for as long as the probes were one-liners.

// TrapBodyLineStyle is which lines a trap body's diagnostics name.
type TrapBodyLineStyle int

const (
	// TrapBodyLineWithin counts from the body's own first line: bash, dash.
	// The zero value, and what this did before the question was one.
	TrapBodyLineWithin TrapBodyLineStyle = iota
	// TrapBodyLineOffsetFromWhereItFired adds where it fired to the line
	// within the body, so a body written on one line reports the firing
	// line and its second line reports one past it: ksh93.
	TrapBodyLineOffsetFromWhereItFired
	// TrapBodyLineWhereItFired names where it fired and nothing else, so
	// every line of the body reports the same number: zsh.
	TrapBodyLineWhereItFired
)

func (t TrapBodyLineStyle) String() string {
	switch t {
	case TrapBodyLineOffsetFromWhereItFired:
		return "TrapBodyLineOffsetFromWhereItFired"
	case TrapBodyLineWhereItFired:
		return "TrapBodyLineWhereItFired"
	}
	return "TrapBodyLineWithin"
}

// firedAt is the line a trap counts as having fired on.
//
// For a signal trap it is wherever the shell had got to. The EXIT trap has
// no such line, and the two dialects that ask answer differently: ksh93
// counts it as the first line, which makes its body read like a script of
// its own, and zsh counts it as the line after the script's last, which is
// where the shell has got to once the script is done.
func (r *Runner) firedAt() int {
	if !r.inExitTrap {
		return r.line
	}
	if r.ask(r.sem().ExitTrapFiresPastTheEnd, "where the EXIT trap counts as having fired") {
		return r.programEnd
	}
	return 1
}

// trapLocationName is what a diagnostic's location calls the trap body it
// was read out of, in the dialect that names one.
//
// Measured 2026-09-17 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`
// over a script file, with a body that will not parse set for each condition
// in turn and with a refused substitution inside a body that does:
//
//	EXIT                 exit trap
//	ERR                  error trap
//	DEBUG                debug trap
//	RETURN               return trap
//	INT                  interrupt trap
//	TERM HUP QUIT USR1 USR2 ALRM    trap
//
// So `INT` is the one *signal* with a word of its own and every other signal
// falls back to the builtin's name — which is measured rather than assumed
// from the four pseudo-conditions having one each.
//
// One reader for both messages that write it. A trap body's own parse failure
// and a substitution refused inside a body that parsed are two sentences
// about the same place, and they were two switches: the first knew EXIT and
// nothing else, so ERR, DEBUG, RETURN and INT wrote `trap` there while the
// second wrote nothing at all.
func trapLocationName(cond string) string {
	switch cond {
	case "EXIT":
		return "exit trap"
	case "ERR":
		return "error trap"
	case "DEBUG":
		return "debug trap"
	case "RETURN":
		return "return trap"
	case "INT":
		return "interrupt trap"
	}
	return "trap"
}

// bodyLineStyle is which of the two questions this body is: the one every
// trap asks, or the one the two conditions that fire at a command ask.
//
// Read from a flag the firing site sets rather than from the condition's
// name, because the name is not in scope here — one entry point runs every
// body, which is what keeps the line rules in one place.
func (r *Runner) bodyLineStyle() TrapBodyLineStyle {
	if r.inCommandTrap {
		return r.sem().CommandTrapBodyLine
	}
	return r.sem().TrapBodyLine
}

// enterTrapBody sets the lines a trap body's diagnostics will name and the
// depth its traces are drawn at, and returns what puts them back.
//
// cond is the condition this body belongs to, which only the depth rule
// reads: EXIT is the one trap whose body is not a level of indirection.
func (r *Runner) enterTrapBody(cond string) func() {
	base, pin, command, inTrap := r.lineBase, r.linePin, r.inCommandTrap, r.inTrapBody
	trapCond := r.trapBodyCond
	text := r.runText
	indirection := r.indirection
	// A trap body is text read again, and the one dialect that counts levels
	// of that counts this one — with EXIT the single exception. Measured on
	// bash 5.3.15, 2026-09-14, `env -i PATH=/usr/bin:/bin` over `-c`, each
	// probe traced with the default `PS4`:
	//
	//	d(){ :; }; trap d DEBUG; set -x; echo one     ++ d   ++ :   + echo one
	//	trap "echo T" ERR; set -x; false              + false       ++ echo T
	//	trap "echo T" INT; set -x; kill -INT $$       + kill …      ++ echo T
	//	set -T; trap "echo T" RETURN; set -x; f       + f … ++ echo T
	//	trap "echo T" EXIT; set -x; echo one          + echo one    + echo T
	//
	// So DEBUG, ERR, RETURN and every signal add a level and EXIT adds none,
	// and an `eval` inside a body adds a second on top — `trap "eval :" USR1`
	// traces `++ eval :` and then `+++ :`. The shell's own commands are
	// unaffected: `echo one` above is `+ ` in the same run, which is what
	// says this is the body's depth and not a mode the trap turns on.
	//
	// A function call is still not a level — `++ d` and `++ :` are the same
	// depth — which is Runner.indirection's own rule and the reason this is
	// counted here rather than wherever a body pushes a frame (#2781).
	if cond != "EXIT" {
		r.indirection++
	}
	// The line the shell had reached is part of what a body borrows and has
	// to give back. A trap body is a script of its own, so running it walks
	// `r.line` through the body's lines — and through the lines of anything
	// the body calls — which leaves the *next* reader of `$LINENO` reading
	// the trap's last line instead of its own.
	//
	// DEBUG is where that is certain rather than likely: it fires before the
	// command it traces, so the next reader is that very command. Measured on
	// bash 5.3.15, the traced command reports its own line; this engine
	// reported the last line the body ran.
	line := r.line
	restore := func() {
		r.lineBase, r.linePin, r.inCommandTrap, r.inTrapBody = base, pin, command, inTrap
		r.trapBodyCond = trapCond
		r.runText = text
		r.line, r.indirection = line, indirection
	}
	switch r.bodyLineStyle() {
	case TrapBodyLineOffsetFromWhereItFired:
		r.lineBase = r.firedAt() - 1
	case TrapBodyLineWhereItFired:
		r.linePin = r.firedAt()
	}
	// Cleared for the run of the body itself. The flag says which condition
	// *this* body belongs to, and a trap that fires while it runs is a
	// question of its own — without this, a signal delivered inside a DEBUG
	// body would be numbered by the rule the DEBUG body was given.
	r.inCommandTrap = false
	// And set for it, which is a different question to the one above: the
	// record of what the shell is running belongs to the script, so nothing a
	// body runs may move it — see Runner.recordRunning.
	r.inTrapBody = true
	// And which condition it is, which the location of a refusal read out of
	// the body names. Beside inTrapBody rather than derived from
	// inCommandTrap, because that flag groups three conditions the naming
	// keeps apart.
	r.trapBodyCond = cond
	return restore
}

// TrapListingSequence is the order a bare `trap` listing prints in.
type TrapListingSequence int

const (
	// TrapListingLowestFirst is EXIT and then the signals in ascending
	// numeric order: bash 5.3, bash-as-`sh`, bash 3.2, zsh, dash and
	// BusyBox ash. The zero value, because it is six of the seven.
	TrapListingLowestFirst TrapListingSequence = iota
	// TrapListingHighestFirst is the same set descending, which puts EXIT
	// last: ksh93 alone.
	TrapListingHighestFirst
)

func (t TrapListingSequence) String() string {
	if t == TrapListingHighestFirst {
		return "TrapListingHighestFirst"
	}
	return "TrapListingLowestFirst"
}
