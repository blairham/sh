// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A caller holding a setting written as a pattern asks the shell what it
// means, rather than bringing its own matcher.
//
// The front end's history has a list of command lines not to record, and every
// shell that has such a list writes it as globs. Without this seam that caller
// would need a second matcher, and a shell whose `case` and whose settings
// disagreed about what `@(a|b)` means would be one thing pretending to be two.
func TestMatchPatternAnswersForASetting(t *testing.T) {
	sem := interp.PosixSemantics()
	r := newTestRunner(t, &interp.Runner{Semantics: &sem})
	for _, tc := range []struct {
		pattern, s string
		want       bool
	}{
		{pattern: "pwd", s: "pwd", want: true},
		// Anchored at both ends. A setting that matched anything containing
		// the pattern would drop `echo pwd` as well, which is the failure a
		// substring search would have.
		{pattern: "pwd", s: "pwd /tmp"},
		{pattern: "pwd", s: "echo pwd"},
		{pattern: "ls*", s: "ls -la", want: true},
		{pattern: "ls*", s: "lsof -h", want: true},
		{pattern: "ls*", s: "als"},
		{pattern: "*secret*", s: "export A_secret=1", want: true},
		{pattern: "?", s: "a", want: true},
		{pattern: "?", s: "ab"},
		{pattern: "[abc]x", s: "bx", want: true},
		{pattern: "[abc]x", s: "dx"},
		// A star crosses a slash, because this is a text pattern and not a
		// path one: `cd /a/b` has to be reachable from `cd *`.
		{pattern: "cd *", s: "cd /a/b", want: true},
		// An empty pattern matches only the empty string, which is what keeps
		// an empty setting from matching every line.
		{pattern: "", s: "", want: true},
		{pattern: "", s: "anything"},
	} {
		if got := r.MatchPattern(tc.pattern, tc.s); got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
		}
	}
}

// It is the *dialect's* pattern language, not one language for everyone.
//
// A bare group is a dialect's answer, so the shell that reads one reads it
// here too, and the shell that does not sees the same text as literal
// parentheses. That is the whole reason this hangs off a Runner rather than
// being a package function: a setting written as a pattern means what the
// shell it was written for means by it.
func TestMatchPatternFollowsTheDialect(t *testing.T) {
	sem := interp.PosixSemantics()

	off := syntax.Dialect{}
	plain := newTestRunner(t, &interp.Runner{Semantics: &sem, Dialect: &off})
	if plain.MatchPattern("(ls|pwd)", "ls") {
		t.Error("a shell without bare groups read one")
	}
	if !plain.MatchPattern("(ls|pwd)", "(ls|pwd)") {
		t.Error("a shell without bare groups did not read the parentheses literally")
	}

	on := syntax.Dialect{PatternAlternation: true}
	grouped := newTestRunner(t, &interp.Runner{Semantics: &sem, Dialect: &on})
	if !grouped.MatchPattern("(ls|pwd)", "ls") {
		t.Error("a shell with bare groups did not read one")
	}
	if grouped.MatchPattern("(ls|pwd)", "cat") {
		t.Error("a group matched something outside it")
	}
}
