// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The `${^spec}` flag: a `^` written between the `${` and the parameter,
// which makes the expansion *distributive* — the word it stands in is
// produced once per element, with the text around it on each, rather than
// once with the elements laid into it. It is the per-expansion spelling of
// the `RC_EXPAND_PARAM` option. One grammar in the panel has the construct,
// so what it means here is that shell's answer, measured and recorded in
// docs/spec/grammar/parameter-expansion.md.
//
// The third of the flags that share a slot, and the one that is not about the
// value at all. `${~spec}` and `${=spec}` each change what one expansion
// *comes to*; this one changes how what it came to is laid into the word, so
// it cannot live where those two live:
//
//   - It is not a step of the expansion. Measured, `x${(j:-:)^a}y` on
//     `a=(1 2)` is the single word `x1-2y`: the group joined first and the
//     distribution then had one element to distribute. So it runs on the
//     *fields the span produced*, after everything the span does to them,
//     which is also why a split reaches it — `setopt shwordsplit; v='a b';
//     x${^v}y` is `xay xby`.
//   - Its subject is the word and not the field. Two distributive spans in
//     one word are a cross product, measured: `${^a}z${^a}` on `a=(1 2)` is
//     `1z1 1z2 2z1 2z2`, four words, the later span varying fastest. That is
//     one rule about a set of open fields, and it lives in the word builder
//     rather than in a transformation of one span's result.
//
// Quoting is not a third thing it asks about, which is the measurement that
// keeps this from being a third quoting rule beside the tilde flag's and the
// split flag's. Every quoted row falls out of "the fields the span produced":
//
//	"x${^a}y"      x1 2y        the quotes joined the list, so one field
//	"x${^a[@]}y"   x1y x2y      `[@]` keeps its fields through quotes
//	"x${^@}y"      x1y x2y      and so does `$@` itself
//	"x${^a[*]}y"   x1 2y        while `[*]` is the join again
//	"x${^=v}y"     xay xby      the split flag reaches through quotes, and
//	                            the distribution spreads what it left
//	set --; "x${^@}y"           no word at all, where `"x${@}y"` is `xy`
//
// So a quoted spelling that keeps its fields distributes over them. A guard
// on Quoting would answer the first row right and the next five wrong.
//
// The *flag* is not a runner option: its state lives on the node, for the
// reason tildeflag.go gives — the decision is per expansion. The option
// behind it is, and it is the default this reads when no `^` was written;
// see Runner.rcExpandOn and Semantics.ParamExpansionDistributesOverTheWord.

// rcExpandParity is the effective answer the written `^` characters give, and
// whether they gave one at all.
//
// Parity and not a toggle: measured under `RC_EXPAND_PARAM` both ways, one
// `^` distributes and two do not, from either starting point, and only a spec
// with no `^` consults the option.
func rcExpandParity(s syntax.Span) (Answer, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil || s.Param.RcExpandFlags == 0 {
		return Unspecified, false
	}
	if s.Param.RcExpandFlags%2 == 1 {
		return Yes, true
	}
	return No, true
}

// rcExpandOn reports whether this span distributes over the word it stands
// in — because its own `^` characters asked for it, or because nothing did
// and the option behind [Semantics.ParamExpansionDistributesOverTheWord] is
// on.
//
// **Parity first, and the option only where no `^` was written.** That is
// what makes the flag and the option one mechanism rather than two: the
// option supplies the default the parity overrides, so `${^a}` distributes
// with `RC_EXPAND_PARAM` off and `${^^a}` does not with it on — measured, and
// the pair is what says the two are not independent switches to be ANDed or
// ORed.
//
// The axis is asked **only of a parameter expansion**, which is the
// measurement that keeps this from being a rule about fields in a word.
// A command substitution produces fields in exactly the same shape and the
// option does not reach it: `x$(echo p q)y` is `xp qy` in both states, while
// the same command wrapped in a parameter expansion —
// `x${(f)"$(printf 'p\nq\n')"}y` — is `xp qy` off and `xpy xqy` on. The
// guard below is therefore on the span's *kind* and not on how many fields
// it handed over; rcExpandParity's own `false` already covers every other
// kind, and the kind is restated here so that a later caller cannot reach
// the axis through a span that has no `${…}` to have written a `^` in.
//
// Read off the axis rather than off a stored bit, so `(setopt rcexpandparam)`
// stays in the subshell and `emulate -R` puts it back with the rest of the
// vector.
//
// Compared against Yes rather than asked through Runner.ask: every shell in
// the panel answers No and the disagreement exists only inside the one that
// has the option, so a core with no dialect chosen has no conflict to be
// told about — and asking here would refuse on every word holding a
// parameter expansion, which is most of them.
func (r *Runner) rcExpandOn(s syntax.Span) bool {
	if a, ok := rcExpandParity(s); ok {
		return a == Yes
	}
	if s.Kind != syntax.ParamExp || s.Param == nil {
		return false
	}
	return r.sem().ParamExpansionDistributesOverTheWord == Yes
}
