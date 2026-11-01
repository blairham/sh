// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode/utf8"
)

// plainPatternShortcut switches the answer below on. It is a variable so a
// test can drive the same patterns through the matcher and through the
// shortcut and require the same answers.
var plainPatternShortcut = true

// plainPatternSpecials are the bytes that can mean something to the matcher
// in some dialect under some option: the POSIX three, the escape, every
// character an extended or ksh-style pattern spells an operator with, and
// the bracket a numeric range is written in. A pattern that holds none of
// them, apart from stars and the angle brackets innerStarsOnly admits, is a
// string to compare, or strings joined by stars.
const plainPatternSpecials = "*?[]\\()|<>^#~!@+"

// plainPatternMatch answers a whole-subject match for the patterns that need
// no matcher: a literal, `*`, a literal with a star at one end or both, and
// literals joined by single stars, which is the shape of F-Sy-H's table
// search `${out[(i)<*>_${k}]}` — a few hundred searches a start, each one
// matched against every key in turn (#6363). ok is false for every other
// pattern, which goes to the matcher as before.
//
// These are most of what a real configuration matches. A startup on a
// prompt theme and a plugin manager (#5873) matched 91,220 patterns, and
// 53,000 of them were these shapes — `case` arms like `(-p)` and
// `(_fsh_lifecycle_*)` run once per option or per function name, and
// `[[ $x == foo* ]]`. Each one paid for every policy the matcher asks about
// the pattern's text before it compared a byte.
//
// The answer is the matcher's answer, and the cases that would make it
// otherwise are excluded rather than reproduced:
//
//   - a run-time option folding case, which compares differently;
//   - a condition whose dialect writes a record of what a pattern with an
//     operator matched, since the star is an operator and the record is the
//     matcher's to write. A literal never writes it — see
//     patternHasAnOperator — and a `case` arm never does.
//
// Nothing else the matcher does reaches these shapes: a flag, a group, a
// bracket, an escape or a numeric range each needs a byte from the set
// above, and neither the policies nor the refusals in matchPatternChecked
// fire on a pattern without one. A literal holding a partial character is
// the one place bytes and characters part, and it is left to the matcher too.
// A star matches any bytes at all, valid characters or not, in the matcher
// as here.
func (r *Runner) plainPatternMatch(pattern, s string, surface patternSurface) (matched, ok bool) {
	if !plainPatternShortcut {
		return false, false
	}
	lit := pattern
	lead := strings.HasPrefix(lit, "*")
	if lead {
		lit = lit[1:]
	}
	trail := strings.HasSuffix(lit, "*") && !escapedAtEnd(lit[:len(lit)-1])
	if trail {
		lit = lit[:len(lit)-1]
	}
	// inner is set when the literal has stars inside it as well, which is
	// what a table search like `${out[(i)<*>_${k}]}` asks of every key.
	var inner bool
	if strings.IndexByte(lit, '\\') >= 0 {
		var ok bool
		if lit, ok = unescapePlainPattern(lit, r.sem().PatternEscapeReaches); !ok {
			return false, false
		}
	} else if strings.ContainsAny(lit, plainPatternSpecials) {
		var ok bool
		if inner, ok = innerStarsOnly(lit); !ok {
			return false, false
		}
	}
	// A literal that is not whole characters is left to the matcher, which
	// is what knows whether the units are characters: there a lone lead
	// byte is never the start of a character the subject holds, so
	// `[[ é == $'\xc3'* ]]` is false where a byte comparison says true.
	// Whole characters line up with the subject's own, since a character's
	// first byte never occurs inside another one.
	if !utf8.ValidString(lit) {
		return false, false
	}
	if r.MatchOption(MatchFoldsCase) {
		return false, false
	}
	if (lead || trail || inner) && surface == patternInACondition && r.recordsAPatternMatch() {
		return false, false
	}
	if inner {
		return matchStarSegments(s, lit, lead, trail), true
	}
	switch {
	case lead && trail:
		return strings.Contains(s, lit), true
	case lead:
		return strings.HasSuffix(s, lit), true
	case trail:
		return strings.HasPrefix(s, lit), true
	}
	return s == lit, true
}

// innerStarsOnly reports whether the specials in a literal with no
// backslash are stars with text on both sides of each, and angle brackets
// that cannot open a numeric range — and so whether lit is still a string to
// compare, in pieces. inner says a star was among them.
//
// A star beside another star is left to the matcher, which is the one that
// knows what a doubled star means where it means anything. An angle bracket
// is the bracket a numeric range is written in and nothing else to the
// matcher: `<` opens one only where `<`, digits, `-`, digits and `>` follow
// in that order (splitNumericRange), and `>` never opens anything. Every
// other `<` is a character, as `<*>_default` is a `<`, anything, and
// `>_default` — the key F-Sy-H's theme loader searches its table with.
func innerStarsOnly(lit string) (inner, ok bool) {
	for i := 0; i < len(lit); i++ {
		switch c := lit[i]; {
		case c == '*':
			if i == 0 || i == len(lit)-1 || lit[i+1] == '*' {
				return false, false
			}
			inner = true
		case c == '<':
			if opensNumericRange(lit[i:]) {
				return false, false
			}
		case c == '>':
		case strings.IndexByte(plainPatternSpecials, c) >= 0:
			return false, false
		}
	}
	return inner, true
}

// opensNumericRange reports whether p starts with the shape of a numeric
// range, `<n-m>` with either bound left out. It reads the shape only, so a
// bound too large to hold — which the matcher reads as text — is still
// counted here, and the pattern goes to the matcher, which is the safe side.
func opensNumericRange(p string) bool {
	i := 1
	for i < len(p) && isDigit(p[i]) {
		i++
	}
	if i >= len(p) || p[i] != '-' {
		return false
	}
	i++
	for i < len(p) && isDigit(p[i]) {
		i++
	}
	return i < len(p) && p[i] == '>'
}

// matchStarSegments is a whole-subject match of a literal whose pieces are
// joined by stars, with a star in front of it and after it where lead and
// trail say so. The first piece is a prefix unless a star leads, the last a
// suffix unless one trails, and each piece between is found leftmost in what
// is left, which is the whole answer for stars and text alone: taking a
// piece any later only leaves the pieces after it less room.
func matchStarSegments(s, lit string, lead, trail bool) bool {
	pieces := strings.Split(lit, "*")
	if !lead {
		if !strings.HasPrefix(s, pieces[0]) {
			return false
		}
		s = s[len(pieces[0]):]
		pieces = pieces[1:]
	}
	if !trail {
		last := pieces[len(pieces)-1]
		if !strings.HasSuffix(s, last) {
			return false
		}
		s = s[:len(s)-len(last)]
		pieces = pieces[:len(pieces)-1]
	}
	for _, piece := range pieces {
		i := strings.Index(s, piece)
		if i < 0 {
			return false
		}
		s = s[i+len(piece):]
	}
	return true
}

// escapedAtEnd reports whether the byte after text would be escaped: whether
// text ends in an odd run of backslashes. A trailing `\*` is a literal star,
// not the star the shortcut reads as "anything after".
//
// Parity is the whole of the rule, because every dialect's escape reaches a
// backslash — the empty set and zsh's set both hold one — so `\\*` is an
// escaped backslash and then a live star.
func escapedAtEnd(text string) bool {
	n := 0
	for n < len(text) && text[len(text)-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}

// unescapePlainPattern is the literal a pattern of escapes and ordinary
// characters stands for, and false for any other pattern.
//
// This is the shape a quoted operand reaches the matcher in. `[[ $x ==
// "$y" ]]`, `case $w in '(')`, and every quoted metacharacter come to the
// matcher with a backslash in front of each metacharacter, so a value like
// `builtin autoload -XU` arrives as `builtin autoload \-XU`. Measured on
// F-Sy-H's highlighter (#6423), 82 of the 155 patterns per call that reached
// the full matcher were these, every one of them a string to compare.
//
// The reading is the matcher's own escape step, which compares the escaped
// byte with one byte of the subject: where reaches holds the character, or
// is empty and so holds every character, the backslash is spent and the
// character is literal. Where it does not, the backslash is a character of
// its own and the next one is read on its own turn — zsh's `bet\a` is five
// characters. An escaped byte past ASCII is left to the matcher, which is
// what decides whether the escape took a byte or a character, and so is a
// backslash with nothing after it. An unescaped special refuses as before.
func unescapePlainPattern(lit, reaches string) (string, bool) {
	var b strings.Builder
	b.Grow(len(lit))
	for i := 0; i < len(lit); i++ {
		c := lit[i]
		if c != '\\' {
			if strings.IndexByte(plainPatternSpecials, c) >= 0 {
				return "", false
			}
			b.WriteByte(c)
			continue
		}
		if i+1 == len(lit) || lit[i+1] >= utf8.RuneSelf {
			return "", false
		}
		if reaches != "" && strings.IndexByte(reaches, lit[i+1]) < 0 {
			b.WriteByte('\\')
			continue
		}
		b.WriteByte(lit[i+1])
		i++
	}
	return b.String(), true
}
