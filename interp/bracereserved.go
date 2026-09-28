// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a bare `{` and a bare `}` are reserved words wherever they stand is
// a question about how text is **lexed**, so it lives on syntax.Dialect and
// not on Semantics — and one dialect spells it as an option a running script
// switches, which is why a runner has to be able to move it.
//
// That is the same combination [Runner.SetCasePatternListReadAsOneWord],
// [Runner.SetBarePatternGroups] and [Runner.SetShortFormBodyIsOneCommandOrNone]
// have, and the same consequences follow: the answer that decides is the one
// in force when the text is parsed, the dialect is copied and replaced rather
// than written through, and the front end's run loop reads the rest of the
// program with whatever it finds.
//
// # Two readings and one setter
//
// The opening half — `{print A}` being a group rather than a word — and the
// closing half — `}` ending the word it ends — are two fields of
// syntax.Dialect, and the option that moves them moves both. They are written
// through one setter because a caller has to read the pair to know what the
// grammar becomes, and two setters would take two copies of the dialect to
// arrive at one answer.
//
// They are **not** one field, because a second name in the same dialect's
// option namespace takes the closing half alone. Measured against
// `/opt/homebrew/bin/zsh` — `zsh 5.9.2 (aarch64-apple-darwin25.4.0)` — run
// `-f` over a script file under `set -n`, 2026-09-27, each option moved on the
// line after `emulate zsh`:
//
//	                 plain   ignorebraces   ignoreclosebraces
//	{ echo A }       parses  refused        refused
//	echo A}          refused parses         parses
//	{print A; }      parses  refused        parses
//
// So the third row is the one that parts them: the closing name leaves the
// opening reading alone.

// BraceReservedWordReadings reports the two readings a bare brace has: whether
// a `{` where a command may begin is the reserved word however the text runs
// on after it, and whether a `}` is reserved wherever a word may stand rather
// than only where a command may begin.
func (r *Runner) BraceReservedWordReadings() (open, closing bool) {
	// One axis per lang call: the value it hands back is shared and carries
	// no adjustment, so a caller may read a field off it and nothing else.
	// See TestNothingReadsAnAdjustedAxisOffLang.
	return r.lang().OpenBraceNeedsNoBlank, r.lang().CloseBraceAlwaysReserved
}

// SetBraceReservedWordReadings moves them, for a dialect whose option
// namespace has a name for the pair.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
func (r *Runner) SetBraceReservedWordReadings(open, closing bool) {
	d := r.dialect()
	if d.OpenBraceNeedsNoBlank == open && d.CloseBraceAlwaysReserved == closing {
		return
	}
	d.OpenBraceNeedsNoBlank = open
	d.CloseBraceAlwaysReserved = closing
	r.Dialect = &d
}
