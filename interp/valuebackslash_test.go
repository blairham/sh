// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A backslash that was in a *value* survives the word it is expanded into.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, from a script file, against bash 5.3.15, bash 5.3.15
// under an argv[0] of `sh`, bash 3.2.57, dash, ksh93u+ and zsh 5.9.2. All six
// keep the backslash in every row below, and keep both of a doubled pair.
//
// The fields an expansion produces are carried in an escaped form, where a
// backslash in front of a character means "this was quoted" and is taken off
// again once the glob stage has had its look. A backslash *in the value* is
// the one byte that form cannot carry unmarked, and nothing marked it: the
// mark and the data were the same byte, so the unescape read the value's own
// backslash as a mark and removed it. `v='a\b'; w=$v` assigned `ab` at status
// 0 with no diagnostic, and `v='a\\b'` assigned one backslash where the panel
// keeps two (#1222).
//
// Written as a table over the contexts rather than as one case, because the
// loss was in the escaped form itself and therefore in every context that
// builds a field from a value — an assignment, a concatenation, a `case`
// subject, a split word and an array element all reached it, and a single
// assignment case could not have said how wide it was. The direct read is in
// the table as the control: it was **correct** throughout, which is what
// makes this a round trip through a word rather than a fault in the value.
func TestAValueKeepsItsBackslashThroughAWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"read directly", `v='a\b'; printf '[%s]' "$v"`, `[a\b]`},
		{"assigned through an expansion", `v='a\b'; w=$v; printf '[%s]' "$w"`, `[a\b]`},
		{"assigned through a quoted expansion", `v='a\b'; w="$v"; printf '[%s]' "$w"`, `[a\b]`},
		{"concatenated with itself", `v='a\b'; u=$v$v; printf '[%s]' "$u"`, `[a\ba\b]`},
		{"a doubled backslash is not halved", `v='a\\b'; w=$v; printf '[%s]' "$w"`, `[a\\b]`},
		{"counted rather than printed", `v='a\b'; w=$v; printf '[%s]' "${#w}"`, `[3]`},
		{"a case subject", `v='a\b'; case $v in 'a\b') printf '[esc]';; ab) printf '[plain]';; *) printf '[none]';; esac`, `[esc]`},
		{"split into fields", `v='a\b'; set -- $v; printf '[%s]' "$@"`, `[a\b]`},
		{"an array element taken one at a time", `a=('x\y' 'p\q'); printf '[%s]' ${a[@]}`, `[x\y][p\q]`},
		{"a command substitution's result", `w=$(printf 'a\\b'); printf '[%s]' "$w"`, `[a\b]`},
		{"a trailing backslash", `v='a\'; w=$v; printf '[%s]' "$w"`, `[a\]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// What a backslash that arrived in a value does to the character behind it,
// once the field it is in is matched as a pattern. Three readings, and the
// directory is what tells them apart.
//
// Measured 2026-09-07 and again 2026-09-12 in a directory holding exactly
// `a\b`, `a*`, `a\bc` and `ab`. The names are chosen so that each reading has
// something of its own to find: a directory holding none of them prints the
// same word whichever rule is in force and could not tell the readings apart,
// which is what made an earlier row here pass with the axis wired to nothing
// (#1367, #1370).
func TestWhatAValueBackslashDoesToWhatFollowsIt(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{`a\b`, `a*`, `a\bc`, `ab`} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	for _, tc := range []struct {
		name, src             string
		quotes, disarms, data string
	}{
		{
			// The row that parts the quoting reading from the other two: the
			// backslash is not matched, so the pattern is `ab*`.
			"a backslash before an ordinary character, with a star beside it",
			`v='a\b*'; set -- $v; printf '[%s]' "$@"`,
			`[ab]`, `[a\b][a\bc]`, `[a\b][a\bc]`,
		},
		{
			// The metacharacter comes from a *literal* span rather than from
			// the value, which is why the reading cannot be chosen where the
			// value is escaped: the field is a word and this span is not all
			// of it.
			"the star written beside the expansion",
			`v='a\b'; set -- $v*; printf '[%s]' "$@"`,
			`[ab]`, `[a\b][a\bc]`, `[a\b][a\bc]`,
		},
		{
			// And the row that parts the data reading from the other two: the
			// `*` behind the backslash is live only there.
			"a backslash before a metacharacter",
			`v='a\*'; set -- $v; printf '[%s]' "$@"`,
			`[a\*]`, `[a\*]`, `[a\b][a\bc]`,
		},
		{
			// A failed match restores the word with the backslash still in
			// it, under every reading: a shell performs no quote removal on
			// the result of an expansion.
			"a pattern that matches nothing",
			`v='a\b[q]'; set -- $v; printf '[%s]' "$@"`,
			`[a\b[q]]`, `[a\b[q]]`, `[a\b[q]]`,
		},
		{
			// The doubled backslash. The first quotes the second, so the
			// quoting reading matches a name with one backslash in it where
			// the other two look for two.
			"a doubled backslash before a metacharacter",
			`v='a\\*'; set -- $v; printf '[%s]' "$@"`,
			`[a\b][a\bc]`, `[a\\*]`, `[a\\*]`,
		},
		{
			// The same four characters written *literally* are two
			// backslashes in every reading, which is what says the mark is
			// about provenance and not about the text.
			"the same text written literally",
			`set -- 'a\\b'*; printf '[%s]' "$@"`,
			`[a\\b*]`, `[a\\b*]`, `[a\\b*]`,
		},
		{
			// Nothing live in the field, so nothing is globbed and all three
			// restore the same word. This is the common shape and it asks
			// no question at all.
			"a backslash with no metacharacter anywhere",
			`v='a\b'; set -- $v; printf '[%s]' "$@"`,
			`[a\b]`, `[a\b]`, `[a\b]`,
		},
		{
			"a backslash at the end of the value",
			`v='a\'; set -- $v; printf '[%s]' "$@"`,
			`[a\]`, `[a\]`, `[a\]`,
		},
		{
			// A metacharacter with no backslash in front of it: the readings
			// have nothing to say and the glob is the ordinary one.
			"the metacharacter alone",
			`v='a*'; set -- $v; printf '[%s]' "$@"`,
			`[a*][a\b][a\bc][ab]`, `[a*][a\b][a\bc][ab]`, `[a*][a\b][a\bc][ab]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				policy ValueBackslashPolicy
				want   string
			}{
				{ValueBackslashQuotesWhatFollows, tc.quotes},
				{ValueBackslashDisarmsWhatFollows, tc.disarms},
				{ValueBackslashIsData, tc.data},
			} {
				sem := testSemantics()
				sem.ValueBackslashInAPattern = side.policy
				out, st := run(t, tc.src, func(r *Runner) {
					r.Semantics, r.Dir = &sem, dir
				})
				if out != side.want || st != 0 {
					t.Errorf("%v: %s = %q (status %d), want %q at 0",
						side.policy, tc.src, out, st, side.want)
				}
			}
		})
	}
}

// The axis is asked where the readings put different patterns on the wire,
// and nowhere else.
//
// Both halves are the placement. An unanswered vector has to report the rows
// the panel divides on, and has to stay quiet on the shapes around them —
// which are the ones a script is actually made of: a value with a backslash
// and nothing live beside it, a backslash at the end of one, a value with no
// backslash at all, and text written literally.
func TestTheValueBackslashAxisIsAskedOnlyWhereTheReadingsPart(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{`a\b`, `a*`} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	for _, tc := range []struct {
		name, src string
		refused   bool
		want      string
	}{
		{"before a metacharacter", `v='a\*'; set -- $v; printf '[%s]' "$@"`, true, ""},
		{"before an ordinary character with a star beside it", `v='a\b*'; set -- $v; printf '[%s]' "$@"`, true, ""},
		{"a star written beside the expansion", `v='a\b'; set -- $v*; printf '[%s]' "$@"`, true, ""},
		{"before an ordinary character", `v='a\b'; set -- $v; printf '[%s]' "$@"`, false, `[a\b]`},
		{"at the end of the value", `v='a\'; set -- $v; printf '[%s]' "$@"`, false, `[a\]`},
		{"no backslash at all", `v='xy'; set -- $v; printf '[%s]' "$@"`, false, `[xy]`},
		{"written literally", `set -- 'a\b'; printf '[%s]' "$@"`, false, `[a\b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.ValueBackslashInAPattern = ValueBackslashUnspecified
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			})
			if tc.refused {
				if !strings.Contains(out, "value's backslash") {
					t.Fatalf("got %q (status %d), want a refusal naming the axis", out, st)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Fatalf("got %q (status %d), want %q at 0 — the axis was asked "+
					"where the panel agrees", out, st, tc.want)
			}
		})
	}
}

// Where the result is never globbed, the axis is not asked at all: the three
// readings put the same text on the wire. Asserted with the axis left
// unanswered, which is what would report the question if it were still being
// put.
func TestTheValueBackslashAxisIsNotAskedWhereNothingIsGlobbed(t *testing.T) {
	out, st := axisRun(t, `v='a\*'; set -- $v; printf '[%s]' "$@"`, func(s *Semantics) {
		s.GlobExpansionResults = No
		s.ValueBackslashInAPattern = ValueBackslashUnspecified
	})
	if out != `[a\*]` || st != 0 {
		t.Fatalf("got %q (status %d), want %q at 0", out, st, `[a\*]`)
	}
}

// Where the dialect does not read an expansion result as a pattern, a value
// carrying a backslash *and* a live metacharacter still comes out as it went
// in.
//
// The branch its own test, because the two sides of `escapeResult` produce the
// same answer for everything narrower than this: a value with no live
// metacharacter never reaches the question, and one with no backslash is the
// same string on both sides. This is the shape that separates them, and the
// escaping has to be applied to the *value* rather than to the already-marked
// form — marking twice doubles what the unescape then halves once, so the
// backslash comes back multiplied instead of restored.
//
// Measured 2026-09-07, same conditions as above, in a directory holding
// `a\bc` and `ab`: zsh 5.9.2 prints `a\b*` for `v='a\b*'; set -- $v`, since
// it globs no expansion result at all.
func TestAValueBackslashSurvivesWhereTheResultIsNotAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a backslash beside a live metacharacter", `v='a\b*'; set -- $v; printf '[%s]' "$@"`, `[a\b*]`},
		{"a metacharacter with no backslash", `v='a*'; set -- $v; printf '[%s]' "$@"`, `[a*]`},
		{"a backslash with no metacharacter", `v='a\b'; set -- $v; printf '[%s]' "$@"`, `[a\b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := axisRun(t, tc.src, func(s *Semantics) { s.GlobExpansionResults = No })
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A value's backslash standing directly in front of a separator loses the
// separator and keeps itself.
//
// Two facts at once, and each is the other's guard. The separator still
// separates — a backslash that arrived in a value quotes nothing for this
// stage, which the whole panel agrees about — and the backslash is still in
// the field, because no shell removes quotes from the result of an expansion.
// So `IFS=:` over `a\:b` is two fields and a first field of **two**
// characters, and the two neighboring mistakes are one field and `a`.
//
// The count is asserted rather than the rendered field because that is the
// only thing that separates the right answer from the one this shell gave:
// `[a\]` and `[a\\]` differ by a character that reads as a quoting artifact
// wherever it is displayed. The splitter walked the escaped form a byte at a
// time, cut at the marked separator, and left the mark on the field in front
// of it, where the unescape read it as a marked backslash (#2212).
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin`, from a script file,
// against bash 5.3.15, bash-as-`sh`, bash 3.2.57, dash, ksh93u+ and zsh 5.9.2
// — the last under `setopt shwordsplit`, since it splits no parameter
// expansion otherwise and answers every row with the value whole.
func TestAValueBackslashBeforeASeparatorLosesOnlyTheSeparator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a marked separator cuts and leaves one backslash",
			`IFS=:; v='a\:b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a\:2][b]`,
		},
		{
			"the separator being whitespace changes nothing",
			`IFS=' '; v='a\ b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a\:2][b]`,
		},
		{
			"a doubled backslash is not halved",
			`IFS=:; v='a\\:b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a\\:3][b]`,
		},
		{
			"a backslash before an ordinary character is untouched",
			`IFS=:; v='a\bc:d'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a\bc:4][d]`,
		},
		{
			"at the leading edge the field is the backslash alone",
			`IFS=:; v='\:b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[\:1][b]`,
		},
		{
			// The trailing-separator axis is answered here because it is
			// live at this shape and is not what the row is about: one
			// reading absorbs the closing separator and the other opens a
			// field for it, and both have to keep the backslash.
			"at the trailing edge the separator is still absorbed",
			`IFS=:; v='a\:'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"`,
			`1[a\:2]`,
		},
		{
			"two separators behind it still leave the empty field between them",
			`IFS=:; v='a\::b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s][%s]' "$2" "$3"`,
			`3[a\:2][][b]`,
		},
		{
			"a mixed IFS separates on the marked byte and the plain one alike",
			`IFS=' :'; v='a\:b c'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s][%s]' "$2" "$3"`,
			`3[a\:2][b][c]`,
		},
		{
			"a word assembled around it keeps the text that follows",
			`IFS=:; a='x\:y'; b='q'; set -- $a$b; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[x\:2][yq]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := axisRun(t, tc.src, func(s *Semantics) {
				s.SplitParamExpansion = Yes
				s.TrailingSeparatorEndsAField = No
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The same rule where the dialect does not read an expansion result as a
// pattern, which is the pair of answers no preset holds and where a
// separator can also be a metacharacter.
//
// It is a case of its own because the marks come from the other producer
// there: with globbing off, the whole result is mark-escaped rather than only
// its backslashes, so an `IFS` holding `*` meets a separator the escape has
// marked for a reason that has nothing to do with the value's backslashes.
// The answer is the same — the mark goes with the separator — and the shell
// that reaches this pair by option agrees: zsh 5.9.2 under `setopt
// shwordsplit` with `IFS='*'` answers `2[a][b]` for `v='a*b'`, measured
// 2026-09-12.
func TestAMarkedSeparatorCutsWhereTheResultIsNotAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a separator that is also a metacharacter",
			`IFS='*'; v='a*b'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a:1][b]`,
		},
		{
			"and the value's own backslash beside it",
			`IFS='*'; v='a\*c'; set -- $v; printf '%d' "$#"; printf '[%s:%d]' "$1" "${#1}"; printf '[%s]' "$2"`,
			`2[a\:2][c]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := axisRun(t, tc.src, func(s *Semantics) {
				s.SplitParamExpansion = Yes
				s.GlobExpansionResults = No
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}
