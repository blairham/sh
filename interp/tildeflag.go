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
	dir, tail, ok := r.tildeSplit(v)
	if !ok {
		return v
	}
	return dir + tail
}

// tildeSplit is tildeValue with the seam still visible: the directory the
// tilde named, and the text that followed it, rather than the two joined.
//
// A pattern needs the two apart, because they are worth different things in
// it. Measured on zsh 5.9.2 with `HOME=/tmp/p78home/a*b`, both directories
// present:
//
//	[[ '/tmp/p78home/a*b' == ~ ]]    true
//	[[ '/tmp/p78home/axxb' == ~ ]]   false
//	[[ "$HOME/abc" == ~/a* ]]        true
//
// So the directory is matched as text — its own metacharacters are not live —
// while the tail after it is a pattern like any other. Joining them first and
// escaping the result would lose the second row; not escaping at all would
// lose the first.
//
// ok is false where nothing expanded, which leaves the caller the word as
// written: a `~user` this package has no database for, a `~-` in a shell with
// no $OLDPWD, and a `~` in a run with no $HOME.
func (r *Runner) tildeSplit(v string) (dir, tail string, ok bool) {
	if !strings.HasPrefix(v, "~") {
		return "", "", false
	}
	rest := v[1:]
	name := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		name, tail = rest[:i], rest[i:]
	}
	switch name {
	case "+", "-":
		// `~+` is $PWD and `~-` is $OLDPWD in three of the four, and only
		// when the variable is set: a fresh shell's `~-` stays literal.
		if dir, ok := r.tildeDirVar(name); ok {
			return dir, tail, true
		}
		return "", "", false
	case "":
		home, ok := r.getVar("HOME")
		if !ok {
			return "", "", false
		}
		return home, tail, true
	}
	// `~user` needs a user database this package does not carry, so it is
	// left alone rather than guessed at.
	return "", "", false
}

// tildeFlagFields applies the `${~spec}` flag to the fields a substituted
// word came to — the operand of `${~x:-word}` and `${~x:+word}`.
//
// Both halves, and the pattern half is why it has to exist at all: the flag
// overrides quoting, so a quoted operand becomes a pattern where an unquoted
// one already was. Measured on zsh 5.9.2, 2026-09-08, in a directory holding
// `Xay` and `Xby` and with `~` naming a directory holding `zz`:
//
//	${~u:-"X[a-b]y"}   Xay Xby     the quotes do not stop the match
//	${~~u:-X[a-b]y}    Xay Xby     and the off parity does not either: the
//	                               operand's own metacharacters are written
//	                               rather than substituted, so they were
//	                               never the flag's to switch off
//	${~u:-"~/zz"}      HOME/zz     the tilde half reaches the operand too
//
// The middle row is the one that says where the flag stops. It is the same
// rule the scalar path follows through escapeResult — the flag decides
// whether the *result* reads as a pattern and leaves written text alone — so
// the marks stay put when the flag is off and come off when it is on.
//
// The tilde half is here rather than in the caller for the reason the pattern
// half is: the operand's fields never reach the split path where
// tildeFlagElements is applied, so `${~u:-"~/zz"}` was the text it was
// written as (#1500).
func (r *Runner) tildeFlagFields(s syntax.Span, head bool, fields []string) []string {
	if !tildeFlagOn(s) || len(fields) == 0 {
		return fields
	}
	// The marks come off before the tilde is looked for and the value's own
	// backslashes go back on after, which is exactly what escapeResult does
	// for an expansion whose result is a pattern: live metacharacters,
	// literal backslashes.
	plain := make([]string, len(fields))
	for i, f := range fields {
		plain[i] = globUnescape(f)
	}
	plain = r.tildeFlagElements(s, head, plain)
	for i, v := range plain {
		// No reading is chosen here: escapeValueBackslashes writes the mark
		// that records a value's backslash and commits to nothing, and
		// resolveValueBackslashes reads it once the whole field exists. This
		// is a round trip through the same form, so it has to produce the same
		// form (#1370).
		plain[i] = escapeValueBackslashes(v)
	}
	return plain
}

// substitutedColonTildes expands the tilde after each colon inside one
// substituted value, which is the half of an assignment's colon rule that
// expandColonTildes cannot reach: that one walks the word's *literal* spans,
// so a colon and a tilde that arrived together out of a parameter were never
// looked at.
//
// Only a tilde-flagged expansion's text comes here. A plain one's does not,
// and that is the measured difference rather than an economy — on zsh 5.9.2
// with `u='a:~/zz'`, `q=${~u}` is the home directory and `q=${u}` is the two
// characters as written. Set against the flag's other half in tildeFlagHead,
// which answers for the tilde at the front of the value, this answers for the
// ones a colon put at the front of a segment.
//
// The segment runs to the next colon, since a colon both ends one and begins
// the next, and a segment that runs to the end of the text is expanded rather
// than held: what follows in the word is a slash or a colon or nothing in
// every shape that has an answer here. `a:${~y}abc` with `y='~'` is the
// exception and it is not one we can serve — zsh reads it as `~abc` and dies
// on a user it has no entry for, and `~user` is left as written throughout
// this package for want of a user database.
func (r *Runner) substitutedColonTildes(v string) string {
	if !strings.Contains(v, ":~") {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != ':' || i+1 >= len(v) || v[i+1] != '~' {
			b.WriteByte(v[i])
			continue
		}
		b.WriteByte(':')
		end := len(v)
		if j := strings.IndexByte(v[i+1:], ':'); j >= 0 {
			end = i + 1 + j
		}
		seg := v[i+1 : end]
		if dir, tail, ok := r.tildeSplit(seg); ok {
			b.WriteString(dir)
			b.WriteString(tail)
		} else {
			b.WriteString(seg)
		}
		i = end - 1
	}
	return b.String()
}
