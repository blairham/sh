// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `${~spec}` flag: a tilde written between the `${` and the parameter,
// which makes the *result* of the substitution eligible for tilde expansion
// and filename generation. One grammar in the panel has the construct, so
// what it means here is that shell's answer, measured and recorded in
// docs/spec/grammar/parameter-expansion.md.
//
// It is two halves that happen to share a character, and they are answered in
// two different places because they already have two different homes:
//
//   - The pattern half is the question GlobExpansionResults answers for every
//     expansion, so the flag *overrides that answer* for one expansion rather
//     than carrying a mechanism of its own. That is what makes it reach the
//     scalar path, the list path, the flag-group path and a pattern operand
//     without four copies of the rule.
//   - The tilde half has no existing axis — no shell in the panel expands a
//     tilde out of a value by default — so it is applied here, to the head of
//     the text the expansion came to, in the positions where a written tilde
//     would have expanded.
//
// Nothing about it is a runner option. The state lives on the node, which is
// where it belongs: the decision is per expansion and reading it off a field
// that `$-` reports would answer for the option while it was flipped, which
// is the mistake #1041 made with `noglob` and had to undo.

// tildeFlagParity is the effective answer the written tildes give, and whether
// they gave one at all.
//
// Parity and not a toggle: measured under `GLOB_SUBST` both ways, one tilde is
// yes and two are no from either starting point. Quoting suppresses the whole
// construct, exactly as it suppresses ordinary filename generation, so a
// quoted span never has an answer of its own.
func tildeFlagParity(s syntax.Span) (Answer, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil ||
		s.Param.TildeFlags == 0 || s.Quoting != syntax.Unquoted {
		return Unspecified, false
	}
	if s.Param.TildeFlags%2 == 1 {
		return Yes, true
	}
	return No, true
}

// globSubstAnswer is the answer to "is the result of this expansion a
// pattern", for one span: the dialect's, unless the span carries written
// tildes, which decide it outright in either direction.
func (r *Runner) globSubstAnswer(s syntax.Span) Answer {
	if a, ok := tildeFlagParity(s); ok {
		return a
	}
	return r.sem().GlobExpansionResults
}

// tildeFlagOn reports whether this span's tildes ask for the tilde half.
func tildeFlagOn(s syntax.Span) bool {
	a, ok := tildeFlagParity(s)
	return ok && a == Yes
}

// tildeFlagHead applies the flag's tilde half to text that begins a word.
//
// head is whether anything has been accumulated in front of this text in the
// word being built. Nothing has, for `${~t}` and for `${empty}${~t}`, and
// something has for `x${~t}` — which is the same limit this implementation's
// literal tilde expansion has, since both ask about the head of a word.
func (r *Runner) tildeFlagHead(s syntax.Span, head bool, text string) string {
	if !head || !tildeFlagOn(s) {
		return text
	}
	return r.tildeValue(text)
}

// tildeFlagElements applies the tilde half to the elements of a list.
//
// The first element joins whatever precedes it in the word, so it is at a head
// only when head says so. Every later element is a field of its own and is
// therefore at its own head, measured: `a=('~/zz' '~/qq'); X${~a[@]}` is
// `X~/zz` and the expanded `~/qq`.
func (r *Runner) tildeFlagElements(s syntax.Span, head bool, elems []string) []string {
	if !tildeFlagOn(s) || len(elems) == 0 {
		return elems
	}
	out := make([]string, len(elems))
	for i, el := range elems {
		if i == 0 && !head {
			out[i] = el
			continue
		}
		out[i] = r.tildeValue(el)
	}
	return out
}

// tildeValue replaces a leading `~` in one value with the directory it names.
//
// The body of expandTilde, lifted out so the written tilde and the substituted
// one cannot drift apart: `~`, `~+` and `~-` resolve, `~user` is left as
// written because this package carries no user database, and anything past the
// first slash is the tail.
func (r *Runner) tildeValue(v string) string {
	if !strings.HasPrefix(v, "~") {
		return v
	}
	rest := v[1:]
	name, tail := rest, ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		name, tail = rest[:i], rest[i:]
	}
	switch name {
	case "+", "-":
		// `~+` is $PWD and `~-` is $OLDPWD in three of the four, and only
		// when the variable is set: a fresh shell's `~-` stays literal.
		if dir, ok := r.tildeDirVar(name); ok {
			return dir + tail
		}
		return v
	case "":
		home, ok := r.getVar("HOME")
		if !ok {
			return v
		}
		return home + tail
	}
	// `~user` needs a user database this package does not carry, so it is
	// left alone rather than guessed at.
	return v
}
