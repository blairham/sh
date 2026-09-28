// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// standingBackslash runs one condition with the two readings of a backslash
// written inside double quotes in a `=~` operand.
//
// `RegexQuotingMakesLiteral` is pinned **No** in both columns, and that is
// the whole reason these rows can be read at all: with it Yes the caller
// discards the reading this axis builds and hands the engine a wholly
// escaped run instead, so every row would answer bash's way whatever this
// axis said. `RegexKeepsAWrittenBackslash` is pinned No for the same reason
// one span over — it is a different span, and a sweep that left it to the
// vector would be measuring whichever of the two was armed.
func standingBackslash(t *testing.T, src string, stands Answer) string {
	t.Helper()
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.DoubleBracket = true
	}, func(r *Runner) {
		s := *r.Semantics
		s.RegexQuotingMakesLiteral = No
		s.RegexKeepsAWrittenBackslash = No
		s.RegexDigitClassEscape = No
		s.RegexDoubleQuotedBackslashStands = stands
		r.Semantics = &s
	})
	return strings.TrimSpace(out)
}

// A backslash written inside double quotes in a `=~` operand is either a
// character of its own or a quote the shell takes off.
//
// **What stands behind it keeps whatever it meant**, which is the half that
// is easy to state wrongly. The pair is not two literal characters: `"a\+b"`
// is a literal backslash with a `+` *quantifying it*. The `+` rows are what
// separate that from "both characters literal", and without them the two
// readings agree everywhere — which is why they are here and why the issue's
// own phrasing needed correcting against them.
//
// Measured 2026-09-28 — see Semantics.RegexDoubleQuotedBackslashStands for
// the whole panel and the binaries.
func TestADoubleQuotedBackslashEitherStandsOrIsAQuote(t *testing.T) {
	for _, tc := range []struct{ name, src, stands, quote string }{
		// The control: with no backslash in it, a quoted run is still an
		// expression under both readings, so the operand is read at all.
		{
			"the control", `[[ axb =~ "a.b" ]] && echo match || echo no`,
			"match", "match",
		},

		// A quoted metacharacter. Standing, the engine gets a literal
		// backslash and a live `.`; as a quote, it gets a literal `.`.
		{
			"a dot, against a dotted subject",
			`[[ "a.b" =~ "a\.b" ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"and against a backslashed one",
			`[[ 'a\.b' =~ "a\.b" ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"and against neither",
			`[[ axb =~ "a\.b" ]] && echo match || echo no`,
			"no", "no",
		},

		// **The `+` rows.** Standing, the `+` quantifies the backslash, so
		// the pattern is one or more backslashes and matches none of `a+b`
		// nor its own text. As a quote, it is a literal `+`.
		{
			"a plus, against a plus",
			`[[ "a+b" =~ "a\+b" ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"against one backslash",
			`[[ 'a\b' =~ "a\+b" ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"against two",
			`[[ 'a\\b' =~ "a\+b" ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"and against its own text",
			`[[ 'a\+b' =~ "a\+b" ]] && echo match || echo no`,
			"no", "no",
		},

		// A letter behind the backslash, which is the issue's own row.
		{
			"a d, against a d", `[[ zadb =~ "za\db" ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"and against a backslashed one",
			`[[ 'za\db' =~ "za\db" ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"and against a digit",
			`[[ za1b =~ "za\db" ]] && echo match || echo no`,
			"no", "no",
		},

		// A **backslash-quoted** span is a different question and neither
		// reading here touches it: both columns answer the same. #4976.
		{
			"a backslash-quoted span is not this",
			`[[ axb =~ a\.b ]] && echo match || echo no`,
			"match", "match",
		},
	} {
		if got := standingBackslash(t, tc.src, Yes); got != tc.stands {
			t.Errorf("%s standing: %s = %q, want %q", tc.name, tc.src, got, tc.stands)
		}
		if got := standingBackslash(t, tc.src, No); got != tc.quote {
			t.Errorf("%s as a quote: %s = %q, want %q", tc.name, tc.src, got, tc.quote)
		}
	}
}

// The whole-run reading wins over this one, which is what keeps the column
// that has it from being moved by an axis it answers No.
//
// With `RegexQuotingMakesLiteral` Yes the caller discards the reading this
// axis builds, so both of its values answer the same — and that answer is
// the whole quoted run as text, which is neither of the two above.
func TestTheWholeRunReadingWinsOverAStandingBackslash(t *testing.T) {
	run := func(src string, stands Answer) string {
		t.Helper()
		out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
			d.DoubleBracket = true
		}, func(r *Runner) {
			s := *r.Semantics
			s.RegexQuotingMakesLiteral = Yes
			s.RegexKeepsAWrittenBackslash = No
			s.RegexDigitClassEscape = No
			s.RegexDoubleQuotedBackslashStands = stands
			r.Semantics = &s
		})
		return strings.TrimSpace(out)
	}
	for _, tc := range []struct{ name, src, want string }{
		// The whole run is text, so a `.` inside quotes is a `.` — which is
		// the row that says this column is on the other reading entirely.
		{"a quoted dot is text", `[[ axb =~ "a.b" ]] && echo match || echo no`, "no"},
		{"its own text matches", `[[ 'a\+b' =~ "a\+b" ]] && echo match || echo no`, "match"},
		{"and a backslash is text too", `[[ 'za\db' =~ "za\db" ]] && echo match || echo no`, "match"},
	} {
		for _, stands := range []Answer{Yes, No} {
			if got := run(tc.src, stands); got != tc.want {
				t.Errorf("%s at stands=%v: %s = %q, want %q", tc.name, stands, tc.src, got, tc.want)
			}
		}
	}
}
