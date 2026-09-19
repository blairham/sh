// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a successful **pattern** match leaves in the record a dialect names —
// Semantics.PatternMatchWritesTheMatchRecord, and #2916, where the record was
// filled by `=~` alone and every glob comparison and pattern operator left it
// holding whatever the last regular expression had put there.
//
// Named for the axis and never for a shell; which value each preset holds is
// asserted in dialect/patternrecord_test.go against the panel's own bytes.
//
// Every row below is run at **both** values of the axis, and the `No` column
// is the control: the record must be exactly what the seeding `=~` left, which
// is what says a row is about this axis and not about the match. A test that
// only ever ran the `Yes` column could not tell a record the pattern wrote
// from one the seed wrote.

// patternRecord runs a snippet with the record named `M` and answers what the
// record holds afterwards, as a count and its elements.
//
// Every snippet is seeded with a `=~` first, so "nothing was written" is a
// legible answer rather than an empty array that could have come from either
// side. The seed is a regular expression because that surface's record is not
// what this axis moves — `SEED` is what the `No` column must read back.
func patternRecord(t *testing.T, writes Answer, src string) string {
	t.Helper()
	sem := permissive()
	sem.RegexMatchSurvivesAFailedMatch = Yes
	sem.RegexMatchOmitsGroupsThatDidNotMatch = Yes
	sem.PatternMatchWritesTheMatchRecord = writes
	out, _ := runGrammar(t, `[[ SEED =~ SEED ]]; `+src+`; printf '%s:%s' "${#M[@]}" "${M[*]}"`,
		quantifiedGroups, func(r *Runner) {
			r.Semantics = &sem
			r.Dialect = quantifiedGroupDialect()
			r.SetRegexMatch("M")
		})
	return out
}

// quantifiedGroups is the grammar flag a `@(a|b)` needs, and
// quantifiedGroupDialect is the same answer for the runner, which builds its
// own dialect for anything it matches at run time.
//
// A test names the construct it depends on rather than a shell that happens
// to have it, which is why this is a flag here and a preset in dialect/.
func quantifiedGroups(d *syntax.Dialect) { d.ExtendedPattern = true }

func quantifiedGroupDialect() *syntax.Dialect {
	d := syntax.Core()
	d.ExtendedPattern = true
	return &d
}

func TestWhatAPatternMatchLeavesInTheRecord(t *testing.T) {
	for _, tc := range []struct{ name, src, yes, no string }{
		{
			// The row the issue is named for: a comparison against a pattern
			// leaves the whole match behind it.
			"a condition's glob", `[[ abcd == a*d ]]`, "1:abcd", "",
		},
		{
			// The operator's sense is nothing to it — the evaluation writes,
			// as it does for `=~`.
			"a condition that did not hold", `[[ abcd != a*d ]]`, "1:abcd", "",
		},
		{
			// A quantified group is a capture group here with nothing having
			// asked for one, which is the whole difference between this plan
			// and a reporting pattern's.
			"a quantified group", `[[ abcd == a?(b)cd ]]`, "2:abcd b", "",
		},
		{
			"two of them", `[[ abcd == a?(b)?(c)d ]]`, "3:abcd b c", "",
		},
		{
			// The closure quantifier is the one byte that is also a wildcard,
			// and it was read as one: this row had no group at all.
			"a closure quantifier", `[[ abcd == a*(b)cd ]]`, "2:abcd b", "",
		},
		{
			"a one-or-more quantifier", `[[ abcd == a+(b)cd ]]`, "2:abcd b", "",
		},
		{
			// A negation captures what it consumed rather than nothing, and
			// nothing else in this matcher had reason to write that span.
			"a negated group", `[[ abcd == a!(z)cd ]]`, "2:abcd b", "",
		},
		{
			// A group that contributed nothing is an empty element and not a
			// gap, which is the opposite of what the same record does with a
			// regular expression's groups — the row below is that control.
			"a group that matched nothing", `[[ abcd == a?(z)bcd ]]`, "2:abcd ", "",
		},
		{
			// The control for the control: a regular expression writes the
			// record under *either* answer, because this axis is about
			// patterns and `=~` has its own. So this row's `No` column is the
			// one row in the table that is not the seed.
			"a regular expression still drops one", `[[ abcd =~ b(z)?c ]]`, "1:bc", "1:bc",
		},
		{
			// A right operand with no operator in it is not a pattern, and
			// writes nothing — where the same literal in a trim does.
			"a literal comparison", `[[ abcd == abcd ]]`, "1:SEED", "",
		},
		{
			"an escaped operator is not one", `[[ 'a*c' == a\*c ]]`, "1:SEED", "",
		},
		{
			"a trim, literal", `v=hello; x=${v#he}`, "1:he", "",
		},
		{
			"a trim, longest", `v=hello; x=${v%%[lo]*}`, "1:llo", "",
		},
		{
			// The **first** match and not the last, which a two-character
			// class over a global replacement is what distinguishes.
			"a global replacement takes its first match", `v=hello; x=${v//[lo]/X}`, "1:l", "",
		},
		{
			"a replacement's groups", `v=hello; x=${v/@(l)(l)/X}`, "3:ll l l", "",
		},
		{
			// Not every match: a `case` arm and a failed operation both leave
			// the record where it was, under either answer.
			"a case arm", `case abcd in a*d) ;; esac`, "1:SEED", "",
		},
		{
			"a comparison that failed", `[[ abcd == z*z ]]`, "1:SEED", "",
		},
		{
			"a replacement that matched nothing", `w=abc; x=${w/zz/Y}`, "1:SEED", "",
		},
		{
			"a substring", `v=hello; x=${v:1:2}`, "1:SEED", "",
		},
		{
			"an ordering test", `[[ abc < b ]]`, "1:SEED", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternRecord(t, Yes, tc.src); got != tc.yes {
				t.Errorf("writing: got %q, want %q", got, tc.yes)
			}
			// The control. Whatever the row does at `Yes`, at `No` the record
			// is the seed — so a row that passes both ways is one the axis
			// never reached, and the rows above that expect `1:SEED` are
			// saying exactly that about their own surface.
			want := tc.no
			if want == "" {
				want = "1:SEED"
			}
			if got := patternRecord(t, No, tc.src); got != want {
				t.Errorf("not writing: got %q, want %q", got, want)
			}
		})
	}
}

// TestAPatternRecordDoesNotMakeAReplacementReport is the cost the axis must
// not have: whether a replacement's right-hand side is read again for each
// match is a question about the pattern's own flags, and filling a record is
// not asking one.
//
// `$((++i))` is the discriminator, because a replacement read once leaves the
// same text in every position and one read per match counts.
func TestAPatternRecordDoesNotMakeAReplacementReport(t *testing.T) {
	for _, writes := range []Answer{Yes, No} {
		sem := permissive()
		sem.RegexMatchSurvivesAFailedMatch = Yes
		sem.RegexMatchOmitsGroupsThatDidNotMatch = Yes
		sem.PatternMatchWritesTheMatchRecord = writes
		out, _ := runGrammar(t, `x=aaa; i=0; printf '%s i=%s' "${x//a/$((++i))}" "$i"`,
			quantifiedGroups, func(r *Runner) {
				r.Semantics = &sem
				r.Dialect = quantifiedGroupDialect()
				r.SetRegexMatch("M")
			})
		if out != "111 i=1" {
			t.Errorf("writes=%v: got %q, want %q", writes, out, "111 i=1")
		}
	}
}

// With no name from the dialect there is no record, so the axis is never
// reached — which is what lets a shell keeping its captures under another
// shape leave it unanswered while every pattern in it still matches.
func TestNoRecordNameAsksNothingOfAPatternMatch(t *testing.T) {
	sem := permissive()
	sem.PatternMatchWritesTheMatchRecord = Unspecified
	out, st := runGrammar(t, `[[ abcd == a*d ]]; v=hello; echo "${v#he}$?"`,
		quantifiedGroups, func(r *Runner) {
			r.Semantics = &sem
			r.Dialect = quantifiedGroupDialect()
		})
	if st != 0 || out != "llo0\n" {
		t.Errorf("out %q status %d, want both matches answered and nothing asked", out, st)
	}
	// And with a name, an unanswered axis refuses by name rather than
	// guessing whether the record is written.
	_, st = runGrammar(t, `[[ abcd == a*d ]]`, quantifiedGroups, func(r *Runner) {
		r.Semantics = &sem
		r.Dialect = quantifiedGroupDialect()
		r.SetRegexMatch("M")
	})
	if st != 2 {
		t.Errorf("status %d, want the refusal by name at 2", st)
	}
}

// TestAPatternRecordFillsNoReportingParameter is the other half of the
// separation: the record and the parameters a *reporting pattern* fills are
// two publishers over one report, and a surface that records must not write
// the second set.
//
// The shell whose record this is has no `$MATCH` or `$match`, so writing them
// would be inventing parameters for it — and the report carries the whole
// match and every group, which is exactly what would fill them.
func TestAPatternRecordFillsNoReportingParameter(t *testing.T) {
	sem := permissive()
	sem.RegexMatchSurvivesAFailedMatch = Yes
	sem.RegexMatchOmitsGroupsThatDidNotMatch = Yes
	sem.PatternMatchWritesTheMatchRecord = Yes
	out, _ := runGrammar(t,
		`[[ abcd == a?(b)cd ]]; printf '%s %s %s' "${M[*]}" "${MATCH-UNSET}" "${match[*]-UNSET}"`,
		quantifiedGroups, func(r *Runner) {
			r.Semantics = &sem
			r.Dialect = quantifiedGroupDialect()
			r.SetRegexMatch("M")
		})
	if out != "abcd b UNSET UNSET" {
		t.Errorf("got %q, want %q", out, "abcd b UNSET UNSET")
	}
}
