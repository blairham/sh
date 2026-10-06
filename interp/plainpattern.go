// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// plainPatternShortcut switches the answer below on. It is a variable so a
// test can drive the same patterns through the matcher and through the
// shortcut and require the same answers.
var plainPatternShortcut = true

// plainPatternSpecials are the bytes that can mean something to the matcher
// in some dialect under some option: the POSIX three, the escape, every
// character an extended or ksh-style pattern spells an operator with, and
// the bracket a numeric range is written in. A pattern that holds none of
// them, apart from a star at either end, is a string to compare.
const plainPatternSpecials = "*?[]\\()|<>^#~!@+"

// plainPatternMatch answers a whole-subject match for the patterns that need
// no matcher: a literal, `*`, and a literal with a star at one end or both.
// ok is false for every other pattern, which goes to the matcher as before.
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
// fire on a pattern without one. A star matches any bytes at all, valid
// characters or not, in the matcher as here.
func (r *Runner) plainPatternMatch(pattern, s string, surface patternSurface) (matched, ok bool) {
	if !plainPatternShortcut {
		return false, false
	}
	lit := pattern
	lead := strings.HasPrefix(lit, "*")
	if lead {
		lit = lit[1:]
	}
	trail := strings.HasSuffix(lit, "*")
	if trail {
		lit = lit[:len(lit)-1]
	}
	if strings.ContainsAny(lit, plainPatternSpecials) {
		return false, false
	}
	if r.MatchOption(MatchFoldsCase) {
		return false, false
	}
	if (lead || trail) && surface == patternInACondition && r.recordsAPatternMatch() {
		return false, false
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
