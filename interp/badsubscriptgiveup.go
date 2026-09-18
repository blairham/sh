// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// badSubscriptGivesUp writes a complaint about a subscript a builtin could not
// evaluate and then gives up exactly as much as the dialect gives up.
//
// One door for the three sites that ask, because they are the same failure
// seen from three builtins and the three outcomes have to mean the same thing
// at all of them: see BadSubscriptPolicy for what each one is and where it was
// measured. Before this, `unset` asked a bool that read "fatal" and the store
// asked nothing at all and was fatal unconditionally — so bash ended a script
// it carries on with, and ksh93 ended one it does not stop at all (#3485) —
// and a *declaration* reached Runner.fatal directly, so bash ended a script it
// runs to the end there too (#3495).
//
// The status is 1 wherever the shell is still running to see it, which is
// unanimous: the complaint leaves a failed builtin behind, and the next
// command reads 1 whether it got there by the builtin returning or by the
// abandoned command's line ending. The two give-up branches take the dialect's
// own fatal status instead, which is the same 1 everywhere it can be observed
// — only dash and BusyBox ash answer that axis differently and neither reaches
// here.
func (r *Runner) badSubscriptGivesUp(p BadSubscriptPolicy, what, sentence string) int {
	if p == BadSubscriptUnspecified {
		// The refusal alone, and not the sentence under it: a shell that was
		// never told what to do here has not decided to complain about the
		// subscript, it has failed to answer a question. Status 2 and the
		// unspecified flag are what every other refused axis leaves, and the
		// caller reads the flag to keep from reporting twice.
		r.diagf("%s\n", r.unanswered(what))
		r.status, r.unspecified = 2, true
		return 2
	}
	r.diagf("%s\n", sentence)
	switch p {
	case BadSubscriptEndsTheScript:
		r.fatalQuiet()
		return r.status
	case BadSubscriptAbandonsTheCommand:
		// The same door the sites that *expand* a subscript go through, `-c`
		// rule and all: see Runner.giveUpForABadSubscript, which is where
		// the rule this branch used to spell out by hand now lives. It is
		// what takes the rest of the line with it, so `unset 'q[b c]'; echo
		// x` prints no x and the next *line* runs.
		r.giveUpForABadSubscript()
		return r.status
	}
	return 1
}

// badSubscriptToADeclaration is the third site: a declaration whose operand
// names an element and whose subscript will not evaluate.
//
// `declare 'a[b c]'=v`, and `typeset`, `local`, `readonly` and `export` in
// whichever dialects let those take a subscript at all. It reached
// Runner.fatal until now, which is zsh's and ksh93's answer written on the
// common path: bash gives up the command it is running and carries on at the
// next top-level one, so a script bash runs to the end stopped here. See
// Semantics.BadSubscriptToADeclaration for the rows and for why this is a
// field of its own rather than either neighbor read a third time.
//
// The sentence is the language's and names no builtin, which is bash's and
// zsh's wording and is what the two sites next door already write. ksh93 puts
// the builtin in front of it and reports from the builtin's own location; that
// is #3496, which owns all three sites at once and is wording rather than
// unwinding.
//
// Nothing comes back, as nothing comes back from declareElement: the status is
// written here, and assignFailed is what keeps the builtin's own return value
// from zeroing it — the same pair changeCompoundKind's abandoning branch sets,
// one refusal over.
func (r *Runner) badSubscriptToADeclaration(sub string, err error) {
	r.status = r.badSubscriptGivesUp(r.sem().BadSubscriptToADeclaration,
		"how much a declaration gives up for an operand's unevaluable subscript",
		r.subscriptFailure(sub, err))
	r.assignFailed = true
}

// valuelessSubscriptedOperand is what a declaration does with an operand that
// names an element and carries no value, and it reports whether the operand is
// **finished** — so a caller that gets false carries on declaring the name
// exactly as it always did.
//
// One door for all four spellings — `typeset`/`declare`, `local`, `readonly`
// and `export` — because the answer is about the declaration and not about
// which word spells it, and because a second copy that omitted a case is the
// failure this repository keeps making.
//
// See Semantics.ValuelessSubscriptedOperand for the three answers and where
// each was measured. Before this the brackets were never read in any dialect:
// `typeset 'a[b c]'` was silent at 0 where zsh and ksh93 both end the script,
// and in the zsh column the operand reached the valueless-declaration listing
// and printed the whole array on its way past (#3501).
func (r *Runner) valuelessSubscriptedOperand(base string, subs []string, f declareFlags, shadows bool) bool {
	sub := subs[len(subs)-1]
	switch r.sem().ValuelessSubscriptedOperand {
	case ValuelessSubscriptedOperandDeclaresTheName:
		return false
	case ValuelessSubscriptedOperandReadsTheSubscript:
		if r.assocDeclared(base) {
			// A table's brackets hold a key rather than an expression, in
			// every column: measured 2026-09-17, `typeset -A m; typeset
			// 'm[b c]'` is silent at 0 in bash and puts the key in with an
			// empty value in zsh and ksh93 alike, with no arithmetic
			// anywhere. So this column writes the key too, which is the one
			// place its answer and the element-writing one coincide — and
			// the reason the constant's name is about the *subscript* rather
			// than about never writing anything.
			r.setAssocElem(base, sub, "")
			return true
		}
		if _, err := r.subscriptValue(sub); err != nil {
			r.badSubscriptToADeclaration(sub, err)
			return true
		}
		return false
	case ValuelessSubscriptedOperandWritesTheElement:
		// The empty-value form under another spelling, which is measured
		// rather than inferred — so it goes through the one path a value
		// goes through, refusals, table keys, array growth and all.
		r.declareElement(base, subs[:len(subs)-1], sub, "", f, shadows)
		return true
	}
	r.diagf("%s\n", r.unanswered(
		"what a declaration does with a subscripted operand that carries no value"))
	r.status, r.unspecified = 2, true
	return true
}
