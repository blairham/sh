// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A character class that is properly closed and whose name the shell has
// never heard of. Three answers, none a variation on the others — see
// Semantics.UnknownCharacterClass. Which preset gives which is asserted in
// the dialect packages and never here.

// unknownClassSem answers the axis by name over the permissive base.
func unknownClassSem(p UnknownClassPolicy) Semantics {
	s := permissive()
	s.UnknownCharacterClass = p
	return s
}

// matchesUnder answers whether pattern matches subject under one policy.
func matchesUnder(t *testing.T, p UnknownClassPolicy, pattern, subject string) bool {
	t.Helper()
	src := `case "` + subject + `" in ` + pattern + `) echo Y;; *) echo n;; esac`
	out, st := run(t, src, withSem(unknownClassSem(p)))
	if st != 0 {
		t.Fatalf("%s vs %q: status %d, out %q", pattern, subject, st, out)
	}
	return strings.TrimSpace(out) == "Y"
}

// The discriminating table. Every column is a policy and every row a pattern
// and subject that at least two of the three answer differently — a
// `[[:nope:]]` on its own is a miss under all three, because there is nothing
// beside the name for it to have an effect on.
func TestAnUnknownClassNameHasThreeAnswers(t *testing.T) {
	for _, tc := range []struct {
		pattern, subject     string
		inert, ends, empties bool
	}{
		{"[a[:nope:]b]", "a", true, true, false},
		{"[a[:nope:]b]", "b", true, false, false},
		{"[a[:nope:]b]", "q", false, false, false},
		// A *known* class in front of the unknown one still matches where
		// the scan stops and does not where the bracket is emptied, which is
		// the row that separates those two.
		{"[a[:upper:][:nope:]b]", "A", true, true, false},
		{"[a[:upper:][:nope:]b]", "b", true, false, false},
		// Nothing written after the name counts where the scan stops.
		{"[[:nope:]a]", "a", true, false, false},
		// The negation does not survive the give-up: a set of just `a`
		// negated would hold `q`, and it does not.
		{"[!a[:nope:]b]", "q", true, false, false},
		{"[!a[:nope:]b]", "a", false, false, false},
	} {
		for _, c := range []struct {
			policy UnknownClassPolicy
			want   bool
		}{
			{UnknownClassIsInert, tc.inert},
			{UnknownClassEndsTheScan, tc.ends},
			{UnknownClassEmptiesTheBracket, tc.empties},
		} {
			if got := matchesUnder(t, c.policy, tc.pattern, tc.subject); got != c.want {
				t.Errorf("%v: %s vs %q = %v, want %v",
					c.policy, tc.pattern, tc.subject, got, c.want)
			}
		}
	}
}

// The empty name is an unknown name and not a case of its own: `[a[::]b]`
// answers exactly as `[a[:nope:]b]` under every policy, measured in every
// column of the panel.
func TestAnEmptyClassNameIsAnUnknownOne(t *testing.T) {
	for _, p := range []UnknownClassPolicy{
		UnknownClassIsInert,
		UnknownClassEndsTheScan,
		UnknownClassEmptiesTheBracket,
	} {
		for _, subject := range []string{"a", "b", "q"} {
			named := matchesUnder(t, p, "[a[:nope:]b]", subject)
			empty := matchesUnder(t, p, "[a[::]b]", subject)
			if named != empty {
				t.Errorf("%v: %q named=%v empty=%v, want the same answer",
					p, subject, named, empty)
			}
		}
	}
}

// A class the shell *does* have is untouched by the axis, whichever answer is
// in force — otherwise the policy would be a rule about brackets rather than
// about a name nobody recognizes.
func TestAKnownClassIsUntouchedByTheAxis(t *testing.T) {
	for _, p := range []UnknownClassPolicy{
		UnknownClassIsInert,
		UnknownClassEndsTheScan,
		UnknownClassEmptiesTheBracket,
	} {
		for _, tc := range []struct {
			pattern, subject string
			want             bool
		}{
			{"[a[:upper:]b]", "A", true},
			{"[a[:upper:]b]", "b", true},
			{"[a[:upper:]b]", "q", false},
			{"[[:digit:]]", "7", true},
		} {
			if got := matchesUnder(t, p, tc.pattern, tc.subject); got != tc.want {
				t.Errorf("%v: %s vs %q = %v, want %v",
					p, tc.pattern, tc.subject, got, tc.want)
			}
		}
	}
}

// The axis is asked only where a pattern actually holds a name the shell has
// not got, the same rule the caret and bracket axes follow — so the core,
// which answers nothing, still matches an ordinary bracket and still refuses
// this one.
func TestTheUnknownClassAxisIsAskedOnlyWhenItApplies(t *testing.T) {
	if out, st := run(t, `case b in [abc]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); out != "in\n" || st != 0 {
		t.Errorf("an ordinary bracket in the core: got %q status %d", out, st)
	}
	if out, st := run(t, `case b in [a[:upper:]b]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); out != "in\n" || st != 0 {
		t.Errorf("a class the core has: got %q status %d", out, st)
	}
	if _, st := run(t, `case b in [a[:nope:]b]) echo in;; *) echo out;; esac`,
		withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse a name it has not got, status %d", st)
	}
}
