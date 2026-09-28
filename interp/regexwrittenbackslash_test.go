// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// keptBackslash runs one condition with the two readings of a written
// backslash in a `=~` operand, and nothing else moved.
//
// `RegexQuotingMakesLiteral` is pinned **No** in both columns on purpose.
// That axis reaches the same answer as this one on a metacharacter row and
// by a different route, so a sweep that left it to the vector would be
// measuring whichever of the two happened to be armed. Its own rows are
// interp/regexquoting_test.go's.
func keptBackslash(t *testing.T, src string, keeps Answer) string {
	t.Helper()
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.DoubleBracket = true
	}, func(r *Runner) {
		s := *r.Semantics
		s.RegexQuotingMakesLiteral = No
		s.RegexDigitClassEscape = No
		s.RegexKeepsAWrittenBackslash = keeps
		r.Semantics = &s
	})
	return strings.TrimSpace(out)
}

// A backslash-quoted character in a `=~` operand either keeps its backslash
// into the engine or loses it to quote removal, and the two readings are a
// column apart: ksh93 keeps it, bash, zsh and BusyBox ash do not.
//
// **Every row is a pair**, because the two readings agree on half of all
// subjects. A row asserting only that `a\.b` fails to match `axb` would pass
// for a reading that made the whole operand literal, and one asserting only
// that `za\wb` matches `zawb` would pass for every reading there is.
//
// Measured 2026-09-28 — see Semantics.RegexKeepsAWrittenBackslash for the
// four columns and the binaries.
func TestAWrittenBackslashEitherReachesTheEngineOrDoesNot(t *testing.T) {
	for _, tc := range []struct{ name, src, kept, dropped string }{
		// The control: with no backslash on it a `.` is a `.` under both
		// readings, so the operator reaches an engine either way.
		{
			"the control", `[[ axb =~ a.b ]] && echo match || echo no`,
			"match", "match",
		},

		// The backslash was protecting a metacharacter. Kept, the engine
		// gets `\.` and reads text; dropped, the `.` is live again.
		{
			"a quoted dot", `[[ axb =~ a\.b ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"and the other half of it", `[[ "a.b" =~ a\.b ]] && echo match || echo no`,
			"match", "match",
		},
		{
			"a quoted plus", `[[ ab =~ a\+b ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"and its other half", `[[ "a+b" =~ a\+b ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"a quoted brace count", `[[ aab =~ a\{2\}b ]] && echo match || echo no`,
			"no", "match",
		},
		{
			"and its other half", `[[ "a{2}b" =~ a\{2\}b ]] && echo match || echo no`,
			"match", "no",
		},

		// The backslash was naming a class. Kept, the engine reads the
		// class; dropped, the letter is a letter.
		{
			"a quoted w matches its letter either way",
			`[[ zawb =~ za\wb ]] && echo match || echo no`,
			"match", "match",
		},
		{
			"and a digit only where it is a class",
			`[[ za1b =~ za\wb ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"a quoted s", `[[ "za b" =~ za\sb ]] && echo match || echo no`,
			"match", "no",
		},
		{
			"a quoted W", `[[ "za.b" =~ za\Wb ]] && echo match || echo no`,
			"match", "no",
		},

		// A **double-quoted** operand is a different question and neither
		// reading here touches it: both columns answer the same. #4977.
		{
			"a double-quoted escape is not this",
			`[[ zadb =~ "za\db" ]] && echo match || echo no`,
			"match", "match",
		},
		{
			"nor a double-quoted metacharacter",
			`[[ axb =~ "a\.b" ]] && echo match || echo no`,
			"no", "no",
		},
	} {
		if got := keptBackslash(t, tc.src, Yes); got != tc.kept {
			t.Errorf("%s kept: %s = %q, want %q", tc.name, tc.src, got, tc.kept)
		}
		if got := keptBackslash(t, tc.src, No); got != tc.dropped {
			t.Errorf("%s dropped: %s = %q, want %q", tc.name, tc.src, got, tc.dropped)
		}
	}
}

// The narrow axis keeps its own two letters where the wide one says no, and
// the wide one covers them where it says yes — so `\d` reads as a class under
// either, and under neither it is the letter.
//
// The two are not one question with two names, and the rows below are how
// they divide the work: a **written** `\d` needs both — the backslash has to
// survive quote removal *and* the engine has to read it — while a `\d` that
// arrived **through a variable** needs only the narrow one, there being no
// quoting for the wide one to be about.
//
// The combination Yes-narrow-No-wide is not hypothetical: it is BusyBox
// v1.37.0, whose engine has the class and whose quote removal eats the
// backslash all the same.
func TestTheDigitClassAxisIsStillItsOwnQuestion(t *testing.T) {
	run := func(src string, wide, narrow Answer) string {
		t.Helper()
		out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
			d.DoubleBracket = true
		}, func(r *Runner) {
			s := *r.Semantics
			s.RegexQuotingMakesLiteral = No
			s.RegexKeepsAWrittenBackslash = wide
			s.RegexDigitClassEscape = narrow
			r.Semantics = &s
		})
		return strings.TrimSpace(out)
	}
	const digit = `[[ za1b =~ za\db ]] && echo match || echo no`
	const letter = `[[ zadb =~ za\db ]] && echo match || echo no`
	for _, tc := range []struct {
		name          string
		wide, narrow  Answer
		digit, letter string
	}{
		// **Two gates and a written `\d` needs both.** The wide axis gets
		// the pair past quote removal and the narrow one decides what the
		// engine reads it as, so three of these four rows are the letter and
		// only the pair of Yes is the class.
		{"neither", No, No, "no", "match"},
		// The narrow one alone is **BusyBox v1.37.0's** pair of answers —
		// its engine reads the class and its quote removal still eats a
		// written backslash — and that column is why the combination has to
		// be expressible at all (#4991).
		//
		// **The row itself is this road's, not that shell's**, and the
		// distinction is worth keeping: BusyBox reaches `=~` through the
		// `[[` *builtin*, so it never runs the code this test drives. What
		// it measures is the factoring — a written `\d` needs both gates —
		// and dialect/ash/regexdigitclass_test.go is where that shell's own
		// rows are. This row used to read `match` because the narrow axis
		// governed quote removal too, which is what made the column
		// inexpressible.
		{"the narrow one alone", No, Yes, "no", "match"},
		{"the wide one alone", Yes, No, "no", "match"},
		{"both", Yes, Yes, "match", "no"},
	} {
		if got := run(digit, tc.wide, tc.narrow); got != tc.digit {
			t.Errorf("%s: %s = %q, want %q", tc.name, digit, got, tc.digit)
		}
		if got := run(letter, tc.wide, tc.narrow); got != tc.letter {
			t.Errorf("%s: %s = %q, want %q", tc.name, letter, got, tc.letter)
		}
	}
	// The variable spelling is the narrow axis's alone: no quote removal
	// happens there, so the wide one has no backslash to keep.
	const live = `r='za\db'; [[ za1b =~ $r ]] && echo match || echo no`
	if got := run(live, Yes, No); got != "no" {
		t.Errorf("a live operand under the wide axis alone: %q, want %q", got, "no")
	}
	// And a live operand is unmoved by the wide axis in the other direction
	// too: with the narrow one Yes it is a class whatever the wide one says.
	if got := run(live, Yes, Yes); got != "match" {
		t.Errorf("a live operand under both: %q, want %q", got, "match")
	}
	// A **double-quoted** escape is neither axis's, and this is the row that
	// says so where it can actually fire: with both Yes, a reading that took
	// a double-quoted span for a backslash-quoted one would hand `\d` to the
	// engine and make this a match. It is not one in ksh93 either.
	const quoted = `[[ za1b =~ "za\db" ]] && echo match || echo no`
	for _, both := range [][2]Answer{{No, No}, {No, Yes}, {Yes, No}, {Yes, Yes}} {
		if got := run(quoted, both[0], both[1]); got != "no" {
			t.Errorf("a double-quoted escape at wide=%v narrow=%v: %q, want %q",
				both[0], both[1], got, "no")
		}
	}
	if got := run(live, No, Yes); got != "match" {
		t.Errorf("a live operand under the narrow axis alone: %q, want %q", got, "match")
	}
}
