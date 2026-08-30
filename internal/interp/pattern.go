// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/internal/syntax"
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
		if s.Quoting == syntax.Unquoted && s.Kind == syntax.Literal {
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
func matchPattern(pattern, s string) bool {
	return matchHere(pattern, s)
}

func matchHere(p, s string) bool {
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
				if matchHere(p, s[i:]) {
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
			rest, ok := matchBracket(p, s[0])
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
func matchBracket(p string, c byte) (rest string, ok bool) {
	i := 1
	negate := false
	// `!` is the portable negation. `^` is an extension dash does not have,
	// where it is an ordinary character — accepted here because the core
	// excludes dash.
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
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
	// An unterminated bracket is not a bracket expression at all.
	return "", false
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
