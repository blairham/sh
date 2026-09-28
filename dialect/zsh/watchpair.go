// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$WATCH` and `$watch`: the users whose logins and logouts this shell
// reports, written either as a colon-joined string or as a list.
//
// # They are one parameter, and the reference does not call it `tied`
//
// That is the whole of what #4907 asked to be measured before any code, and
// both halves of it are facts about the same shell in the same run. Measured
// 2026-09-27 on zsh 5.9.2 under `-f` from a script file with `env -i
// PATH=/usr/bin:/bin TERM=dumb` and a scratch `HOME`.
//
// The join is real and it is `:`, in both directions:
//
//	watch=(a b);  print -r -- "$WATCH"          a:b
//	WATCH=cc;     print -r -- "${(j:,:)watch}"  cc
//	WATCH='a:b::c'; print -r -- "${#watch}"     4    — an empty field is a field
//
// And the words are not a tie's:
//
//	${(t)PATH}    scalar-tied-export-special     ${(t)WATCH}   scalar-special
//	${(t)path}    array-tied-special             ${(t)watch}   array-special
//
// So implementing this pair through the built-in tie — which is what the
// behavior above asks for — would have answered `scalar-tied-special` where
// the reference answers `scalar-special`, trading one wrong word for another.
// That was the question, and the third measurement settles it: `typeset +T`
// there lists the eight tied pairs and `ZSH_EVAL_CONTEXT`, and **names
// neither half of this one**. The word is not decided by the join. It marks
// the tie *table* — what `typeset -T` writes into and what `typeset +T`
// walks — and this pair is not in it. `interp.Runner.PairNames` is that
// distinction in the substrate: a join with no name of its own, which mirrors
// exactly as a tie does and is invisible to every listing that says `tied`.
//
// # Both halves arrive on a first reference
//
// They are in the deferral roster, which is measured rather than assumed from
// their being specials: a bare `typeset` in a shell that has referred to
// nothing writes `undefined WATCH` and `undefined watch` among its forty such
// rows, and `typeset -p WATCH` as the first statement of a script is nothing
// at 0. See deferredparameters.go — they are the only two names in that
// roster that no module names.
//
// # What an `unset` does, and the half of it this shell does not model
//
// Measured, one shell per row, from a pair holding `(a b)`:
//
//	unset watch    ${+WATCH} 1, ${+watch} 0, `$WATCH` empty, still scalar-special
//	               and watch=(q r) afterwards writes `$WATCH` again
//	unset WATCH    ${+WATCH} 0, ${+watch} 1, `$watch` emptied, still array-special
//	               and watch=(q r) afterwards does **not** write `$WATCH`
//
// So the reference removes **one** half and leaves the other standing, empty
// and still special, with the pairing intact in one direction only: after
// `unset watch`, writing `watch` reaches `$WATCH` again and writing `WATCH`
// does not bring `watch` back.
//
// This engine's `unset` of either half of a pair removes **both** names,
// because half a tie is not a state it has — see the tie branch in
// interp/builtin.go, where that is measured for `typeset -T` and for the
// shell's own eight, both of which agree with this shell. So here the same
// two lines give `${+WATCH}` of 0 against 1, and a `WATCH=x:y` afterwards
// re-makes the pair rather than writing a lone scalar.
//
// That row is **left where it was found** rather than fixed in passing, and
// it is written down rather than left to be discovered. It is a change to
// what `unset` means for every pair in the engine, measured for the other
// nine against a shell that agrees with this one about them, and a pair that
// is half-removed is a state nothing else here can currently be in. What a
// script sees on every line before the `unset` is the join, and the join is
// right.
func registerWatchPair(r *interp.Runner) {
	r.PairNames("WATCH", "watch", ":")
	// The shell's own, which is the `special` word in `${(t)…}` and is what
	// separates these two from an array a script wrote. Both halves carry
	// it there — `scalar-special` and `array-special` — and nothing else.
	r.MarkShellOwnParameter("WATCH")
	r.MarkShellOwnParameter("watch")
}
