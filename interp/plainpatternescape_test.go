// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A quoted operand is a string to compare. F-Sy-H's lifecycle check runs
// `[[ ${functions[$name]-} == "${applied[$name]-}" ]]` over every autoload
// name on every highlight, and the value `builtin autoload -XU` reaches the
// matcher as `builtin autoload \-XU`. That, and the quoted `(`, `]]` and
// `<<<` its highlighter compares words against, were 82 of the 155 patterns
// per highlight call that paid for the full matcher's policies (#6423).
//
// The answers themselves are TestEscapedPatternsAnswerAsTheMatcherDoes's;
// this holds that the shortcut is the one giving them.
func TestAnEscapedLiteralIsAnsweredWithoutTheMatcher(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	for _, c := range []struct {
		reaches          string
		pattern, subject string
		want, ok         bool
	}{
		{"", `builtin autoload \-XU`, "builtin autoload -XU", true, true},
		{"", `\(`, "(", true, true},
		{"", `\]\]`, "]]", true, true},
		{"", `\<\<\<`, "<<<", true, true},
		{"", `\-\-*`, "--x", true, true},
		{"", `*\*`, "a*", true, true},
		{"", `*\*`, "a", false, true},
		{"", `bet\a`, "beta", true, true},
		{"-=!*?[]()|^~#<>\\", `bet\a`, `bet\a`, true, true},
		{"-=!*?[]()|^~#<>\\", `bet\a`, "beta", false, true},
		{"-=!*?[]()|^~#<>\\", `\\*`, `\x`, true, true},
		// Still the matcher's: a live special, a backslash with nothing
		// after it, an escaped byte past ASCII, and a special the escape
		// does not reach in this dialect.
		{"", `\(*)`, "(x)", false, false},
		{"", `a\`, `a\`, false, false},
		{"", "\\\xc3\xa9", "é", false, false},
		{"-=!*?[]()|^~#<>\\", `\+`, `\+`, false, false},
	} {
		sem := PosixSemantics()
		sem.PatternEscapeReaches = c.reaches
		// testrunner:bare — the shortcut is the subject; nothing here runs
		// a command, opens a file or reaches a directory.
		r := &Runner{Semantics: &sem, Dialect: &d}
		got, ok := r.plainPatternMatch(c.pattern, c.subject, patternInACondition)
		if ok != c.ok || got != c.want {
			t.Errorf("escapes %q: %q against %q: got (%v, %v), want (%v, %v)",
				c.reaches, c.pattern, c.subject, got, ok, c.want, c.ok)
		}
	}
}
