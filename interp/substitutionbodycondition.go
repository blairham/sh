// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a `$( … )` body whose read stopped at the **closing parenthesis**
// with an `if` short of its `then` is left to the moment it runs is a question
// about how a line is *read*, so it lives on syntax.Dialect and not on
// Semantics — and one dialect spells it as an option a running script
// switches, which is why a runner has to be able to move it.
//
// That is the combination [Runner.SetDoubledQuoteInSingleQuotes] and the two
// beside it have, and the same consequences follow: the answer that decides is
// the one in force when the text is lexed, the dialect is copied and replaced
// rather than written through, and the front end's run loop reads the rest of
// the program with whatever it finds.

// SubstitutionBodyRefusesAnUnfinishedCondition reports whether this runner
// settles the read where a body's read stopped at the closing parenthesis
// with an `if` or `elif` still short of its `then`.
func (r *Runner) SubstitutionBodyRefusesAnUnfinishedCondition() bool {
	return r.lang().SubstitutionBodyRefusesAnUnfinishedCondition
}

// SetSubstitutionBodyRefusesAnUnfinishedCondition moves it, for a dialect whose
// option namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
func (r *Runner) SetSubstitutionBodyRefusesAnUnfinishedCondition(on bool) {
	// Asked of the dialect in place, so a request for the state it is
	// already in copies nothing. See Runner.lang.
	if r.lang().SubstitutionBodyRefusesAnUnfinishedCondition == on {
		return
	}
	d := r.dialect()
	d.SubstitutionBodyRefusesAnUnfinishedCondition = on
	r.Dialect = &d
}
