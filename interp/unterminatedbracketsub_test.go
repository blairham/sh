// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bracket a sub-expression left open — `[[:alpha:]`, whose closing `]` is
// the class's own and not the bracket's. See
// Semantics.UnterminatedBracketAfterASubExpression. Which preset gives which
// answer is asserted in the dialect packages and never here.

// subBracketSem answers the two bracket axes independently, which is the
// whole point of the second one: one column reads a bare `[` as a literal
// character and this shape as no match at all, so a suite that moved them
// together could not tell the axes apart.
func subBracketSem(plain, sub BracketPolicy) Semantics {
	s := permissive()
	s.UnterminatedBracket = plain
	s.UnterminatedBracketAfterASubExpression = sub
	s.CollatingElements = OneCharacterIsACollatingElement
	s.UnterminatedCharacterClass = UnterminatedClassIsOrdinaryCharacters
	return s
}

// trimmedWith returns what a prefix trim leaves, so that *what* matched is
// visible as text rather than only as a yes or no — the instrument the panel
// rows were taken with.
func trimmedWith(t *testing.T, plain, sub BracketPolicy, pattern, subject string) string {
	t.Helper()
	src := `v='` + subject + `'; printf "[%s]" "${v#` + pattern + `}"`
	out, _ := run(t, src, withSem(subBracketSem(plain, sub)))
	return out
}

// The axis, and the reading each value gives.
//
// Measured 2026-09-16 under LC_ALL=C: bash 5.3.20 takes `[` plus one of
// `:alpha`'s five characters, ksh93u+ and dash 0.5.12 match nothing, and zsh
// 5.9.2 calls it no pattern at all.
func TestABracketLeftOpenByASubExpressionHasItsOwnAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, subject string
		literal, noMatch       string
	}{
		// The literal reading is `[` and then the ordinary set
		// `[:alpha:]` — `:`, `a`, `l`, `p`, `h` — so it takes exactly two
		// characters and only these two.
		{"a class, and a member of it", "[[:alpha:]", "[a", "[]", "[[a]"},
		{"a class, and the colon", "[[:alpha:]", "[:x", "[x]", "[[:x]"},
		{"a class, and a character outside it", "[[:alpha:]", "[x", "[[x]", "[[x]"},
		{"a class, and no leading bracket", "[[:alpha:]", "ab", "[ab]", "[ab]"},
		// The same shape reached through a collating element rather than a
		// class name.
		{"a collating element", "[[.a.]", "[a", "[]", "[[a]"},
		{"an equivalence class", "[[=a=]", "[a", "[]", "[[a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimmedWith(t, BracketLiteral, BracketLiteral, tc.pattern, tc.subject); got != tc.literal {
				t.Errorf("literal: got %s, want %s", got, tc.literal)
			}
			if got := trimmedWith(t, BracketLiteral, BracketNoMatch, tc.pattern, tc.subject); got != tc.noMatch {
				t.Errorf("no match: got %s, want %s", got, tc.noMatch)
			}
		})
	}
}

// matchesWith answers a `case` arm, which is the surface that asks *both*
// bracket axes — the trim above is on the path where a bare `[` is literal
// whatever the dialect says, so it can show this axis and not its neighbor.
func matchesWith(t *testing.T, plain, sub BracketPolicy, pattern, subject string) bool {
	t.Helper()
	src := `case "` + subject + `" in ` + pattern + `) echo Y;; *) echo n;; esac`
	out, st := run(t, src, withSem(subBracketSem(plain, sub)))
	if st != 0 {
		t.Fatalf("%s vs %q: status %d, out %q", pattern, subject, st, out)
	}
	return strings.TrimSpace(out) == "Y"
}

// The two axes are independent, which is the column ksh93 holds: a bare `[`
// is a literal `[` there and `[[:alpha:]` matches nothing. A suite that set
// one field for both would pass whichever way the code read it.
func TestTheTwoBracketAxesAreAskedSeparately(t *testing.T) {
	for _, tc := range []struct {
		name          string
		plain, sub    BracketPolicy
		bare, classed bool
	}{
		// ksh93's pair, and the reason there are two axes at all.
		{"literal bare, no match behind a class", BracketLiteral, BracketNoMatch, true, false},
		// The other way round, which no column holds and which the code
		// must still keep apart.
		{"no match bare, literal behind a class", BracketNoMatch, BracketLiteral, false, true},
		// bash's pair and dash's, where the two agree — the rows that
		// cannot tell the axes apart and are here to say so.
		{"literal both ways", BracketLiteral, BracketLiteral, true, true},
		{"no match either way", BracketNoMatch, BracketNoMatch, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesWith(t, tc.plain, tc.sub, "[", "["); got != tc.bare {
				t.Errorf("a bare bracket: got %v, want %v", got, tc.bare)
			}
			if got := matchesWith(t, tc.plain, tc.sub, "[[:alpha:]", "[a"); got != tc.classed {
				t.Errorf("a class-opened bracket: got %v, want %v", got, tc.classed)
			}
		})
	}
}

// A bracket that really closes asks neither axis, so a `[[:alpha:]]` is an
// ordinary class wherever the two are set — and the core, which answers
// neither, still runs it.
func TestAClosedBracketAsksNeitherBracketAxis(t *testing.T) {
	for _, tc := range []struct{ pattern, subject, want string }{
		{"[[:alpha:]]", "ab", "[b]"},
		{"[[:alpha:]]", "[a", "[[a]"},
	} {
		if got := trimmedWith(t, BracketNoMatch, BracketNoMatch, tc.pattern, tc.subject); got != tc.want {
			t.Errorf("%s vs %q: got %s, want %s", tc.pattern, tc.subject, got, tc.want)
		}
	}
	core := CoreSemantics()
	if out, st := run(t, `case ab in [[:alpha:]]b) echo in;; *) echo out;; esac`, withSem(core)); out != "in\n" || st != 0 {
		t.Errorf("a closed class in the core: got %q status %d", out, st)
	}
	// And the core refuses the shape that does pose the question, naming it.
	out, st := run(t, `case "[a" in [[:alpha:]) echo in;; *) echo out;; esac`, withSem(core))
	if st != 2 || !strings.Contains(out, "left unterminated") {
		t.Errorf("the core should refuse and name the axis: %q status %d", out, st)
	}
}

// The bad-pattern reading reaches this shape too, and it is what zsh gives:
// the refusal was being lost because the slot it is written to was handed
// over only where a separate scan had already spotted the open bracket — and
// that scan stops at the class's own `]`.
func TestTheBadPatternReadingReachesABracketASubExpressionLeftOpen(t *testing.T) {
	sem := subBracketSem(BracketBadPattern, BracketBadPattern)
	out, _ := run(t, `case "[a" in [[:alpha:]) echo hit;; *) echo miss;; esac; echo after`, withSem(sem))
	if !strings.Contains(out, "bad pattern") {
		t.Errorf("got %q, want a bad-pattern refusal", out)
	}
	if strings.Contains(out, "hit") || strings.Contains(out, "miss") || strings.Contains(out, "after") {
		t.Errorf("the script should stop: got %q", out)
	}
	// The control: with the sub axis somewhere else, the same pattern is
	// matched rather than refused, so the row above is about this axis and
	// not about brackets in general.
	out, _ = run(t, `case "[a" in [[:alpha:]) echo hit;; *) echo miss;; esac; echo after`,
		withSem(subBracketSem(BracketBadPattern, BracketLiteral)))
	if !strings.Contains(out, "hit") || !strings.Contains(out, "after") {
		t.Errorf("literal sub axis: got %q", out)
	}
}
