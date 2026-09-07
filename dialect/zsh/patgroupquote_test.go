// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A quoted or escaped character inside a pattern group is text, run rather
// than only parsed (#1248).
//
// Behavioral because parsing is half the answer and the quieter half is the
// other one: a group scanned as raw text parses and then hands the matcher
// the quotes, so `[[ b == ("b") ]]` asks whether `b` is three characters and
// says no, at status 1, with nothing said anywhere.
func TestAQuotedOperatorInsideAPatternGroupIsText(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The filed shape: the escape and the two quotings of it.
		{`[[ "<x" == (\<)* ]] && echo Y || echo N`, "Y"},
		{`[[ "<x" == ("<")* ]] && echo Y || echo N`, "Y"},
		{`[[ "<x" == ('<')* ]] && echo Y || echo N`, "Y"},
		{`[[ ">x" == (\>)* ]] && echo Y || echo N`, "Y"},
		// The other two word-ending operators, which share the clause.
		{`[[ 'a;b' == (a\;b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a&b' == (a\&b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a;b' == ("a;b") ]] && echo Y || echo N`, "Y"},
		// The group's own delimiters and its alternation bar, which the
		// matcher has to be told about too: an escaped `|` is not an arm
		// boundary.
		{`[[ 'a)b' == (a\)b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a(b' == (a\(b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a|b' == (a\|b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a|b' == ("a|b") ]] && echo Y || echo N`, "Y"},
		// Quoted text inside a group is *literal*, which is the other
		// direction and the one a scan that merely got past the parser
		// would fail: a quoted `*` matches a star and nothing else.
		{`[[ b == ("b") ]] && echo Y || echo N`, "Y"},
		{`[[ b == (\b) ]] && echo Y || echo N`, "Y"},
		{`[[ b == (a|"b") ]] && echo Y || echo N`, "Y"},
		{`[[ 'a*b' == (a"*"b) ]] && echo Y || echo N`, "Y"},
		{`[[ axb == (a"*"b) ]] && echo Y || echo N`, "N"},
		{`[[ '<1-9>' == ("<1-9>") ]] && echo Y || echo N`, "Y"},
		{`[[ 5 == ("<1-9>") ]] && echo Y || echo N`, "N"},
		// `$'…'` is a quoting form too, and reaches the same span.
		{`[[ "<x" == ($'\x3c')* ]] && echo Y || echo N`, "Y"},
		// Nested, and inside a range's own delimiters.
		{`[[ "<x" == ((\<))* ]] && echo Y || echo N`, "Y"},
		{`[[ "<1-2>" == (\<1-2\>) ]] && echo Y || echo N`, "Y"},
		// Bare, the four are still operators — the fix is about quoting and
		// not about handing the group the characters. Each of these is a
		// parse error in zsh 5.9.2 and has to stay one here, so it is asked
		// through an `eval`, where a refusal is a status rather than a
		// failure to read the test's own source.
		{`eval '[[ "<x" == (<)* ]]' 2>/dev/null; echo st=$?`, "st=1"},
		{`eval '[[ ">x" == (>)* ]]' 2>/dev/null; echo st=$?`, "st=1"},
		// And in the *middle* of a word too, which is neither the route nor
		// the position the rule used to be written as: `a(b<c)` is a parse
		// error in zsh 5.9.2, and reading the `<` into the group matched
		// `ab<c` against it here.
		{`eval '[[ "ab<c" == a(b<c) ]]' 2>/dev/null; echo st=$?`, "st=1"},
		{`eval '[[ "ab;c" == a(b;c) ]]' 2>/dev/null; echo st=$?`, "st=1"},
		{`[[ 'ab<c' == a(b\<c) ]] && echo Y || echo N`, "Y"},
		{`[[ ab == a(b|c) ]] && echo Y || echo N`, "Y"},
		{`[[ a5 == a(<0-9>) ]] && echo Y || echo N`, "Y"},
		{`eval '[[ "<x" == (\<)* ]]' 2>/dev/null; echo st=$?`, "st=0"},
		// And a bare range inside a group still works, which is the flag
		// the `<` cases have to stay clear of (#1217).
		{`[[ 5x == (<0-9>)* ]] && echo Y || echo N`, "Y"},
		// A line continuation is removed inside a group as it is anywhere
		// else, so the group is `(ab)` and matches `ab`. The backslash case
		// beside it goes the other way and keeps what it protects, which is
		// why this is written out: reading the newline as protected makes
		// the pattern `a`, newline, `b`.
		{"k=ab; [[ $k == (a\\\nb) ]] && echo Y || echo N", "Y"},
		// The controls: a bare escape outside a group, and a bare group.
		{`[[ "<x" == \<* ]] && echo Y || echo N`, "Y"},
		{`[[ ax == (a|b)* ]] && echo Y || echo N`, "Y"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The same group, reached by the other two routes: a `case` arm's pattern and
// an ordinary argument that globs. It is one scanner, and #1217 is the reason
// that is asserted rather than assumed.
func TestAQuotedOperatorInAGroupReachesEveryRoute(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`case "<x" in (\<)*) echo Y;; *) echo N;; esac`, "Y"},
		{`case "<x" in ("<")*) echo Y;; *) echo N;; esac`, "Y"},
		{`case b in (a|"b")) echo Y;; *) echo N;; esac`, "Y"},
		{`v="<x"; print -r -- ${v/(\<)/L}`, "Lx"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
