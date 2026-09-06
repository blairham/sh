// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// splitGlobQualifiers as a function, for the two rules the shell surface
// cannot reach: an empty group and a backslash run.
//
// `f1()` never arrives here, because the lexer refuses to take `()` into a
// word at all — `echo *()` is a syntax error in every dialect. And a run of
// backslashes is not spellable as a field either, the escaping being added by
// the expander rather than written. Both are still rules this function has,
// and a rule nothing asserts is a rule that quietly changes.
func TestSplittingATrailingQualifierList(t *testing.T) {
	for _, tc := range []struct {
		field, pattern, list string
		ok                   bool
	}{
		{`*(.)`, `*`, `.`, true},
		{`f1(.)`, `f1`, `.`, true},
		{`(.)`, ``, `.`, true},
		{`*(D.)`, `*`, `D.`, true},
		{`*(b(c))`, `*`, `b(c)`, true},
		// A group holding an alternation is that alternation.
		{`f(1|2)`, `f(1|2)`, ``, false},
		// An empty group is not a list. Left as it is rather than read as a
		// list of nothing, which every file would pass.
		{`f1()`, `f1()`, ``, false},
		// Not at the end of the word.
		{`*(.)x`, `*(.)x`, ``, false},
		{`*`, `*`, ``, false},
		{``, ``, ``, false},
		{`)`, `)`, ``, false},
		// Escaped parentheses are not a group: one backslash protects, two
		// are a backslash that does not, three protect again. The odd/even
		// count is the rule, not "is there a backslash".
		{`f1\(.\)`, `f1\(.\)`, ``, false},
		{`f1\\(.)`, `f1\\`, `.`, true},
		{`f1\\\(.\)`, `f1\\\(.\)`, ``, false},
		// The two ends are asked separately, and each on its own: an
		// escaped opener with a live closer is no group, and a live group
		// with an escaped `)` after it is not at the end of the word.
		{`*\(x)`, `*\(x)`, ``, false},
		{`*(.)x\)`, `*(.)x\)`, ``, false},
	} {
		pattern, list, ok := splitGlobQualifiers(tc.field)
		if pattern != tc.pattern || list != tc.list || ok != tc.ok {
			t.Errorf("splitGlobQualifiers(%q) = %q, %q, %v; want %q, %q, %v",
				tc.field, pattern, list, ok, tc.pattern, tc.list, tc.ok)
		}
	}
}

// hasUnescapedMeta's answer for a parenthesized group, which is the rule that
// makes `echo f(1|2)` a pattern with no `*` in it.
//
// An *unclosed* `(` is an ordinary character, the same rule an unterminated
// bracket expression follows and for the same reason: a field that is not a
// pattern must not be reported as a pattern that matched nothing. And the
// whole question is a dialect's — in the four shells without bare groups a
// `(` in a field never reaches a pattern at all.
func TestAGroupIsAMetacharacterOnlyWhereItCloses(t *testing.T) {
	for _, tc := range []struct {
		field                    string
		patternGroup, wantIsMeta bool
	}{
		{`f(1|2)`, true, true},
		{`(x)`, true, true},
		{`a(b(c))`, true, true},
		// Unclosed: an ordinary character.
		{`a(b`, true, false},
		// Asked of each `(` in turn rather than of the first one, so an
		// outer that never closes does not hide an inner that does. The
		// lexer refuses this shape before a field can hold it — the group
		// scan counts depth and runs out — so this is the function's rule
		// rather than a reachable answer.
		{`a(b(c)`, true, true},
		// Escaped: an ordinary character too, which is what keeps a quoted
		// group and one from a value literal.
		{`a\(b\)`, true, false},
		{`f(1|2)`, false, false},
		{`a(b`, false, false},
		// And the rest of the metacharacters are unaffected either way.
		{`a*b`, false, true},
		{`a?b`, true, true},
	} {
		if got := hasUnescapedMeta(tc.field, false, tc.patternGroup); got != tc.wantIsMeta {
			t.Errorf("hasUnescapedMeta(%q, false, %v) = %v, want %v",
				tc.field, tc.patternGroup, got, tc.wantIsMeta)
		}
	}
}
