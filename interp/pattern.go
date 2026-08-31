// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// patternOf renders a word as a pattern, escaping the parts that were quoted.
//
// docs/spec/grammar/patterns.md: quoting decides whether text is a pattern at
// all. `$p` matches as a pattern where `"$p"` matches as a literal, so the
// matcher cannot be handed a plain string — it has to be told which characters
// were quoted, and escaping them here is how that is carried.
func (r *Runner) patternOf(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range w.Spans {
		text := s.Value
		if s.Kind == syntax.ParamExp {
			text = r.expandParam(s.Param)
		}
		// Unquoted literal text is a pattern, and so is the *result* of an
		// unquoted expansion where the dialect says so — the same axis that
		// decides whether `x="et*"; echo $x` globs, reaching into `[[ ]]`.
		// Escaping it unconditionally made `p="a*"; [[ abc == $p ]]` fail.
		if s.Quoting == syntax.Unquoted &&
			(s.Kind == syntax.Literal ||
				r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion")) {
			b.WriteString(text)
			continue
		}
		// Quoted text, and the result of an expansion in a quoted context,
		// are literal: every metacharacter in them is escaped.
		for i := 0; i < len(text); i++ {
			if strings.IndexByte(`*?[\`, text[i]) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(text[i])
		}
	}
	return b.String()
}

// matchPattern reports whether pattern matches the whole of s.
//
// The language is the one in patterns.md and nothing more: `*`, `?` and
// bracket expressions. There is no alternation and no repetition, and no way
// to ask for a longer or shorter match — `${x#p}` and `${x##p}` choose that by
// doubling the *operator*.
//
// The restrictions that apply in pathname expansion — no metacharacter matches
// `/` or a leading period — are deliberately not here. They belong to the
// caller, because the same language is used by `case` and by parameter
// expansion where there is no filesystem and no components. Putting them here
// would be wrong in two places out of three.
// patternOpts carries the dialect's answers into the matcher, which has no
// Runner and should not need one. It grew from a single bool the moment a
// second axis reached the same code.
type patternOpts struct {
	caret   bool
	bracket BracketPolicy
	// bad is set when the pattern is one the dialect rejects outright. It is
	// a field rather than a return value because matchHere recurses, and
	// threading a second result through every branch obscured the matching.
	bad *bool
}

func matchPattern(pattern, s string, o patternOpts) bool {
	return matchHere(pattern, s, o)
}

func matchHere(p, s string, o patternOpts) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			// Collapse a run of stars, then try every split point. The
			// shortest-first order does not matter: this answers whether a
			// match exists, not where it ends.
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if p == "" {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchHere(p, s[i:], o) {
					return true
				}
			}
			return false

		case '?':
			if s == "" {
				return false
			}
			p, s = p[1:], s[1:]

		case '[':
			if s == "" {
				return false
			}
			rest, ok := matchBracket(p, s[0], o)
			if !ok {
				return false
			}
			p, s = rest, s[1:]

		case '\\':
			// An escaped metacharacter is an ordinary character.
			if len(p) < 2 {
				return s == "\\"
			}
			if s == "" || s[0] != p[1] {
				return false
			}
			p, s = p[2:], s[1:]

		default:
			if s == "" || s[0] != p[0] {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return s == ""
}

// matchBracket consumes a bracket expression from p and reports whether c is
// in it, returning what is left of the pattern.
func matchBracket(p string, c byte, o patternOpts) (rest string, ok bool) {
	i := 1
	negate := false
	// `!` is the portable negation, everywhere. `^` is an extension dash
	// does not have, where it is an ordinary character — so whether it
	// negates is the caller's answer rather than this file's. Assuming it
	// did made `[^abc]` match the complement under the dash dialect, where
	// dash matches a literal caret.
	if i < len(p) && (p[i] == '!' || (p[i] == '^' && o.caret)) {
		negate = true
		i++
	}
	matched := false
	first := true
	for i < len(p) {
		if p[i] == ']' && !first {
			i++
			if negate {
				return p[i:], !matched
			}
			return p[i:], matched
		}
		first = false

		// A character class, [[:digit:]] and friends.
		if strings.HasPrefix(p[i:], "[:") {
			end := strings.Index(p[i:], ":]")
			if end >= 0 {
				if inClass(p[i+2:i+end], c) {
					matched = true
				}
				i += end + 2
				continue
			}
		}

		lo := p[i]
		// A `-` is literal at the end, which is why `[a-]` matches a dash.
		if i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']' {
			if c >= lo && c <= p[i+2] {
				matched = true
			}
			i += 3
			continue
		}
		if c == lo {
			matched = true
		}
		i++
	}
	// An unterminated bracket is not a bracket expression, and what it is
	// instead is the dialect's answer rather than this file's.
	switch o.bracket {
	case BracketLiteral:
		// bash and ksh93: an ordinary `[`, and the rest of the pattern
		// carries on from just after it.
		return p[1:], c == '['
	case BracketBadPattern:
		// zsh: not a pattern at all. Recorded rather than returned, because
		// matchHere recurses and a second result would have to be threaded
		// through every branch of it.
		if o.bad != nil {
			*o.bad = true
		}
		return "", false
	}
	// dash, and the shape the matcher had before any of this: a class that
	// can never match.
	return "", false
}

// hasUnterminatedBracket reports whether a pattern contains a `[` with no
// closing `]`, so the axis is asked only about patterns it applies to.
func hasUnterminatedBracket(p string) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			i++
			continue
		}
		if p[i] == '[' && !closesBracket(p, i) {
			return true
		}
	}
	return false
}

func inClass(name string, c byte) bool {
	switch name {
	case "digit":
		return isDigit(c)
	case "alpha":
		return isLetter(c)
	case "alnum":
		return isLetter(c) || isDigit(c)
	case "upper":
		return c >= 'A' && c <= 'Z'
	case "lower":
		return c >= 'a' && c <= 'z'
	case "space":
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
	case "punct":
		return c > ' ' && c < 127 && !isLetter(c) && !isDigit(c)
	case "xdigit":
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	}
	return false
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// patternOpts resolves the dialect's pattern answers for one pattern.
//
// The bracket axis is deliberately not resolved here: this is the path used by
// parameter expansion and by globbing, where an unterminated bracket is
// literal in every shell measured. Only `case` and `[[ ]]` ask it, through
// matchPatternR.
func (r *Runner) patternOpts(pattern string) patternOpts {
	return patternOpts{caret: r.caretNegates(pattern), bracket: BracketLiteral}
}
