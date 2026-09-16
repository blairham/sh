// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// starStarTree lays out a directory tree under a temporary root and returns
// it. Every name is two characters so that a `*` and a `?` both have
// something to be, which is what the separator rule is asked about.
func starStarTree(t *testing.T, dirs, files []string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// bareCrossing is the state every assertion here is about: `**` crosses
// levels and still crosses them with nothing behind it, which is bash's
// `globstar` and ksh93's. With the second option off a bare `**` is an
// ordinary `*` and no zero-level match arises at all.
//
// The last two are bash's answers to the questions ksh93 answers the other
// way, and they are written here rather than left to the zero value because
// this file is bash's reading throughout — see
// TestZeroLevelStarStarIsNamedByWhatStandsAheadOfItInKsh93 for the other one.
func bareCrossing(dir string) func(*Runner) {
	return withOption(StarStarCrossesDirectories, true,
		withOption(StarStarAloneCrossesDirectories, true,
			withOption(RepeatedStarStarIsOneComponent, true,
				withOption(StarStarZeroLevelIsTheDirectoryItStartsFrom, true,
					withOption(StarStarPatternsReadLinkedDirectories, true, inDir(dir))))))
}

// bareCrossingKsh93 is bareCrossing with the two options that shell answers
// differently turned back off: a zero-level `**` takes its match from what
// the component ahead of it listed, and a pattern holding a `**` reads no
// directory through a symbolic link.
func bareCrossingKsh93(dir string) func(*Runner) {
	return withOption(StarStarCrossesDirectories, true,
		withOption(StarStarAloneCrossesDirectories, true,
			withOption(RepeatedStarStarIsOneComponent, true, inDir(dir))))
}

// TestZeroLevelStarStarIsNamedByWhatStandsAheadOfIt.
//
// A `**` that stood for no level at all still names the directory it started
// from, and what it calls that directory is decided entirely by the
// components in front of it. Measured 2026-09-13 against bash 5.3.15 under
// `shopt -s globstar` in this fixture.
//
// The pairs are the point. `cx/**` and `c*/**` list the same three names and
// spell the first one differently, so the separator is a property of neither
// the directory nor the `**`.
func TestZeroLevelStarStarIsNamedByWhatStandsAheadOfIt(t *testing.T) {
	dir := starStarTree(t, []string{"cx/dx"}, []string{"ax", "cx/dx/ax"})
	for _, tc := range []struct{ name, src, want string }{
		{
			"a spelled prefix keeps the separator",
			`printf "[%s]" cx/**`, `[cx/][cx/dx][cx/dx/ax]`,
		},
		{
			"a star ahead of it drops the separator",
			`printf "[%s]" c*/**`, `[cx][cx/dx][cx/dx/ax]`,
		},
		{
			"a question mark ahead of it drops it too",
			`printf "[%s]" ?x/**`, `[cx][cx/dx][cx/dx/ax]`,
		},
		{
			"a bracket ahead of it drops it too",
			`printf "[%s]" [c]x/**`, `[cx][cx/dx][cx/dx/ax]`,
		},
		{
			// Quoting is not what decides it: the prefix still spells the
			// name out, and bash answers `cx/` here exactly as it does for
			// the unquoted spelling.
			"quoting the prefix is still spelling it",
			`printf "[%s]" "cx"/**`, `[cx/][cx/dx][cx/dx/ax]`,
		},
		{
			// An earlier `**` describes rather than spells, so the rule
			// composes with itself — and this is the row that says the
			// question is asked of the field as written, since folding the
			// run first leaves `cx/**`, which answers the opposite.
			"an earlier crossing drops it",
			`printf "[%s]" cx/**/**`, `[cx][cx/dx][cx/dx/ax]`,
		},
		{
			// The separators already standing are the ones reported. A
			// second one written here would be `cx///`.
			"a written separator run is reproduced and not added to",
			`printf "[%s]" cx//**`, `[cx//][cx//dx][cx//dx/ax]`,
		},
		{
			// And where the pattern asked for the slash itself there is
			// still only one, whatever the prefix was.
			"a trailing slash supplies it after a star as well",
			`printf "[%s]" c*/**/`, `[cx/][cx/dx/]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, bareCrossing(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestPathnameExpansionKeepsEveryRouteToAName.
//
// **A pathname expansion is not a set.** Two `**` components are two
// alternatives, each standing for zero or more levels, so a name reachable
// two ways is written twice. Measured 2026-09-13: `echo **/cx/**` in this
// fixture is `cx cx/cx cx/cx cx/cx/ax cx/cx/ax` in bash 5.3.15 under the
// option and in ksh93 under its own, and no column anywhere takes the
// duplicates out.
//
// The fixture nests `cx` inside itself on purpose: without a name that
// matches the middle component at two depths there is only one route to
// anything and the assertion cannot fail.
func TestPathnameExpansionKeepsEveryRouteToAName(t *testing.T) {
	dir := starStarTree(t, []string{"cx/cx"}, []string{"ax", "cx/cx/ax"})
	const want = `[cx][cx/cx][cx/cx][cx/cx/ax][cx/cx/ax]`
	out, _ := run(t, `printf "[%s]" **/cx/**`, bareCrossing(dir))
	if out != want {
		t.Errorf("**/cx/** = %s, want %s", out, want)
	}
}

// TestRunOfStarStarIsOneComponentOnlyWhereTheDialectSaysSo is the axis, and
// it is visible only because of the test above: with duplicates surviving, a
// run of `**` components is a cross product, and whether a shell has one is
// what this asks.
//
// Measured 2026-09-13 in this fixture: `echo **/**/` is `cx/ cx/dx/` in bash
// 5.3.15 under `shopt -s globstar` and in ksh93 under `set -o globstar`,
// against `cx/ cx/ cx/dx/ cx/dx/ cx/dx/` in zsh — one copy per way of
// splitting the path between the two components.
func TestRunOfStarStarIsOneComponentOnlyWhereTheDialectSaysSo(t *testing.T) {
	dir := starStarTree(t, []string{"cx/dx"}, []string{"ax", "cx/dx/ax"})
	for _, tc := range []struct {
		name string
		one  bool
		want string
	}{
		{"a run is one component", true, `[cx/][cx/dx/]`},
		{"a run is a cross product", false, `[cx/][cx/][cx/dx/][cx/dx/][cx/dx/]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := withOption(RepeatedStarStarIsOneComponent, tc.one,
				withOption(StarStarCrossesDirectories, true,
					withOption(StarStarAloneCrossesDirectories, true, inDir(dir))))
			out, _ := run(t, `printf "[%s]" **/**/`, opt)
			if out != tc.want {
				t.Errorf("**/**/ = %s, want %s", out, tc.want)
			}
		})
	}
}

// TestRunOfStarStarStopsAtTheNextComponent is the control for the fold, and
// it is what a fold reaching one component too far would fail: `**/**/ax` is
// `ax cx/dx/ax` in bash 5.3.15, which is what `**/ax` answers — the trailing
// `ax` is not inside the run and is not eaten by it.
//
// The trailing separator of the test above is the same question a second way:
// the empty component a pattern ends with follows no further `**`, so it
// still writes its own slash rather than disappearing into the run.
func TestRunOfStarStarStopsAtTheNextComponent(t *testing.T) {
	dir := starStarTree(t, []string{"cx/dx"}, []string{"ax", "cx/dx/ax"})
	for _, tc := range []struct{ name, src, want string }{
		{"a real component behind the run", `printf "[%s]" **/**/ax`, `[ax][cx/dx/ax]`},
		{"three of them fold to one as well", `printf "[%s]" **/**/**/ax`, `[ax][cx/dx/ax]`},
		{"a separator inside the run folds with it", `printf "[%s]" **//**/ax`, `[ax][cx/dx/ax]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, bareCrossing(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestZeroLevelStarStarIsNamedByWhatStandsAheadOfItInKsh93 is the other
// reading of the same question, and it is a different question rather than
// the one above negated — which is the whole reason it is an option and not
// a `!`.
//
// bash asks whether the path ahead of the `**` was **spelled**, and writes a
// separator where it was. ksh93 asks where the starting directory's name
// **came from**, and reports it only where a listing produced it: a literal
// component is joined onto the path and stat'd, a pattern is resolved by
// reading the directory it sits in, and a literal that follows a `**` is a
// listing too, because that walk reads every level it crosses on the way
// past.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01 under `set -o globstar` in this
// fixture, `[…]` per word throughout. The pair that makes the rule provable
// is `*/q/**` against `**/q/**`: both spell a literal `q`, both hold a
// pattern ahead of it, and they differ.
func TestZeroLevelStarStarIsNamedByWhatStandsAheadOfItInKsh93(t *testing.T) {
	dir := starStarTree(t,
		[]string{"p/q/w"},
		[]string{"topf", "p/pf", "p/q/qf", "p/q/w/leaf"})
	for _, tc := range []struct{ name, src, want string }{
		{
			// Spelled out, so nothing listed the starting directory and
			// there is no zero-level match at all. bash writes `[p/]` here.
			"a spelled prefix has no zero-level match",
			`printf "[%s]" p/**`, `[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// A pattern listed it, so the listing is the match — `topf` and
			// all, which is a plain file and could never be a directory the
			// walk stood in. That word is what says the match is the
			// listing rather than the walk's own position.
			"a pattern ahead of it reports the whole listing",
			`printf "[%s]" */**`,
			`[p][p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf][topf]`,
		},
		{
			"deeper, and still the listing",
			`printf "[%s]" p/*/**`, `[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// The literal was joined onto what the `*` matched, so nothing
			// listed it. bash writes `[p/q]` here.
			"a literal behind a pattern was joined and not listed",
			`printf "[%s]" */q/**`, `[p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// And the same literal after a `**`, which *is* a listing: the
			// walk read every level it crossed to find it.
			"a literal behind a crossing was listed on the way past",
			`printf "[%s]" **/q/**`, `[p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// One literal further on and it is a join again, which is the
			// row that keeps the rule from being read as "a `**` anywhere
			// ahead of it".
			"a second literal is a join again",
			`printf "[%s]" **/q/w/**`, `[p/q/w/leaf]`,
		},
		{
			// Read after the run collapses, unlike the separator question
			// next door: `p/**/**` is one `**` by the time the walk runs, so
			// the component ahead of it is the literal `p`. bash writes
			// `[p]` here, from the same field read before the collapse.
			"the run collapses before this is asked",
			`printf "[%s]" p/**/**`, `[p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]`,
		},
		{
			// A trailing slash still writes its one slash, and the listing
			// is filtered to directories by the empty component behind it.
			"a trailing slash keeps the directories out of the listing",
			`printf "[%s]" */**/`, `[p/][p/q/][p/q/w/]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, bareCrossingKsh93(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}
