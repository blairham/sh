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

// enterTrapBody sets the lines a trap body's diagnostics will name, and
// returns what puts them back.
func (r *Runner) enterTrapBody() func() {
	base, pin, command := r.lineBase, r.linePin, r.inCommandTrap
	restore := func() { r.lineBase, r.linePin, r.inCommandTrap = base, pin, command }
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
	return restore
}
