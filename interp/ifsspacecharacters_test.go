// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// ifsSpace runs src with Semantics.IFSWhitespaceIsEverySpaceCharacter under
// the test's control and the neighboring tail questions answered flat.
//
// The tail axes matter because a closing separator is what several of these
// lines end on, and a run of them left unanswered would refuse before this
// question was reached.
func ifsSpace(t *testing.T, src string, every interp.Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.LastPipelineElementInCurrentShell = interp.Yes
		sem.TrailingSeparatorEndsAField = interp.No
		sem.ReadTrailingWhitespaceEndsAField = interp.No
		sem.ReadNoFieldsIsOneEmptyElement = interp.No
		sem.SplitParamExpansion = interp.Yes
		sem.IFSWhitespaceIsEverySpaceCharacter = every
		r.Semantics = &sem
	})
}

const countArgs = `printf 'n=%s' "$#"; for e in "$@"; do printf '[%s]' "$e"; done`

// The three characters POSIX leaves out of its own rule — the vertical tab,
// the form feed and the carriage return — either are the whitespace half of
// IFS or are ordinary separators that delimit once each and are never
// absorbed.
func TestTheWhitespaceHalfOfIFSIsTheDialectsOwn(t *testing.T) {
	for _, tc := range []struct{ name, src, three, every string }{
		{
			// A run of two with nothing between them: one delimiter where
			// the character is whitespace, two where it is not.
			"a run of vertical tabs",
			`IFS=$(printf '\v'); v=$(printf 'a\v\vb'); set -- $v; ` + countArgs,
			`n=3[a][][b]`, `n=2[a][b]`,
		},
		{
			"a run of carriage returns",
			`IFS=$(printf '\r'); v=$(printf 'a\r\rb'); set -- $v; ` + countArgs,
			`n=3[a][][b]`, `n=2[a][b]`,
		},
		{
			"a run of form feeds",
			`IFS=$(printf '\f'); v=$(printf 'a\f\fb'); set -- $v; ` + countArgs,
			`n=3[a][][b]`, `n=2[a][b]`,
		},
		{
			// The edges: a leading separator opens a field where the
			// character is not whitespace, and both ends are discarded
			// where it is.
			"a vertical tab at each end",
			`IFS=$(printf '\v'); v=$(printf '\va\v'); set -- $v; ` + countArgs,
			`n=2[][a]`, `n=1[a]`,
		},
		{
			// The control. Space is the whitespace half under either
			// answer, so a line that never leaves the POSIX three cannot
			// tell them apart.
			"a run of spaces",
			`v='a  b'; set -- $v; ` + countArgs,
			`n=2[a][b]`, `n=2[a][b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := ifsSpace(t, tc.src, interp.No); out != tc.three || st != 0 {
				t.Errorf("POSIX's three: got %q status %d, want %q at 0", out, st, tc.three)
			}
			if out, st := ifsSpace(t, tc.src, interp.Yes); out != tc.every || st != 0 {
				t.Errorf("every space character: got %q status %d, want %q at 0", out, st, tc.every)
			}
		})
	}
}

// The same answer reaches `read`, which is where #4170 found it: the last
// name takes the remainder of the line with the closing run of IFS whitespace
// off it, and which characters that run may hold is this question.
func TestTheWhitespaceHalfOfIFSReachesReadsRemainder(t *testing.T) {
	// IFS is every space character but the space itself, so the leading
	// spaces of the first field survive and the tail is a run of the three
	// at issue with a space inside it.
	src := `IFS=$(printf '\t\n\v\f\r'); printf '  line\tb \t\r\f\v\n' | ` +
		`{ read -r v1 v2; printf 'v1=[%s] v2=[%s]' "$v1" "$v2"; }`
	if out, st := ifsSpace(t, src, interp.No); out != "v1=[  line] v2=[b \t\r\f\v]" || st != 0 {
		t.Errorf("POSIX's three: got %q status %d", out, st)
	}
	if out, st := ifsSpace(t, src, interp.Yes); out != "v1=[  line] v2=[b ]" || st != 0 {
		t.Errorf("every space character: got %q status %d", out, st)
	}
}

// The guard, and the half two answered runs cannot show: a separator set with
// none of the three characters in it reaches no question at all, so every
// ordinary script splits under an unanswered vector without a word.
func TestAnOrdinaryIFSAsksNothingAboutItsWhitespaceHalf(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the default separators",
			`v='  a  b  '; set -- $v; ` + countArgs,
			`n=2[a][b]`,
		},
		{
			"a colon",
			`IFS=:; v='a::b'; set -- $v; ` + countArgs,
			`n=3[a][][b]`,
		},
		{
			// Written with the newline first: a command substitution eats
			// the trailing ones, so `$(printf '\n')` alone would set IFS to
			// nothing at all and measure the wrong rule.
			"a newline, which is whitespace in every column",
			`IFS=$(printf '\nx'); v=$(printf 'a\n\nb'); set -- $v; ` + countArgs,
			`n=2[a][b]`,
		},
		{
			"read with the default separators",
			`printf '  a  b  \n' | { read -r x y; printf '[%s][%s]' "$x" "$y"; }`,
			`[a][b]`,
		},
		{
			// IFS set and empty disables splitting outright, so nothing is
			// whitespace and nothing is asked.
			"an empty IFS",
			`IFS=; v=$(printf 'a\v\vb'); set -- $v; ` + countArgs,
			`n=1[a` + "\v\v" + `b]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := ifsSpace(t, tc.src, interp.Unspecified)
			if out != tc.want || st != 0 {
				t.Errorf("unanswered: got %q status %d, want %q at 0 with nothing said", out, st, tc.want)
			}
		})
	}
}

// `[[:IFSSPACE:]]` reads the same half, so a pattern asking which separators
// are whitespace gets the dialect's answer rather than a fixed three.
func TestTheIFSSpaceClassReadsTheDialectsHalf(t *testing.T) {
	src := `IFS=$(printf '\v'); c=$(printf '\v'); case $c in [[:IFSSPACE:]]) printf hit;; *) printf miss;; esac`
	run := func(every interp.Answer) (string, int) {
		t.Helper()
		return runGrammar(t, src, nil, func(r *interp.Runner) {
			sem := interp.CoreSemantics()
			sem.PatternClasses = "IFS IFSSPACE"
			sem.IFSWhitespaceIsEverySpaceCharacter = every
			r.Semantics = &sem
		})
	}
	if out, st := run(interp.No); out != "miss" || st != 0 {
		t.Errorf("POSIX's three: got %q status %d, want miss at 0", out, st)
	}
	if out, st := run(interp.Yes); out != "hit" || st != 0 {
		t.Errorf("every space character: got %q status %d, want hit at 0", out, st)
	}
}
