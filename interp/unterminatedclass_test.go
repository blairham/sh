// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `[:` that nothing closes, which is a question of its own rather than a
// corner of the unknown-name axis beside it — see
// Semantics.UnterminatedCharacterClass. Which preset gives which answer is
// asserted in the dialect packages and never here.

// unterminatedClassSem answers the axis by name over the permissive base.
//
// The bracket axis comes with it, because one of the four answers hands the
// text straight to it: a shell that takes the `]` as part of the name it is
// still looking for has an unterminated bracket on its hands and nothing else
// to say about it.
func unterminatedClassSem(p UnterminatedClassPolicy) Semantics {
	s := permissive()
	s.UnterminatedCharacterClass = p
	s.UnterminatedBracket = BracketLiteral
	// And the axis beside it, because the text arrives there rather than at
	// the one above: the bracket these rows leave open was left open by the
	// `[:` inside it. Both at the same value, so the rows below measure the
	// class policy and nothing else.
	s.UnterminatedBracketAfterASubExpression = BracketLiteral
	return s
}

// trimmedUnder returns what a prefix trim with pattern leaves of subject, so
// that *what* matched is visible as text rather than only as a yes or no.
func trimmedUnder(t *testing.T, p UnterminatedClassPolicy, pattern, subject string) string {
	t.Helper()
	src := `v='` + subject + `'; printf "[%s]" "${v#` + pattern + `}"`
	out, st := run(t, src, withSem(unterminatedClassSem(p)))
	if st != 0 {
		t.Fatalf("%s on %q: status %d, out %q", pattern, subject, st, out)
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, "["), "]")
}

// The discriminating table. Every column is a policy and every row a pattern
// and subject that at least two of the four answer differently.
func TestAnUnterminatedClassHasFourAnswers(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject string
		// What is left of the subject under each reading.
		ordinary, ends, empties, swallows string
	}{
		// The two rows the issue was filed from: does the bracket hold the
		// colon, and does it hold the opening bracket?
		{"[[:]", ":x", "x", ":x", ":x", ":x"},
		{"[[:]", "[:y", ":y", "[:y", "[:y", "y"},
		// A member written *before* the `[:` is what separates ending the
		// scan from emptying the bracket.
		{"[a[:]", "ab", "b", "b", "ab", "ab"},
		// And one written after it separates ending the scan from leaving
		// the two characters ordinary.
		{"[[:b]", "ba", "a", "ba", "ba", "ba"},
		{"[a[:]", ":z", "z", ":z", ":z", ":z"},
		// The row that only the fourth reading answers: with the `]` taken
		// as part of the name, `[[:]]` is a literal `[`, a bracket holding
		// the colon, and a literal `]` — three things rather than one.
		{"[[:]]", "[:]z", "[:]z", "[:]z", "[:]z", "z"},
	} {
		for _, c := range []struct {
			policy UnterminatedClassPolicy
			want   string
		}{
			{UnterminatedClassIsOrdinaryCharacters, tc.ordinary},
			{UnterminatedClassEndsTheScan, tc.ends},
			{UnterminatedClassEmptiesTheBracket, tc.empties},
			{UnterminatedClassSwallowsTheClosingBracket, tc.swallows},
		} {
			if got := trimmedUnder(t, c.policy, tc.pattern, tc.subject); got != c.want {
				t.Errorf("%v: %s on %q left %q, want %q",
					c.policy, tc.pattern, tc.subject, got, c.want)
			}
		}
	}
}

// A class that *is* closed is untouched by the axis, whichever answer is in
// force — otherwise the policy would be a rule about brackets rather than
// about a name that never ends.
func TestAClosedClassIsUntouchedByTheUnterminatedAxis(t *testing.T) {
	for _, p := range []UnterminatedClassPolicy{
		UnterminatedClassIsOrdinaryCharacters,
		UnterminatedClassEndsTheScan,
		UnterminatedClassEmptiesTheBracket,
		UnterminatedClassSwallowsTheClosingBracket,
	} {
		for _, tc := range []struct{ pattern, subject, want string }{
			{"[[:digit:]]", "7x", "x"},
			{"[a[:upper:]b]", "Az", "z"},
			{"[abc]", "bq", "q"},
		} {
			if got := trimmedUnder(t, p, tc.pattern, tc.subject); got != tc.want {
				t.Errorf("%v: %s on %q left %q, want %q",
					p, tc.pattern, tc.subject, got, tc.want)
			}
		}
	}
}

// The axis is asked only where a pattern actually holds a `[:` nothing
// closes, the same rule the sibling axes follow — so the core, which answers
// nothing, still matches an ordinary bracket and a closed class, and refuses
// only this one.
func TestTheUnterminatedClassAxisIsAskedOnlyWhenItApplies(t *testing.T) {
	for _, src := range []string{
		`case b in [abc]) echo in;; *) echo out;; esac`,
		`case 7 in [[:digit:]]) echo in;; *) echo out;; esac`,
	} {
		if out, st := run(t, src, withSem(CoreSemantics())); out != "in\n" || st != 0 {
			t.Errorf("%s in the core: got %q status %d", src, out, st)
		}
	}
	out, st := run(t, `case : in [[:]) echo in;; *) echo out;; esac`, withSem(CoreSemantics()))
	if st == 0 || !strings.Contains(out, "nothing closes") {
		t.Errorf("an unterminated class in the core: got %q status %d, want it refused", out, st)
	}
}
