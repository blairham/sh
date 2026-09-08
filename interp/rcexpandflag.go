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
// Nothing about it is a runner option. The state lives on the node, for the
// reason tildeflag.go gives: the decision is per expansion.

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

// rcExpandOn reports whether this span's `^` characters ask for the
// distribution.
//
// There is no option behind the `false`: `RC_EXPAND_PARAM` is recorded and
// inert in this shell, exactly as `GLOB_SUBST` is behind `${~spec}`, so a
// spec with no `^` is never distributive. The parity is still read in both
// directions rather than only for "on", because that is what `${^^name}`
// exists to say and it must keep saying it when the option is honored.
func rcExpandOn(s syntax.Span) bool {
	a, ok := rcExpandParity(s)
	return ok && a == Yes
}
