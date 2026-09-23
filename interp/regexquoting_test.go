// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// literalRegex answers the one axis these rows turn on the way the column
// that has the construct answers it: a quoted portion of a `=~` operand is
// text rather than expression.
func literalRegex() func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		sem.RegexQuotingMakesLiteral = Yes
		r.Semantics = &sem
	}
}

// TestAQuotedPortionOfARegexIsLiteralAndTheRestIsNot — quoting decides a
// `=~` operand a **span at a time**, exactly as it decides a glob operand.
//
// This used to ask the *word* whether anything in it was quoted and then
// escape the whole expanded value, so one quote anywhere turned every
// metacharacter in the operand into a letter. Measured against bash 5.3.20 on
// 2026-09-22: every `match` row below answered `no` before #4173.
func TestAQuotedPortionOfARegexIsLiteralAndTheRestIsNot(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The anchors on either side of a quoted letter are still anchors.
		{"anchors survive a quote", `[[ ab =~ ^"a"b$ ]] && echo match || echo no`, "match"},
		{"a whole quoted middle", `[[ ab =~ ^"ab"$ ]] && echo match || echo no`, "match"},
		// A backslash is a quote too, and it quotes its one character only.
		{"a backslash quotes one character", `[[ ab =~ ^\ab$ ]] && echo match || echo no`, "match"},
		{"a dot outside the quote is a dot", `[[ axb =~ "a".b ]] && echo match || echo no`, "match"},
		{"a star outside the quote repeats", `[[ aab =~ "a"a*b ]] && echo match || echo no`, "match"},
		{"a group outside the quote groups", `[[ ab =~ "a"(b) ]] && echo match || echo no`, "match"},
		// The controls, which are the rows a quote is *about*: inside the
		// quote a metacharacter is a letter, and that has not moved.
		{"a quoted dot is a dot", `[[ axb =~ "a.b" ]] && echo match || echo no`, "no"},
		{"and matches itself", `[[ a.b =~ "a.b" ]] && echo match || echo no`, "match"},
		{"a quoted plus is a plus", `[[ "a+b" =~ a"+"b ]] && echo match || echo no`, "match"},
		{"an unquoted plus repeats", `[[ aab =~ a"+"b ]] && echo match || echo no`, "no"},
		// An expansion is a span like any other: live unquoted, text quoted.
		{"an unquoted value is expression", `r='a.b'; [[ axb =~ $r ]] && echo match || echo no`, "match"},
		{"a quoted value is text", `r='a.b'; [[ axb =~ "$r" ]] && echo match || echo no`, "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, literalRegex())
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// TestARegexMatchIsLeftmostLongest — a POSIX regular expression matches the
// **longest** of the alternatives that can start at the leftmost position,
// which is not what Go's regexp package prefers by default.
//
// The whole match is read back through the capture record, so preferring the
// first alternative is a wrong value carried forward rather than a status:
// `[[ ab =~ a|ab ]]` holds either way and the recorded match was `a`.
// Measured against bash 5.3.20, 2026-09-22.
func TestARegexMatchIsLeftmostLongest(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The expression is held in a parameter because a bare `|` in a
		// condition operand is a token the core grammar does not take there,
		// and the alternation is the shape this is about.
		{"the longer alternative wins", `r='a|ab'; [[ ab =~ $r ]]; echo "[${M[0]}]"`, "[ab]"},
		{"however it is written", `r='ab|a'; [[ ab =~ $r ]]; echo "[${M[0]}]"`, "[ab]"},
		{"three of them", `r='a|aa|aaa'; [[ aaa =~ $r ]]; echo "[${M[0]}]"`, "[aaa]"},
		{"and the leftmost still wins over the longer", `r='x|aaa'; [[ xaaa =~ $r ]]; echo "[${M[0]}]"`, "[x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, rematch("M"))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}
