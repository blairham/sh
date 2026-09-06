// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Two halves of one question about a pattern that may stand for nothing.
//
// The first is core and was silently wrong: a group whose arm matches no text
// never matched zero-length, so `@(|a)b` did not match `b` in any dialect
// while bash 5.3.15, bash as `sh`, bash 3.2.57 and ksh93u+ all match it.
// Measured 2026-09-06, `env -i` with a scratch HOME, `shopt -s extglob` on a
// line of its own so it is in force before the next line is parsed.
//
// The second is a grammar flag one shell answers yes to: an alternative of a
// `case` arm's pattern list written as nothing. The parse half is
// syntax/emptycasepattern_test.go.

// runWithDialect runs src with one flag turned on, and turns it on for the
// *runner* as well as for the parse. The matcher asks the runner which shell
// it is — a group is only a group where the dialect says so — so a helper
// that told only the parser would assert the core's answer while claiming to
// assert this one.
func runWithDialect(t *testing.T, src string, enable func(*syntax.Dialect)) (string, int) {
	t.Helper()
	return runGrammar(t, src, enable, func(r *Runner) {
		d := syntax.Core()
		enable(&d)
		r.Dialect = &d
	})
}

// runGroups runs with extended patterns on, which is what spells a group in
// the two shells that agree about the answer below.
func runGroups(t *testing.T, src string) (string, int) {
	t.Helper()
	return runWithDialect(t, src, func(d *syntax.Dialect) { d.ExtendedPattern = true })
}

// runEmptyAlt runs with the empty alternative on and nothing else.
func runEmptyAlt(t *testing.T, src string) (string, int) {
	t.Helper()
	return runWithDialect(t, src, func(d *syntax.Dialect) { d.CasePatternMayBeEmpty = true })
}

// A group whose arm matches no text stands for no text, however it is
// quantified and including not at all. Every line here is `m` in bash 5.3.15,
// bash as `sh`, bash 3.2.57 and ksh93u+; four of the six were `no` here.
func TestAGroupWhoseArmMatchesNothingStandsForNothing(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a starred arm at the front", `case b in @(*)b) echo m;; *) echo no;; esac`},
		{"an empty leading arm", `case b in @(|a)b) echo m;; *) echo no;; esac`},
		{"the arm that is not empty still matches", `case ab in @(|a)b) echo m;; *) echo no;; esac`},
		{"an empty trailing arm against an empty subject", `case "" in @(a|)) echo m;; *) echo no;; esac`},
		{"one or more of something that may be nothing", `case "" in +(|a)) echo m;; *) echo no;; esac`},
		{"zero or one, which was already right", `case b in ?(a)b) echo m;; *) echo no;; esac`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGroups(t, tc.src)
			if want := "m\n"; out != want {
				t.Errorf("out = %q, want %q", out, want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The same rule reaching the trim operators, where it is the difference
// between the two of them. `${v#@(|a)}` trims the shortest match an
// alternative offers and the empty one is the shortest there is, so it trims
// nothing; `##` takes the longest and trims the `a`. Measured: `abc` and `bc`
// in bash and ksh93 alike, where this answered `bc` for both — a plausible
// wrong string, which is the failure this repository exists to avoid.
func TestTheShortestMatchOfAnAlternativeMayBeNoTextAtAll(t *testing.T) {
	out, st := runGroups(t, `v=abc; echo "[${v#@(|a)}][${v##@(|a)}]"`)
	if want := "[abc][bc]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// It does not reach the negated group, which asks the ordinary question and
// inverts it and so already tried every split from zero. `!(a)` against an
// empty subject is a match in bash and ksh93, and was before this.
func TestTheNegatedGroupIsUnchanged(t *testing.T) {
	out, st := runGroups(t, `case "" in !(a)) echo m;; *) echo no;; esac`)
	if want := "m\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The grammar half, running: an arm with an alternative written as nothing
// matches the empty subject and each of the named ones, and nothing else.
// `case "" in (|https|git)` is the idiom for "one of these schemes, or none".
func TestAnEmptyAlternativeMatchesTheEmptySubjectAndOnlyThat(t *testing.T) {
	for _, tc := range []struct{ subject, want string }{
		{"", "m"},
		{"https", "m"},
		{"git", "m"},
		{"ftp", "no"},
	} {
		src := `case "` + tc.subject + `" in (|https|git) echo m;; *) echo no;; esac`
		out, st := runEmptyAlt(t, src)
		if want := tc.want + "\n"; out != want {
			t.Errorf("%q: out = %q, want %q", src, out, want)
		}
		if st != 0 {
			t.Errorf("%q: status = %d, want 0", src, st)
		}
	}
}

// An alternative written as nothing matches *only* nothing — not everything,
// which is what an empty pattern would mean if it were read as a glob of no
// characters, and the reading a list of three empties would hide.
func TestAListOfNothingButEmptyAlternativesMatchesOnlyTheEmptySubject(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`case "" in (|) echo m;; *) echo no;; esac`, "m"},
		{`case x in (|) echo m;; *) echo no;; esac`, "no"},
		{`case "" in (||) echo m;; *) echo no;; esac`, "m"},
		{`case x in (||) echo m;; *) echo no;; esac`, "no"},
		{`case "" in (a||b) echo m;; *) echo no;; esac`, "m"},
		{`case b in (a||b) echo m;; *) echo no;; esac`, "m"},
		{`case "" in (a|b|) echo m;; *) echo no;; esac`, "m"},
	} {
		out, st := runEmptyAlt(t, tc.src)
		if want := tc.want + "\n"; out != want {
			t.Errorf("%q: out = %q, want %q", tc.src, out, want)
		}
		if st != 0 {
			t.Errorf("%q: status = %d, want 0", tc.src, st)
		}
	}
}
