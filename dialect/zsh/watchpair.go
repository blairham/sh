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
// **Modeled since #4999**, and the fear that kept it out of #4907 turned out
// to be misplaced: it is not a change to what `unset` means for every pair in
// the engine. A tie and this pair are two different joins, and the `wordless`
// field already told them apart — `interp.Runner.PairNames` has exactly one
// user and the tie table has the other nine. The tie branch in
// interp/builtin.go still removes both halves of a tie, which is what the
// reference does for `typeset -T SCA sca` and for the shell's own eight, and
// dialect/zsh/watchunset_test.go carries those six rows as the control beside
// the four that moved.
//
// The surviving half is **emptied** and keeps `special`, and the join is then
// live only into a half that is still there: after `unset watch`, writing
// `watch` re-creates it and reaches `$WATCH`, and writing `WATCH` does not
// bring `watch` back. Four cells, varying which half was removed and which is
// then written, because those are the two nouns the rule is keyed on — see
// interp.Runner.pairHalfWasRemoved.
func registerWatchPair(r *interp.Runner) {
	r.PairNames("WATCH", "watch", ":")
	// The shell's own, which is the `special` word in `${(t)…}` and is what
	// separates these two from an array a script wrote. Both halves carry
	// it there — `scalar-special` and `array-special` — and nothing else.
	r.MarkShellOwnParameter("WATCH")
	r.MarkShellOwnParameter("watch")
	// And the module the pair belongs to, which a reference to either half
	// loads — see loadWatchModuleOnAReference for the grid and for the
	// control that says it is a reference and not a write.
	r.SetParameterArrival("WATCH", loadWatchModuleOnAReference)
	r.SetParameterArrival("watch", loadWatchModuleOnAReference)
}

// registerWatchModuleParameters installs `$WATCHFMT` and `$LOGCHECK`, the two
// `zsh/watch` brings besides the pair above.
//
// Measured 2026-09-28 on zsh 5.9.2 under `-f` from a script file with `env -i
// PATH=/usr/bin:/bin TERM=dumb` and a scratch `HOME`, after `zmodload
// zsh/watch`:
//
//	${(t)WATCHFMT}   scalar    $WATCHFMT   %n has %a %l from %m.
//	${(t)LOGCHECK}   integer   $LOGCHECK   60
//	typeset -p WATCHFMT   typeset WATCHFMT='%n has %a %l from %m.'
//	typeset -p LOGCHECK   typeset -i LOGCHECK=60
//
// Neither word carries `special` or either hiding letter, and both names are
// writable — `WATCHFMT=x` and `LOGCHECK=5` are taken at 0 — so they are
// ordinary parameters the module lays down rather than views of anything.
//
// **Neither has a reader here**, which is worth saying in advance: there is no
// login watch in this shell, so what is modeled is the arrival and the values
// and not the feature.
func registerWatchModuleParameters(r *interp.Runner) {
	r.SetVar("WATCHFMT", "%n has %a %l from %m.")
	r.SetVar("LOGCHECK", "60")
	r.MarkInteger("LOGCHECK")
}

// loadWatchModuleOnAReference is the second route into that installer: in the
// shell being modeled, **a reference to either half of the pair loads the
// module**.
//
// The issue reported a *write* as the trigger. It is a reference, and the
// difference is measurable — measured 2026-09-28 by asking `zmodload -e
// zsh/watch` after one line, with the probe naming neither `WATCHFMT` nor
// `LOGCHECK`, because a probe that named them would be asking the module to
// arrive in order to find out whether it had:
//
//	(nothing)                1, not loaded
//	: ${+watch}              0, loaded      — a set test is a reference
//	: ${+WATCH}              0, loaded
//	print -r -- "$watch"     0, loaded      — so is a plain read
//	watch=(a b)              0, loaded      — and so is a write
//	WATCH=cc                 0, loaded
//	: ${+WATCHFMT}           1, not loaded  — the module's *other* names are not
//	: ${+LOGCHECK}           1, not loaded
//	typeset -p WATCH         0 at 0, not loaded
//	unset watch              not loaded
//
// The last four rows are the control and they are what makes this a rule about
// a **reference** rather than about the name appearing anywhere: a listing and
// an `unset` mention `WATCH` and load nothing, which is the same line the
// deferral roster draws — see interp.Runner.referredToParameter, whose own
// comment says a listing must not be the thing that brings a name in.
func loadWatchModuleOnAReference(r *interp.Runner) {
	// Recorded as loaded as well as installed, because `zmodload -e
	// zsh/watch` is what the measurement above was taken with: the module is
	// **0** on the line after the reference and 1 before it, so a shell that
	// installed the two parameters and said the module was absent would be
	// wrong about the thing the probe asked.
	zmodloadSetLoaded(r, "zsh/watch", true)
	installGatedParameters(r, "zsh/watch")
}
