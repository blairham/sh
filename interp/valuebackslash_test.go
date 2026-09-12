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

// Whether the character a value's backslash precedes is a metacharacter, and
// the axis that decides it.
//
// The other half of the same marking, and it needs a directory because it is
// only observable against real names. Measured 2026-09-07 and again
// 2026-09-12 in a directory holding exactly `a\b` and `a*`: bash 5.3.15,
// bash-as-`sh`, bash 3.2.57, dash and zsh 5.9.2 all leave the word as `a\*`,
// matching neither name — so the `*` behind the backslash is not live and the
// backslash is still there to be printed. ksh93u+ takes the backslash as data
// and the `*` as live, and answers `a\b`.
//
// Both files are present on purpose: a directory holding neither would print
// `a\*` whatever the rule was, and could not tell the readings apart. Which
// is also why the assertion is over both answers of
// Semantics.ValueBackslashQuotesWhatFollows rather than over the majority
// one — a row that pinned only the five-shell reading would pass with the
// axis wired to nothing (#1367).
func TestAValueBackslashTakesTheMetacharacterOffWhatFollowsIt(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{`a\b`, `a*`} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	// The control runs in the same directory and against the same two names:
	// an unmarked `*` matches both, so a fix that simply stopped globbing
	// expansion results would pass the row above and fail this one.
	for _, tc := range []struct{ name, src, quotes, data string }{
		{"a backslash before the metacharacter", `v='a\*'; set -- $v; printf '[%s]' "$@"`, `[a\*]`, `[a\b]`},
		// The two readings answer these alike, which is what says the axis
		// belongs at the row above and nowhere near them.
		{"the metacharacter alone", `v='a*'; set -- $v; printf '[%s]' "$@"`, `[a*][a\b]`, `[a*][a\b]`},
		{"a backslash before an ordinary character", `v='a\b'; set -- $v; printf '[%s]' "$@"`, `[a\b]`, `[a\b]`},
		{"a backslash at the end of the value", `v='a\'; set -- $v; printf '[%s]' "$@"`, `[a\]`, `[a\]`},
		// The second backslash is what the first one quotes, so the `*` is
		// not behind a backslash at all and stays live under both readings.
		{"a doubled backslash in front of the metacharacter", `v='a\\*'; set -- $v; printf '[%s]' "$@"`, `[a\\*]`, `[a\\*]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.quotes}, {No, tc.data}} {
				out, st := run(t, tc.src, func(r *Runner) {
					sem := testSemantics()
					sem.ValueBackslashQuotesWhatFollows = side.answer
					r.Semantics, r.Dir = &sem, dir
				})
				if out != side.want || st != 0 {
					t.Errorf("%v: out = %q (status %d), want %q at 0", side.answer, out, st, side.want)
				}
			}
		})
	}
}

// The axis is asked where the two readings part and nowhere else.
//
// Both halves matter. An unanswered vector must refuse the row the panel
// divides on — otherwise the axis is decoration — and must **not** refuse the
// rows around it, which are unanimous across all six columns: a backslash
// before an ordinary character, one at the end of a value, and a value with
// no backslash in it at all.
func TestTheValueBackslashAxisIsAskedOnlyBeforeAMetacharacter(t *testing.T) {
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
		{"before an ordinary character", `v='a\b'; set -- $v; printf '[%s]' "$@"`, false, `[a\b]`},
		{"at the end of the value", `v='a\'; set -- $v; printf '[%s]' "$@"`, false, `[a\]`},
		{"no backslash at all", `v='xy'; set -- $v; printf '[%s]' "$@"`, false, `[xy]`},
		{"a doubled backslash before a metacharacter", `v='a\\*'; set -- $v; printf '[%s]' "$@"`, false, `[a\\*]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				sem := testSemantics()
				sem.ValueBackslashQuotesWhatFollows = Unspecified
				r.Semantics, r.Dir = &sem, dir
			})
			if tc.refused {
				// The refusal is what is asserted, not the status: the
				// `printf` that follows it in the snippet succeeds and the
				// shell's status is that command's.
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

// Where the result is never globbed, the axis is not asked at all: the two
// readings put the same text on the wire, so there is nothing to disagree
// about. Asserted with the axis left unanswered, which is what would report
// the question if it were still being put.
func TestTheValueBackslashAxisIsNotAskedWhereNothingIsGlobbed(t *testing.T) {
	out, st := axisRun(t, `v='a\*'; set -- $v; printf '[%s]' "$@"`, func(s *Semantics) {
		s.GlobExpansionResults = No
		s.ValueBackslashQuotesWhatFollows = Unspecified
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
