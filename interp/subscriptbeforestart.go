// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strconv"

// subscriptBeforeTheFirstElement answers a read whose negative subscript
// counted back past the array's first element.
//
// The reach is the same one NegativeSubscriptPastTheStartInserts answers on
// the left of `=`, seen from the right of it, and the panel does not answer
// the two alike: the column that refuses a *store* here may still let the
// *read* stand, so the two are separate fields rather than one read twice.
//
// One door for every read route, which is the point of putting it under
// Runner.elemAt rather than at each expansion: `${a[-4]}`, `${#a[-4]}`,
// `${a[-4]+set}`, `${a[-4]:-d}`, `$(( a[-4] ))` and `[[ -v a[-4] ]]` are one
// complaint each in ksh93 and in bash, and a rule stated at one spelling is a
// rule the other five contradict — which is how this family has grown wrong
// before.
//
// `end` is one past the array's highest subscript, so it is zero for a name
// holding no element at all: an unset name, a name holding a plain string, and
// an array that has been emptied. Semantics.SubscriptBeforeTheFirstElementNeedsAnElement
// is what decides whether such a name has an end to count back from.
func (r *Runner) subscriptBeforeTheFirstElement(name string, n, end int, length subscriptLength) {
	p := r.sem().SubscriptBeforeTheFirstElementRead
	if p == SubscriptBeforeStartIsNothing {
		// The whole of the silent answer, and the fast path: nothing is
		// asked about the name, because nothing depends on it.
		return
	}
	if end == 0 && r.ask(r.sem().SubscriptBeforeTheFirstElementNeedsAnElement,
		"a name holding no element having no end to count back from") {
		return
	}
	if r.unspecified {
		return
	}
	if length.is && r.refusesTheLengthBeforeTheFirstElement(length.written) {
		return
	}
	if r.unspecified {
		return
	}
	sentence := Wording(r.diag().SubscriptBeforeTheFirstElementRead,
		"%[1]s: bad array subscript", name, strconv.Itoa(n))
	switch p {
	case SubscriptBeforeStartIsReported:
		// The complaint and nothing else. The status is deliberately left
		// where it was: measured, bash's `a=(x y z); echo "[${a[-4]}]"; echo
		// after` writes the complaint, then `[]` and `after`, and exits 0 —
		// so the expansion is empty and the shell has not failed.
		r.diagf("%s\n", sentence)
	case SubscriptBeforeStartEndsTheScript:
		// The same door the store's refusal goes through, so the two agree
		// about how far a fatal subscript unwinds in a dialect that has one.
		r.failedSubscript("%s\n", sentence)
	default:
		r.diagf("%s\n", r.unanswered(
			"a negative subscript counting back past the first element"))
		r.status, r.unspecified = 2, true
	}
}

// subscriptLength is what a length route has to say about a subscript that
// reached past the first element, and what a read route has not.
//
// Two facts rather than the node, because the three call sites that count a
// position are not all looking at one: `$(( a[-4] ))`, a nameref's target and
// a redirection's operand each reach elemAt with no `${#…}` anywhere in
// sight, and a zero value is exactly what they mean.
type subscriptLength struct {
	// is says the expansion was `${#a[-4]}` rather than `${a[-4]}`.
	is bool
	// written is the subscript as the script wrote it, which is the whole of
	// the wording's subject in the column that refuses this: `[-4]`, with its
	// brackets and without the name.
	written string
}

// refusesTheLengthBeforeTheFirstElement is `${#a[-4]}` where the dialect
// answers the length differently from the read, and reports whether it did.
//
// The subject is the subscript **as it was written**, brackets included and
// with no name in front of it, where the read one line up names the array —
// which is the same split, in the same column, that
// Semantics.EmptyAssociativeKeyRefusesTheLength and
// Diagnostics.EmptyAssociativeKeyLength already record for `${#m[$w]}` under
// an empty key. That pair next door is the model rather than a new shape.
//
// And it abandons the word rather than standing beside an empty value:
// measured 2026-09-18 on bash 5.3.20, `a=(x y z); echo "[${#a[-4]}]"; echo
// after` writes the complaint, then `after`, and never the `[…]` line at all,
// where the read one line up writes `[]` and carries on.
func (r *Runner) refusesTheLengthBeforeTheFirstElement(written string) bool {
	if !r.ask(r.sem().SubscriptBeforeTheFirstElementRefusesTheLength,
		"`${#a[-4]}`, the length of an element a negative subscript reached past") {
		// Either the dialect answers the length as the read answers it —
		// zsh, which is silent either way, and ksh93, which gives the read's
		// sentence and ends the script — or no dialect was chosen and ask
		// has said so.
		return false
	}
	r.diagf("%s\n", Wording(r.diag().SubscriptBeforeTheFirstElementLength,
		"[%[1]s]: bad array subscript", written))
	r.expandErr = true
	return true
}
