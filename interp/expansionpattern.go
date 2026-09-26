// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// operandPatternOpts builds patternOpts for a pattern **operand of a parameter
// expansion** — the trims `${v#p}`, `${v##p}`, `${v%p}` and `${v%%p}`, the
// substitutions `${v/p/r}` and `${v//p/r}` with their anchored and deleting
// spellings, and the flag-projected trim `${(M)v#p}`.
//
// One thing differs from patternOpts and it is the whole of this file: the
// **unterminated-bracket axis is resolved here rather than pinned literal**.
// patternOpts pins it, on the reading that an unterminated bracket is literal
// in every shell this path serves, and that reading is wrong — the axis was
// measured on this very surface, with a prefix trim, and the panel splits
// three ways over it. Re-measured 2026-09-26 with
// `w='[abc'; printf '%s\n' "${w#[a}"`:
//
//	bash 5.3   bc                      the `[a` taken as two characters
//	ksh93      bc                      the same
//	dash       [abc                    a class that can never match
//	zsh 5.9.2  bad pattern: [a, 1      not a pattern at all
//
// Those are exactly [Semantics.UnterminatedBracket]'s three values, so the
// surface has no answer of its own to give: it asks. Pinning it made the two
// literal columns right by accident and the other two wrong — dash stripped a
// prefix it should not have matched, and **zsh reported success with a value
// it should have refused to compute**, which is the shape a script cannot see
// (#4646).
//
// The rule, with one noun in it: **the axis decides, not the surface.** Two
// pairs hold a noun fixed and move the other thing. Hold the *text* `[a`
// fixed and move the dialect: three different answers above. Hold the
// *dialect* fixed at zsh and move whether the bracket closes: `${v#[ab]}` is
// `bc` at status 0 and `${v#[a}` is refused. And the pair that names the trap
// this surface sets — a pattern that will not compile and a pattern that
// simply misses produce the *same value* and differ only in status: on
// `v=zzz`, `${v#q}` is `zzz` at 0 and `${v#[a}` is refused at 1. A probe
// reading the value alone cannot tell them apart.
//
// It answers two things at once because both turn on the same axis and the
// axis must be **resolved once**: a dialect that has not answered it reports
// so by name, and asking twice for one expansion reported it twice.
//
// refused is the **eager** half: a pattern this dialect will not compile is
// refused here, before any matching happens, and the caller hands the value
// back untouched.
//
// Eager because otherwise *whether the shell refuses depends on the value*,
// which is not a distinction the reference draws. Measured 2026-09-26 on zsh
// 5.9.2, `-f -c`: `v=zzz; ${v#x[a}` reports `bad pattern: x[a` at 1, with a
// pattern whose first character already rules the subject out. A refusal
// raised from inside the matcher cannot produce that row, and not only
// because the match fails early: spanByLength skips a candidate piece the
// pattern's edge literals could not fill, so on some subjects the matcher is
// never called at all. That prefilter is correct as a prefilter — it can only
// skip pieces that would not have matched — and it is exactly what makes a
// match-time refusal unreliable. The discriminating pair is one value apart:
// `v=a; ${v#[a}` reaches the matcher and `v=zzz; ${v#[a}` does not, and the
// reference refuses both.
//
// So the question is asked of the **pattern**, which is where compiling one
// belongs.
//
// **Status 1**, which is this surface's own number rather than a shared one:
// measured, `[[ x == (#Z)a ]]` exits 2 and the same refusal reached through
// `${x#…}` exits 1 — see extendedPatternOpts, which already carried that
// split for the pattern flags and reaches the same fatalPattern.
//
// Fatal to the script and not to the expansion. Measured 2026-09-26 from
// `-c`, with the refusal in each position: `${v#[a} || print CAUGHT` prints
// nothing and the script ends at 1, so `||` does not catch it; a function
// body ends the whole script and names itself in the report; an `eval` and a
// subshell each contain it and the script runs on with `$?` at 1. That is the
// boundary FatalErrorEndsBorrowedTextOnly already draws, which is why this
// goes through fatalPattern rather than raising anything of its own.
func (r *Runner) operandPatternOpts(pattern string, bad *bool, subjects ...string) (patternOpts, bool) {
	o := r.patternOpts(pattern, subjects...)
	o.bad = bad
	// A group nothing closes is the same "will not compile" arriving by the
	// other scan, and it is eager for the identical reason — measured on zsh
	// 5.9.2 (`-f -c`, 2026-09-26), `v=zzz; ${v#a(b}` is `bad pattern: a(b` at
	// 1, on a subject the pattern's first literal already rules out. It is
	// composed here rather than beside the bracket below because it does not
	// go through the bracket policy: there is one answer, not three. See
	// badPatternFromAnOpenGroup and #4645.
	if r.badPatternFromAnOpenGroup(pattern) {
		r.fatalPattern(pattern, 1)
		return o, true
	}
	if !hasUnterminatedBracket(pattern) {
		return o, false
	}
	o.bracket = r.bracketPolicy()
	if o.bracket != BracketBadPattern {
		return o, false
	}
	r.fatalPattern(pattern, 1)
	return o, true
}

// metABadExpansionPattern is the **lazy** half, and it is here for the one
// shape the scan above cannot see: a bracket a `[:name:]` left open.
// `${v#[[:alpha:]}` looks closed to hasUnterminatedBracket, which stops at
// the class's own `]`, so only the matcher — whose scan reads the class —
// knows. Measured, real zsh refuses that pattern too, and this is the same
// division of labor matchPatternR already makes for a `case` arm.
//
// It inherits the subject dependence the eager half exists to avoid, because
// there is nothing else to inherit: the question is only ever put where the
// matcher goes. A pattern of this shape that the matcher never reaches is
// still answered wrong, in every surface of the program alike.
func (r *Runner) metABadExpansionPattern(bad bool, pattern string) bool {
	if !bad {
		return false
	}
	r.fatalPattern(pattern, 1)
	return true
}
