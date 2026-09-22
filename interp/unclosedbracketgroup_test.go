// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A `[` that nothing closes takes the rest of the pattern with it, so the
// group it stands in is never closed and its parentheses are ordinary text.
//
// The scan looking for the `)` that closes a group steps over a bracket
// expression whole, because a `)` inside one is a member (#3075). What it did
// with a bracket that never closes was stand still, so the `)` behind it
// closed the group anyway and `@(ab|[)` matched `ab`.
//
// Measured 2026-09-22 on bash 5.3.20 with `extglob` and on ksh93u+, in a
// directory holding `ab`, `a)b`, `a]b`, `x`, `a[` and `abcx`. Both answer
// every row below the same way.
func TestAnUnclosedBracketSwallowsTheGroupsCloser(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"ab", "a)b", "a]b", "x", "a[", "abcx"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pattern, want string }{
		// Not a group, so the word is the six or more characters it is
		// written as, no name is that, and it comes back unexpanded.
		{`@(ab|[)`, `[@(ab|[)]`},
		{`abcx*(x[)`, `[abcx*(x[)]`},
		{`!([q)*`, `[!([q)*]`},
		{`+(a|c[)*`, `[+(a|c[)*]`},
		// The controls, which say this is the bracket reaching the end of
		// the pattern rather than any `[` poisoning a group: an escaped one
		// is not a bracket, a `]` alone is an ordinary character, and a
		// bracket that does close leaves the group a group.
		{`@(a\[)`, `[a[]`},
		{`abcx*(x])`, `[abcx]`},
		{`@(a[b])`, `[ab]`},
	} {
		if got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

// And the other half of the same rule: a bracket that *does* close may still
// have taken a `)` on the way, and the group then closes at a later one.
//
// These two cannot be asked through pathname expansion, because bash's own
// command grammar refuses the word before any pattern is matched — so they
// are measured with `[[ $s == $p ]]`, where bash 5.3.20 and ksh93u+ both
// answer yes to the first two and no to the third.
func TestABracketMayTakeTheGroupsCloserAndStillClose(t *testing.T) {
	for _, tc := range []struct{ subject, pattern, want string }{
		// `[)]` is the one-member set `)`, so the body is `a[)]b`.
		{"a)b", `@(a[)]b)`, "yes"},
		// `[)b]` is the two-member set, so the arms are `a[)b]` and `x`.
		{"ab", `@(a[)b]|x)`, "yes"},
		{"x", `@(a[)b]|x)`, "yes"},
		{"a]b", `@(a[)b]|x)`, "no"},
	} {
		if got := matchWith(t, tc.subject, tc.pattern, true, false); got != tc.want {
			t.Errorf("%q ~ %q = %q, want %q", tc.subject, tc.pattern, got, tc.want)
		}
	}
}
