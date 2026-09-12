// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `${=spec}` flag: an `=` written between the `${` and the parameter,
// which splits the *result* of the substitution into words on `IFS` whatever
// the `SH_WORD_SPLIT` option says. One grammar in the panel has the
// construct, so what it means here is that shell's answer, measured and
// recorded in docs/spec/grammar/parameter-expansion.md.
//
// It is the sibling of `${~spec}`, and the same three decisions were taken
// for the same reasons: the count is parity rather than a toggle, the state
// lives on the node rather than on the Runner, and the flag *overrides an
// answer the expander already computes* rather than carrying a mechanism of
// its own. There is exactly one field splitter in this package and this flag
// calls it; a second one would be a second set of edge cases to keep in step.
//
// Two things about it are not the tilde flag's, and both are measured:
//
//   - Quoting does not suppress it. `"${=v}"` splits, where `"${~g}"` is the
//     value unchanged. So this flag cannot ride on expansionResult's quoted
//     short-circuit, and the splitting happens where the *fields* of an
//     expansion are produced.
//   - It is one string that is split, not each element of a list. Measured:
//     `a=(' x ' y); "${=a[@]}"` is three fields, not the four that splitting
//     each element would give — the elements are joined on IFS[0] first,
//     which is #1043's finding about what an unquoted list comes to, reached
//     from the other side.
//
// A context that never splits is not overridden. `x=${=v}` is the value
// unchanged, and so are `[[ ${=v} = "a b" ]]`, `case ${=v}` and a
// here-document body — all measured. The flag decides the *option's*
// question, not the context's, so splitNever still answers first.

// splitFlagParity is the effective answer the written `=` characters give,
// and whether they gave one at all.
//
// Parity and not a toggle: measured under `SH_WORD_SPLIT` both ways, one `=`
// splits and two do not, from either starting point, and only a spec with no
// `=` consults the option.
//
// No quoting clause, unlike tildeFlagParity. That is the measured difference
// between the two flags and not an omission: `"${=v}"` on `a b` is two
// fields in the shell that has the construct.
func splitFlagParity(s syntax.Span) (Answer, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil || s.Param.SplitFlags == 0 ||
		s.Param.HasFlags {
		return Unspecified, false
	}
	if s.Param.SplitFlags%2 == 1 {
		return Yes, true
	}
	return No, true
}

// splitFlagInGroup is the same parity read for an expansion that also carries
// a parenthesized flag group, where the split is not a step after the group
// but a step *inside* it — the group's own splitting rule, which runs before
// the ordering, the quoting and the case conversion.
//
// Measured, and this is what fixes the order: `v='c a b'; ${(o)=v}` is
// `a b c`, so the split ran before the sort and not on the sorted single
// word; `v='a b'; ${(q)=v}` is two plain words, where a split after the
// quoting would have left the backslash the quoting added.
//
// `(f)`, `(s)` and `(0)` are that same step with a separator of their own, so
// an `=` beside any of them adds nothing: `v='a b'; ${(s.,.)=v}` is one field,
// and `${(s.,.)==v}` still splits because the group decides and the parity is
// never consulted.
func splitFlagInGroup(e *syntax.ParamExpr, sp splitPolicy) bool {
	return sp != splitNever && e.SplitFlags%2 == 1 &&
		!strings.ContainsAny(e.Flags, splitFlagLetters)
}

// splitFlagAnswer is the answer to "is the result of this expansion split
// into fields", for one span: the context's, unless the context leaves the
// question open and the span carries written `=` characters, which decide it
// outright in either direction.
func splitFlagAnswer(s syntax.Span, sp splitPolicy, sem Answer) Answer {
	if sp == splitNever {
		// The exemption is unanimous in these contexts and the flag does not
		// reach it, measured: `x=${=v}` is the value unchanged.
		return sp.answer(sem)
	}
	if a, ok := splitFlagParity(s); ok {
		return a
	}
	return sp.answer(sem)
}

// splitFlagOn reports whether this span's `=` characters ask for the split
// here, in this context.
func splitFlagOn(s syntax.Span, sp splitPolicy) bool {
	if sp == splitNever {
		return false
	}
	a, ok := splitFlagParity(s)
	return ok && a == Yes
}

// splitFlagFields applies the flag to whatever the expansion came to.
//
// One string is split, so a list is joined on IFS[0] first — which is not a
// step of this flag's own but the answer this implementation already gives
// for what an unquoted list comes to when it is read as one value.
//
// Quoted, the edges are kept: `"${=v}"` on `' a '` is three fields, the outer
// two empty, and on `”` it is one empty field. Unquoted the ordinary rules
// discard them, which is the same splitter with its ordinary answer.
func (r *Runner) splitFlagFields(s syntax.Span, sp splitPolicy, parts []string) []string {
	if !splitFlagOn(s, sp) {
		return parts
	}
	ifs, set := r.ifs()
	return r.splitFieldsAsking(strings.Join(parts, ifsFirst(ifs, set)), nil, ifs, set,
		s.Quoting != syntax.Unquoted, true)
}
