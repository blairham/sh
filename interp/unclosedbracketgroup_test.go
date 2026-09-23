// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
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
// **The pattern has to reach the matcher from a value**, and this test used to
// say so and then not do it. Its own comment named `[[ $s == $p ]]` — the route
// where no word scan ever sees the pattern — while the helper it called wrote
// the pattern into a `case` arm, which is a route the references refuse: with
// `extglob` on, `case x in @(a[)]b))` is `syntax error near unexpected token
// ')'` in bash 5.3.20 and `')' unexpected` in ksh93u+, so the rows could not
// have been measured through it. The claim was right and the instrument was
// not, and it is what made this test fail when the word scanner learned the
// refusal (#4197).
//
// Re-measured 2026-09-23 through the route the comment names, and the two
// references agree on every row: yes, yes, yes, no.
func TestABracketMayTakeTheGroupsCloserAndStillClose(t *testing.T) {
	for _, tc := range []struct{ subject, pattern, want string }{
		// `[)]` is the one-member set `)`, so the body is `a[)]b`.
		{"a)b", `@(a[)]b)`, "yes"},
		// `[)b]` is the two-member set, so the arms are `a[)b]` and `x`.
		{"ab", `@(a[)b]|x)`, "yes"},
		{"x", `@(a[)b]|x)`, "yes"},
		{"a]b", `@(a[)b]|x)`, "no"},
	} {
		if got := matchFromAValue(t, tc.subject, tc.pattern); got != tc.want {
			t.Errorf("%q ~ %q = %q, want %q", tc.subject, tc.pattern, got, tc.want)
		}
	}
}

// And the word scanner's side of the same text, which is the refusal both
// references make and this shell did not.
//
// Measured 2026-09-23 with `extglob` on: a group closed only by a parenthesis
// standing inside a bracket expression is `syntax error near unexpected token
// ')'` in bash 5.3.20 and `')' unexpected` in ksh93u+, in command position and
// in a `case` arm alike. zsh 5.9.2 refuses its own spelling of it too, so all
// three columns agree and this is core rather than an axis.
//
// The bracket still protects the word-ending operators, which is the control:
// `x@([;])y` matches `x;y` in both references and here, and the same for `<`,
// `>` and `&`. So it is the parentheses alone that the bracket does not hold.
func TestAGroupClosedInsideABracketIsRefusedInAWord(t *testing.T) {
	for _, pattern := range []string{`@(a[)]b)`, `!(a[)]b)`, `@(a[)b]|x)`} {
		if got := matchWith(t, "a)b", pattern, true, false); !strings.HasPrefix(got, "parse:") {
			t.Errorf("%s = %q, want a parse failure — the group closes inside the bracket", pattern, got)
		}
	}
	// The control, through the same helper: a bracket with no parenthesis in it
	// leaves the group a group.
	for _, tc := range []struct{ subject, pattern, want string }{
		{"x;y", `x@([;])y`, "yes"},
		{"x<y", `x@([<])y`, "yes"},
		{"x&y", `x@([&])y`, "yes"},
		{"x>y", `x@([>])y`, "yes"},
		{"ab", `@(a[b])`, "yes"},
	} {
		if got := matchWith(t, tc.subject, tc.pattern, true, false); got != tc.want {
			t.Errorf("%q ~ %q = %q, want %q", tc.subject, tc.pattern, got, tc.want)
		}
	}
}

// matchFromAValue matches with the **pattern in a variable**, so no word scan
// ever reads it: `[[ $s == $p ]]` is the one route a pattern the command
// grammar refuses can still reach the matcher by.
func matchFromAValue(t *testing.T, subject, pattern string) string {
	t.Helper()
	d := syntax.Core()
	d.ExtendedPattern, d.DoubleBracket = true, true
	src := "s=" + singleQuotedForTest(subject) + "; p=" + singleQuotedForTest(pattern) +
		`; if [[ $s == $p ]]; then echo yes; else echo no; fi`
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	// A pattern that reaches the matcher from a value is the whole route, so the
	// axis that says whether such a result *is* group syntax has to be answered
	// or there is nothing to match with. Both references read it, which is what
	// the rows below are measured against.
	s.ExpansionResultSuppliesGroupSyntax = Yes
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// singleQuotedForTest wraps text so the shell reads it literally, which is what
// keeps the pattern out of every scan but the matcher's.
func singleQuotedForTest(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'''`) + "'"
}
