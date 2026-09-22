// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// hiddenDir is one hidden name and one plain one, which is the whole of what
// the leading-period rule is about.
func hiddenDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{".hidden", "plain"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A pattern matches a leading period only if it explicitly begins with one —
// and where it begins with a group, every alternative of that group is a
// place it could begin.
//
// Every row is a measurement on zsh 5.9.2 with `extendedglob` on, in exactly
// the directory hiddenDir builds. See interp/globhidden.go for the reasoning
// and for what reads only the pattern's first byte gets wrong.
func TestAGroupsAlternativesCanBeginWithAPeriod(t *testing.T) {
	dir := hiddenDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a plain period", `printf "[%s]" .hidden(#qN)`, "[.hidden]"},
		{"an escaped one", `printf "[%s]" \.hidden(#qN)`, "[.hidden]"},
		{"a period in front of a group", `printf "[%s]" .(hidden|x)(#qN)`, "[.hidden]"},
		{"the first alternative of a group", `printf "[%s]" (.hidden|plain)(#qN)`, "[.hidden][plain]"},
		{"and a later one", `printf "[%s]" (x|.hidden)(#qN)`, "[.hidden]"},
		{"an alternative that is only the period", `printf "[%s]" (.|x)hidden(#qN)`, "[.hidden]"},
		{"a group of one", `printf "[%s]" (.hidden)(#qN)`, "[.hidden]"},
		{"a group inside a group", `printf "[%s]" ((.hidden))(#qN)`, "[.hidden]"},
		{"an alternative that goes on to be a pattern", `printf "[%s]" (.h*)(#qN)`, "[.hidden]"},
		{"past an empty alternative", `printf "[%s]" (|.hidden)(#qN)`, "[.hidden]"},
		{"past a pattern-flag group", `printf "[%s]" (#i)(.HIDDEN|x)(#qN)`, "[.hidden]"},
		{"with more pattern after the group", `printf "[%s]" (.hidden|plain)*(#qN)`, "[.hidden][plain]"},
		// A bracket expression inside an alternative is one member after
		// another, so the `|` in it is a member and not a split — without
		// that the text behind the bar read as an alternative of its own,
		// and a period at the front of it made the pattern look explicit.
		// Measured on zsh 5.9.2 in this directory: both are `[]` (#3075).
		{"a bar inside a bracket is not a split", `printf "[%s]" ([a|.]hidden)(#qN)`, "[]"},
		{"nor is it in a later alternative", `printf "[%s]" (x|[a|.]hidden)(#qN)`, "[]"},
		// The three that say this is "is one written there" rather than
		// "could this pattern match one".
		{"a bracket is not explicit", `printf "[%s]" [.]hidden(#qN)`, "[]"},
		{"nor is a metacharacter", `printf "[%s]" ?hidden(#qN)`, "[]"},
		{"which is the rule's whole point", `printf "[%s]" *(#qN)`, "[plain]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runCondition(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A parenthesis inside a bracket expression does not end the group (#3075).
//
// The other half of the same blind spot, and the half that decides whether
// the leading-period question is asked at all: the scan looking for the `)`
// that closes a group counted the ones inside the bracket, so `([(]|.hidden)`
// had no end, the pattern was judged not to begin with a period, and the
// dotfile its second alternative names was passed over.
//
// The pattern is carried in a variable rather than written, because the shell
// this is measured from will not read a `(#q…)` group after a literal one —
// `([(]|.hidden)(#qN)` is `no matches found` there, with the qualifier group
// still in the text it prints back. Through a variable it answers cleanly.
// Measured 2026-09-15 in exactly the directory hiddenDir builds.
func TestAParenthesisInsideABracketDoesNotEndTheGroup(t *testing.T) {
	dir := hiddenDir(t)
	for _, tc := range []struct{ name, pattern, want string }{
		{"a hidden name behind the bracket", "([(]|.hidden)", "[.hidden]"},
		{"a plain one behind it", "([(]|plain)", "[plain]"},
		{
			// The control that says the group was found rather than the
			// period simply being taken on trust: the same shape with no
			// bracket in it answered this all along.
			"no bracket at all", "(x|.hidden)", "[.hidden]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `p='` + tc.pattern + `'; printf "[%s]" ${~p}(#qN)`
			out, st := runCondition(t, dir, src)
			if out != tc.want || st != 0 {
				t.Errorf("%s: got %q (status %d), want %q at 0", tc.pattern, out, st, tc.want)
			}
		})
	}
}

// quantifiedDir is the directory the rows below are measured in: two hidden
// names, a plain one, and a one-character plain one so a `?` arm has
// something to reach.
func quantifiedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{".a", ".b", ".foo", "bar", "x"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The same question for the dialect with *quantified* groups, where the
// quantifier decides two things the bare group never had to answer.
//
// Every row is measured 2026-09-22 on bash 5.3.20 with `extglob` and on
// ksh93u+, in exactly the directory quantifiedDir builds, and the two agree
// row for row. See interp/globhidden.go for the reading.
func TestAQuantifiedGroupsArmsCanBeginWithAPeriod(t *testing.T) {
	dir := quantifiedDir(t)
	for _, tc := range []struct{ pattern, want string }{
		// An arm may begin with one, under any quantifier but `!`.
		{`@(.foo)`, `[.foo]`},
		{`+(.foo)`, `[.foo]`},
		{`*(.foo)`, `[.foo]`},
		{`?(.foo)`, `[.foo]`},
		{`@(x|.foo)`, `[.foo][x]`},
		{`@(.*)`, `[.a][.b][.foo]`},
		{`@(@(.foo))`, `[.foo]`},
		{`@(\.a)`, `[.a]`},
		// A closure may draw nothing, so a period standing behind one is
		// still the first thing the pattern writes. `@` and `+` must draw
		// something and are not stepped over.
		{`*(bar).foo`, `[.foo]`},
		{`?(bar).foo`, `[.foo]`},
		{`+(bar).foo`, `[+(bar).foo]`},
		{`@(bar).foo`, `[@(bar).foo]`},
		{`*(x)@(.foo)`, `[.foo]`},
		// Whether the group can draw nothing is the quantifier's answer and
		// not its arms': `@(x|)` matches the empty string and still refuses
		// the period behind it, where `*(x|y)` matches no empty arm and
		// allows it. The pair is what says this is a rule about which bytes
		// are written.
		{`@(x|).foo`, `[@(x|).foo]`},
		{`*(x|y).foo`, `[.foo]`},
		// `!` neither lends its arms nor steps aside, though it plainly
		// matches the empty string.
		{`!(bar).foo`, `[!(bar).foo]`},
		{`!(.foo)`, `[bar][x]`},
		// And a bracket is still not explicit, inside a group as out.
		{`@([.])a`, `[@([.])a]`},
	} {
		if got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

// The other half of the rule, which is the *name's* rather than the
// pattern's: a period the subject begins with has to be taken by a period the
// pattern wrote, on the branch that matched.
//
// Being offered the hidden names is not the same as matching them. A pattern
// is offered them because some place it could start writes a period; the arm
// that reaches a given name still has to. Measured 2026-09-22 in exactly the
// directory quantifiedDir builds, where bash 5.3.20 and ksh93u+ agree row for
// row: `@(.foo|*)` reaches `bar` through its `*` and does not reach `.a`, and
// `@(?|.?)` reaches `.a` and `.b` through `.?` and the plain `x` through `?`.
func TestALeadingPeriodIsTakenOnlyByAWrittenPeriod(t *testing.T) {
	dir := quantifiedDir(t)
	for _, tc := range []struct{ pattern, want string }{
		// The star arm is not a written period, so it reaches the plain
		// names and stops.
		{`@(.foo|*)`, `[.foo][bar][x]`},
		{`*(.foo|*)`, `[.foo][bar][x]`},
		// Nor is `?`, nor a bracket, nor a negation.
		{`@(?|.?)`, `[.a][.b][x]`},
		{`@([.]a|.b)`, `[.b]`},
		{`@(!(z)|.b)`, `[.b][bar][x]`},
		// A written period does take it, wherever the arm stands.
		{`@(.a|.b)`, `[.a][.b]`},
		{`*(.).a`, `[.a]`},
	} {
		if got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

// The same half, in the dialect with *bare* groups, because the rule lives in
// the matcher and both dialects reach it.
//
// Measured 2026-09-22 on zsh 5.9.2 with `extendedglob`, in a directory
// holding `.a`, `.b` and `plain`.
func TestABareGroupsArmStillHasToWriteThePeriod(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{".a", ".b", "plain"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pattern, want string }{
		{`(.a|*)`, "[.a][plain]"},
		{`(?|.?)`, "[.a][.b]"},
		{`([.]a|.b)`, "[.b]"},
	} {
		src := `p='` + tc.pattern + `'; printf "[%s]" ${~p}(#qN)`
		out, st := runCondition(t, dir, src)
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q at 0", tc.pattern, out, st, tc.want)
		}
	}
}
