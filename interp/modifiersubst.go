// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// `:s/l/r/` is the one modifier that is not a function of its letter, and the
// one that leaves something behind.
//
// It replaces a **literal** substring, not a pattern — measured three ways in
// the shell that has it: with `x=abc`, `${x:s/?/Z/}`, `${x:s/[ab]/Z/}` and
// `${x:s/b*/Z/}` all answer `abc`, and each of `?`, `[b]` and `*` is replaced
// where the value really contains it. So `${x/a/b}`'s pattern machinery is
// the wrong tool here and deliberately not reused.
//
// And it is stateful, which is the part with nowhere else to live. The
// pattern and replacement are remembered for the whole shell — not per
// parameter — so `${x:s/X/-/}` followed by `${y:s//+/}` reuses the `X`, and
// `:&` repeats the whole substitution. It is a scalar pair on the Runner, so
// `c := *r` gives a subshell its own copy with the parent's contents, which
// is the same treatment every other scalar gets.

// lastSubstitution is the pattern and replacement `:s` last used, which an
// empty pattern and `:&` both reach for.
type lastSubstitution struct {
	pattern string
	with    string
	set     bool
}

// substituteModifier is `:s<d>pattern<d>replacement<d>`, where the delimiter
// is whatever byte follows the letter.
//
// Any byte serves as the delimiter — `/ | # , :` all measured — and the
// closing one is optional: `${x:s/X/-}` is the same as `${x:s/X/-/}`. A
// backslash escapes a delimiter inside either half, which is how a `/` is
// replaced with `/` as its own delimiter.
func (r *Runner) substituteModifier(value, rest string, global bool, e *syntax.ParamExpr) (string, bool) {
	if rest == "" {
		// `${x:s}` with nothing after it. Not a modifier complaint — the
		// segment is a substitution with no body, which that shell reports as
		// a bad substitution, the same as any other malformed `${ }`.
		r.reportBadSubstitution(e)
		return "", false
	}
	delim := modifierDelimiter(rest, r.modifierDelimitersAreCharacters(rest))
	pattern, after, ok := scanDelimited(rest[len(delim):], delim)
	if !ok {
		r.reportBadSubstitution(e)
		return "", false
	}
	// The pattern is a literal string, so its escapes are resolved here and
	// what is remembered for the next `:s` is the string itself. The
	// replacement keeps its escapes until it is used, because an `&` in it is
	// not a character.
	written := pattern
	pattern = unescapeModifier(pattern)
	with, tail, closed := scanDelimited(after, delim)
	if !closed {
		// The closing delimiter is optional, so everything left is the
		// replacement.
		with, tail = after, ""
	}
	if tail != "" {
		// Text after the substitution, which is the complaint that names
		// nothing — the same one a letter with junk after it gets.
		r.refuseModifier(e, "")
		return "", false
	}
	if pattern == "" {
		// An empty pattern means the one before it, and there may not be
		// one. Reported by name rather than silently doing nothing.
		if !r.lastSubst.set {
			r.diagf("%s\n", Wording(r.diag().SubstringRangeError, "%[2]s",
				r.paramSubject(e), "no previous substitution"))
			r.expandErr = true
			return "", false
		}
		pattern = r.lastSubst.pattern
	}
	r.lastSubst = lastSubstitution{pattern: pattern, with: with, set: true}
	if r.histSubstPattern {
		// The modifier's own escapes are resolved first, as they are for the
		// literal reading; what is left is the pattern's, and the operator's
		// delimiter goes back in escaped.
		patternText := strings.ReplaceAll(unescapeModifier(written), "/", "\\/")
		if out, ok := r.substitutePattern(value, patternText, with, global); ok {
			return out, true
		}
	}
	return substituteLiteral(value, pattern, with, global), true
}

// substitutePattern is `:s` under one shell's `histsubstpattern`, where the
// left half is a pattern read the way `${x/pat/rep}` reads one — `#` and `%`
// anchor it, `(#b)` fills `$match` — and the right half is expanded for each
// match the same way. Measured 2026-10-02 on zsh 5.9.2 under `-f` with
// `extendedglob`, in a directory holding `tmpcd`, `tmpfile1` and `tmpfile2`:
//
//	print *(:s/t??/TING/)                             TINGcd TINGfile1 …
//	foo=(one.c two.c three.c)
//	print ${foo:s/#%(#b)t(*).c/T${match[1]}.X/}       one.c Two.X Three.X
//
// and with the option off the same two are literal and change nothing
// (#5155). Read through the expansion's own grammar: the two halves are
// handed to the parser as `${v/pat/rep}`, so every rule that operator keeps
// is kept here without a second copy.
func (r *Runner) substitutePattern(value, pattern, with string, global bool) (string, bool) {
	op := "/"
	if global {
		op = "//"
	}
	if r.modifierTextEscaped {
		with = globUnescape(with)
	}
	f, err := syntax.Parse("${_"+op+pattern+"/"+withoutQuotes(with)+"}", r.dialect())
	if err != nil || len(f.Stmts) != 1 {
		return "", false
	}
	p, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(p.Cmds) != 1 {
		return "", false
	}
	c, ok := p.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(c.Args) != 1 || len(c.Args[0].Spans) != 1 || c.Args[0].Spans[0].Param == nil {
		return "", false
	}
	e := c.Args[0].Spans[0].Param
	return r.replaceWith(value, r.patternOf(e.Arg), e), true
}

// repeatSubstitution is `:&` — the last substitution again, on this value.
//
// A no-op at status 0 where there has been none, which is measured and is
// **not** the same as an empty pattern: `${x:s//new/}` with no previous
// substitution is refused by name, and `${x:&}` with none is silence. The two
// reach for the same memory and answer differently when it is empty.
func (r *Runner) repeatSubstitution(value string, global bool, _ *syntax.ParamExpr) (string, bool) {
	if !r.lastSubst.set {
		return value, true
	}
	return substituteLiteral(value, r.lastSubst.pattern, r.lastSubst.with, global), true
}

// scanDelimited reads up to the next unescaped delim, answering the field as
// it was written, what is left after the delimiter, and whether one was found.
//
// A backslash protects the byte after it, so an escaped delimiter does not end
// the field. The escapes are left in the text rather than resolved here,
// because the replacement half has one more question to ask of them than the
// pattern half does — see substituteLiteral.
func scanDelimited(s, delim string) (text, rest string, found bool) {
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
		case strings.HasPrefix(s[i:], delim):
			return s[:i], s[i+len(delim):], true
		}
	}
	return s, "", false
}

// unescapeModifier resolves the escapes of a field that has already been
// found: a backslash stands for the byte after it, whatever that byte is.
// Measured, with the modifier's own delimiter out of the way: `${x:s/\./:/}`
// replaces a `.` rather than a backslash-dot, and `${x:s/\\a/:/}` replaces a
// backslash followed by an `a`.
func unescapeModifier(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// substituteLiteral replaces pattern with the replacement — the first
// occurrence, or every one where global.
//
// `&` in the replacement is the matched text and `\&` is a literal `&`, which
// is the one piece of interpretation the replacement gets. Since the pattern
// is a literal string the match is always the pattern itself, so this is
// simpler than it looks; it is written out anyway because a replacement that
// silently dropped its `&` would be a plausible wrong answer.
func substituteLiteral(value, pattern, with string, global bool) string {
	if pattern == "" {
		return value
	}
	n := 1
	if global {
		n = -1
	}
	return strings.Replace(value, pattern, expandAmpersand(with, pattern, backslashProtectsAnything), n)
}

// backslashRule is what a backslash does to the byte after it in a
// replacement, which is the one thing the two constructs that read an `&`
// disagree about.
type backslashRule bool

const (
	// backslashProtectsAnything is the history modifier's rule: a backslash
	// stands for the byte after it whatever that byte is, and disappears.
	// Measured with the modifier's own delimiter out of the way — see
	// unescapeModifier, which resolves the *pattern* half the same way.
	backslashProtectsAnything backslashRule = false

	// backslashProtectsItself is the parameter substitution's rule: a
	// backslash is an escape only before an `&` or another backslash, and is
	// kept where it stands in front of anything else. Measured on bash
	// 5.3.15, 2026-09-11, with `v=abc` and the reading on — `r='[\&]'` gives
	// `a[&]c`, `r='[\\&]'` gives `a[\b]c`, and `r='[\a]'` gives `a[\a]c`,
	// where the modifier's rule would have answered `a[a]c` for the last.
	backslashProtectsItself backslashRule = true
)

// expandAmpersand puts the matched text where the replacement wrote `&`, and
// resolves the replacement's escapes in the same pass.
//
// One pass rather than two, because the order of the two questions is
// observable: `\&` is a literal ampersand and `\\&` is a literal backslash
// followed by the matched text. A pass that resolved the escapes first would
// turn the second into `\&` and then read that as the literal, and a pass that
// answered the ampersands first would never see the difference at all.
//
// One function rather than two, because the `&` is the same rule in both
// places and only the backslash differs: a second copy carrying the modifier's
// escape rule is exactly how a fix to one of them would miss the other, which
// is a shape this repository has paid for more than once. What the parameter
// substitution hands in is a **real** match rather than the pattern itself —
// the modifier replaces a literal substring, so there the two are the same
// string and here they are not.
func expandAmpersand(with, matched string, rule backslashRule) string {
	var b strings.Builder
	for i := 0; i < len(with); i++ {
		switch {
		case with[i] == '\\' && i+1 < len(with):
			if rule == backslashProtectsItself &&
				with[i+1] != '&' && with[i+1] != '\\' {
				// Not an escape under this rule, so the backslash is text
				// and the byte after it is read again as itself.
				b.WriteByte(with[i])
				continue
			}
			b.WriteByte(with[i+1])
			i++
		case with[i] == '&':
			b.WriteString(matched)
		default:
			b.WriteByte(with[i])
		}
	}
	return b.String()
}

// withoutQuotes takes the quote characters out of a modifier's replacement,
// which the modifier's own reading has already done before the expansion
// sees it: measured 2026-10-02 on zsh 5.9.2, under `histsubstpattern`
// `${x:s/#(#b)tmp(*e)/'scrunchy${match[1]}'/}` on `tmpfile1` is
// `scrunchyfile1` — the quotes gone and the parameter still expanded —
// where the same text as `${x/…/'…'}` keeps the parameter literal.
func withoutQuotes(s string) string {
	if !strings.ContainsAny(s, `'"`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			b.WriteByte(c)
			i++
			b.WriteByte(s[i])
		case c == '\'' || c == '"':
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
