// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether an unquoted list is joined on the first character of IFS before it
// is split is a question of its own, and the list path was answering it
// without asking: it never joined, which is right for three of the six shells
// in the panel and wrong for the other three.
//
// It only shows under a *non-whitespace* IFS. Under a whitespace one the two
// readings give the same fields — a run of separators is one delimiter and an
// empty element leaves nothing behind either way — which is why `a=("" x)` is
// `[x]` in every shell measured and why the drop looked unanimous.
//
// Measured 2026-09-06 on the snippets below, against bash 5.3.15, bash 3.2.57,
// that 5.3.15 build invoked as `sh`, dash, ksh93u+ 2012-08-01 and zsh 5.9.2 —
// the last both as it comes and under `setopt shwordsplit`.

// joinRun runs src with the two axes an unquoted list asks answered, and
// nothing else moved.
func joinRun(t *testing.T, src string, join Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = Yes
		s.UnquotedListJoinsOnIFS = join
	})
}

// The row the issue is about: an empty element between two others.
//
// bash joins to `x::y`, where two separators meet and the field between them
// survives, so three fields come out of three parameters. ksh93 and dash take
// the elements one at a time and the empty one is no field at all, so two do.
// The field count is asserted with the fields because `[x][y]` and `[x][][y]`
// are the same characters once the boundaries are gone.
func TestAnEmptyElementSurvivesOnlyWhereTheListIsJoined(t *testing.T) {
	const src = `IFS=:; set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := joinRun(t, src, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
	if out, st := joinRun(t, src, No); out != "2[x][y]" || st != 0 {
		t.Errorf("element by element: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// The array spelling is the same question reached by its own branch, and the
// two must not part company: `$@` and `${a[@]}` are the same fields for the
// same elements in every shell measured.
func TestTheArraySpellingJoinsWithTheParameters(t *testing.T) {
	const src = `IFS=:; a=(x "" y); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := joinRun(t, src, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
	if out, st := joinRun(t, src, No); out != "2[x][y]" || st != 0 {
		t.Errorf("element by element: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// The join is one answer with two faces, and this is the second: it is also
// what *loses* a trailing empty element, because a trailing separator makes no
// field. So the axis cannot be spelled "an empty element survives" — that
// reading keeps the trailing one and bash does not.
//
// bash 5.3.15: `IFS=:; set -- x y ""` is two fields, the same two the join
// `x:y:` splits into. zsh under `setopt shwordsplit`, which does not join,
// keeps three.
func TestTheJoinLosesATrailingEmptyElement(t *testing.T) {
	const src = `IFS=:; set -- x y ""; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := joinRun(t, src, Yes); out != "2[x][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
	// Taken one at a time the empty element is dropped too, so this pair
	// agrees — which is why the row above is the one that separates them and
	// this one is here to say the join does not simply add a field.
	if out, st := joinRun(t, src, No); out != "2[x][y]" || st != 0 {
		t.Errorf("element by element: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// And the third face, which no reading of an *empty element* rule reaches at
// all: an element that ends in a separator meets the one the join inserts, and
// the empty field between them is a field bash has and ksh93 does not.
//
// `IFS=:; set -- "x:" y` is `x::y` joined — three fields — and `[x]` then
// `[y]` taken one at a time.
func TestAnElementEndingInASeparatorMeetsTheJoins(t *testing.T) {
	const src = `IFS=:; set -- "x:" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := joinRun(t, src, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
	if out, st := joinRun(t, src, No); out != "2[x][y]" || st != 0 {
		t.Errorf("element by element: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// A whitespace IFS is where the two readings coincide, and the axis must not
// be asked there — a plain `for f in $@` cannot be made to demand a dialect.
//
// Left unanswered on purpose: an unanswered axis that is asked says so and
// refuses, so the fields coming back at status 0 are the whole assertion.
func TestAWhitespaceSeparatorNeverAsksTheJoin(t *testing.T) {
	for _, src := range []string{
		`set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`,
		`a=("" x); set -- ${a[@]}; printf "%d" "$#"; printf "[%s]" "$@"`,
		`set -- "a b" c; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`,
	} {
		out, st := joinRun(t, src, Unspecified)
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s: asked the join where it changes nothing: %q", src, out)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", src, st)
		}
	}
}

// Nor is it asked where there is nothing to join with. An IFS that is set and
// empty has no first character, and no shell in the panel joins there:
// `IFS=""; set -- x y` is two fields for `$@`, where joining would make it
// one. Nor where there is nothing to join: a single element is its own join.
//
// Both axes are left unanswered, and the splitting one is the sharper of the
// two: a lone empty element is dropped by every shell in the panel whether it
// splits or not, so a list of one must not reach the splitting question on its
// way to a join it could never make. A lone element that *holds* a separator
// is a different case and does reach it — `IFS=:; set -- ":"` is one field or
// none depending on the splitting answer, which is the older question and not
// this one.
func TestNothingToJoinWithNeverAsksTheJoin(t *testing.T) {
	for _, src := range []string{
		`IFS=""; set -- x ""; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`,
		`IFS=:; set -- ""; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`,
		`IFS=:; set -- x y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`,
	} {
		out, st := axisRun(t, src, func(s *Semantics) {
			s.GlobExpansionResults = Yes
		})
		if strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s: asked an axis where it cannot change anything: %q", src, out)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", src, st)
		}
	}
}

// The splitting answer stands in front of the join, and it has to: with
// splitting off there is nothing to undo the join and the whole list would
// come back as one field, which no shell in the panel does. zsh has its
// splitting off as it comes and still gives one field per element.
//
// Asserted with the join answered *yes*, which is the combination that would
// go wrong: a shell that joined without splitting would print one field here.
func TestSplittingStandsInFrontOfTheJoin(t *testing.T) {
	const src = `IFS=:; set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`

	out, st := axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = No
		s.GlobExpansionResults = Yes
		s.UnquotedListJoinsOnIFS = Yes
	})
	if out != "2[x][y]" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0 — the elements, unjoined", out, st, "2[x][y]")
	}
}

// The quoted spellings are not this question and must not reach it. `"$@"` is
// one field per parameter and `"$*"` is one field holding the join, in every
// shell measured — both core, and neither moves with the axis.
func TestTheQuotedSpellingsDoNotAskTheJoin(t *testing.T) {
	const at = `IFS=:; set -- x "" y; set -- "$@"; printf "%d" "$#"; printf "[%s]" "$@"`
	const star = `IFS=:; set -- x "" y; set -- "$*"; printf "%d" "$#"; printf "[%s]" "$@"`

	for _, join := range []Answer{Yes, No, Unspecified} {
		if out, st := joinRun(t, at, join); out != `3[x][][y]` || st != 0 {
			t.Errorf(`join=%v: "$@" gave %q status %d, want one field per parameter`, join, out, st)
		}
		if out, st := joinRun(t, star, join); out != `1[x::y]` || st != 0 {
			t.Errorf(`join=%v: "$*" gave %q status %d, want one joined field`, join, out, st)
		}
	}
}

// The join is on the *first* character of IFS and not on the whole of it,
// which only a separator set with more than one character can show.
//
// bash 5.3.15: `IFS=":,"; set -- x "" y` is three fields — `x:` and `:y` with
// one empty between, which is the join `x::y` split back. Joining on both
// characters would put `:,` between each pair, and each of those is a
// delimiter of its own, so five fields would come out.
func TestTheJoinUsesTheFirstCharacterOfIFS(t *testing.T) {
	const src = `IFS=":,"; set -- x "" y; set -- $@; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := joinRun(t, src, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
}

// The preset for the specification takes the reading that removes a null
// field, which is what POSIX describes and what dash — the panel's shell
// written to that text — does. bash's join is the departure from it.
//
// Pinned because nothing else reaches this preset: every test above answers
// the axis for itself, and every dialect answers it in its own package, so
// without this row the specification's answer is whatever the last edit left
// behind.
func TestTheSpecificationsAnswerIsTheUnjoinedReading(t *testing.T) {
	if got := PosixSemantics().UnquotedListJoinsOnIFS; got != No {
		t.Errorf("PosixSemantics().UnquotedListJoinsOnIFS = %v, want %v", got, No)
	}
	// And the core leaves it unanswered, because the shells disagree: a
	// binary with no dialect behind it says so rather than picking one.
	if got := CoreSemantics().UnquotedListJoinsOnIFS; got != Unspecified {
		t.Errorf("CoreSemantics().UnquotedListJoinsOnIFS = %v, want it unanswered", got)
	}
}
