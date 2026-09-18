// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// starStarLinkDir holds a real directory `r` with a file in it, a symbolic link `s`
// pointing at `r`, a file `y` at the top and a link `up` pointing at the
// directory itself.
//
// The last one is the shape the whole restriction is for: a link to an
// ancestor makes a walk that follows links unbounded, and nothing about the
// names says so.
func starStarLinkDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "r"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"r/x", "y"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("r", filepath.Join(dir, "s")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(dir, "up")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// crossing turns the level-crossing reading on, which is the state every
// assertion below is about: with it off `**` is an ordinary `*` and the
// question does not arise.
//
// And with them, the two readings bash and zsh share and ksh93 does not: a
// link the *pattern* reached is listed like any other directory, and a `**`
// that matched no level at all names the directory it started from — `s/**/`
// is `[s/]` in those two and no match in ksh93, which will not begin a walk
// behind a link. This file is those two shells throughout —
// see TestAStarStarPatternReadsNoLinkedDirectoryWhereTheDialectSaysSo for the
// third column.
func crossing(dir string) func(*Runner) {
	return withOption(StarStarCrossesDirectories, true,
		withOption(StarStarPatternsReadLinkedDirectories, true,
			withOption(StarStarZeroLevelIsTheDirectoryItStartsFrom, true, inDir(dir))))
}

// TestStarStarDoesNotEnterASymbolicLink is #2360.
//
// Measured 2026-09-12 in this fixture: `echo **/x` is `r/x` in bash 5.3.15
// under `shopt -s globstar`, in ksh93 under `set -o globstar` and in zsh,
// which has the crossing with no option at all. Where the option is absent —
// bash 3.2, dash — `**` is an ordinary `*` and the answer is `r/x s/x`, which
// is what this walk used to give with the crossing on as well.
//
// The assertion is on the strings rather than on a count, because the extra
// path is a *second name for a file already matched*: a count says two and
// says nothing about which two.
func TestStarStarDoesNotEnterASymbolicLink(t *testing.T) {
	dir := starStarLinkDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The reproduction, in its shortest form.
		{"a link to a sibling directory", `printf "[%s]" **/x`, `[r/x]`},
		// And the contrast that shows it is the walk and not the fixture:
		// an ordinary component names the link and goes through it, here
		// and in all six columns.
		{"an ordinary component still follows one", `printf "[%s]" s/*`, `[s/x]`},
		// Two components after the `**`, which is the shape that proves the
		// restriction belongs to `**` rather than to descent in general:
		// `*` matches the link and the next `*` looks inside it, both of
		// them ordinary components. Measured 2026-09-12, `**/*/*` is
		// `r/x s/x up/r up/s up/up up/y` in bash 5.3.15 with the option and
		// in zsh — the walk reaches through both links exactly one level,
		// because that is how far ordinary components were written to go.
		{
			"a component after it may follow one",
			`printf "[%s]" **/*/*`,
			`[r/x][s/x][up/r][up/s][up/up][up/y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, crossing(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestStarStarIsBoundedByALinkToItsOwnAncestor is the reason the rule is
// worth a P2 rather than a curiosity. `up` points at the directory holding
// it, so a walk that followed it would keep finding the same files under
// longer and longer names; what stopped it here was a depth limit, so the
// cost was exponential in link density before it was anything else.
//
// Measured: `echo **/y` is `y` in bash 5.3.15 with the option and in zsh.
// The test asserts the answer and not merely that the call returned, because
// a depth limit also returns.
func TestStarStarIsBoundedByALinkToItsOwnAncestor(t *testing.T) {
	dir := starStarLinkDir(t)
	done := make(chan string, 1)
	go func() {
		out, _ := run(t, `printf "[%s]" **/y`, crossing(dir))
		done <- out
	}()
	select {
	case out := <-done:
		if out != `[y]` {
			t.Errorf("**/y = %s, want [y]", out)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("**/y did not finish: the walk is following the link to its own ancestor")
	}
}

// TestStarStarStartsWhereThePatternSaysEvenThroughALink: the restriction is
// about where `**` descends **to**, not about where a pattern says to begin.
// Measured, `s/**/x` is `s/x` in bash 5.3.15 with the option and in zsh —
// the component's own starting directory is one of the levels it stands for
// however it was reached.
//
// This is the mirror of the test above and it is not redundant: the cheapest
// wrong fix is to drop every link from the set, which passes every assertion
// there and answers nothing here.
func TestStarStarStartsWhereThePatternSaysEvenThroughALink(t *testing.T) {
	dir := starStarLinkDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a component behind it", `printf "[%s]" s/**/x`, `[s/x]`},
		{"nothing behind it", `printf "[%s]" s/**/`, `[s/]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, crossing(dir))
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestStarStarSlashListsALinkOnlyWhereTheDialectSaysSo is the axis, and it is
// the one question about `**` and links where the panel splits. Measured
// 2026-09-12 in this fixture: `echo **/` is `r/ s/ up/` in bash 5.3.15 under
// `shopt -s globstar` and in ksh93 under `set -o globstar`, where `**`
// matches the entries beneath a directory and the trailing slash keeps the
// ones that are directories; and `r/` in zsh, where the component stands for
// the levels the walk crossed and a link is not one of them.
//
// Neither shell **enters** the link, which is what makes this a second
// question rather than the first one asked again: `**/x` is `r/x` in both.
func TestStarStarSlashListsALinkOnlyWhereTheDialectSaysSo(t *testing.T) {
	dir := starStarLinkDir(t)
	for _, tc := range []struct {
		name string
		sees bool
		want string
	}{
		{"seeing links", true, `[r/][s/][up/]`},
		{"not seeing them", false, `[r/]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := withOption(StarStarSeesLinkedDirectories, tc.sees, crossing(dir))
			out, _ := run(t, `printf "[%s]" **/`, opt)
			if out != tc.want {
				t.Errorf("**/ = %s, want %s", out, tc.want)
			}
		})
	}
}

// TestStarStarSeeingALinkStillDoesNotEnterIt: the option above says what
// `**/` lists and must say nothing about where the walk goes. Without this,
// turning it on to answer bash's `**/` would quietly reopen the bug it was
// added beside.
func TestStarStarSeeingALinkStillDoesNotEnterIt(t *testing.T) {
	dir := starStarLinkDir(t)
	opt := withOption(StarStarSeesLinkedDirectories, true, crossing(dir))
	if out, _ := run(t, `printf "[%s]" **/x`, opt); out != `[r/x]` {
		t.Errorf("**/x = %s, want [r/x]", out)
	}
}

// TestAStarStarPatternReadsNoLinkedDirectoryWhereTheDialectSaysSo is the
// third column, and it is a fact about the whole **pattern** rather than
// about the `**` component: one shell answers a field holding a
// level-crossing `**` with a physical walk and reads no directory through a
// link anywhere in it — while the same shell, with the same option still set,
// reads straight through that link for a field with no `**` in it.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01 under `set -o globstar` in this
// fixture, against bash 5.3.20 under `shopt -s globstar` and zsh 5.9.2.
func TestAStarStarPatternReadsNoLinkedDirectoryWhereTheDialectSaysSo(t *testing.T) {
	dir := starStarLinkDir(t)
	// The crossing on and the link reading off, which is the ksh93 pair. The
	// zero-level option is off with it, so `s/**` has no self match to fall
	// back on either — and that is why the row below is a miss and not `[s/]`.
	physical := withOption(StarStarCrossesDirectories, true, inDir(dir))
	for _, tc := range []struct{ name, src, want string }{
		{
			// The walk will not begin behind the link, so the pattern is
			// the word. bash answers `[s/x]` for this.
			"a walk does not begin behind a link", `printf "[%s]" s/**/x`, `[s/**/x]`,
		},
		{
			// Nor where a pattern put it there rather than a spelling: the
			// `s` and `up` the `*` matched are both links, so only `r` is
			// walked. Measured — ksh93 answers `[r/x]` where bash answers
			// `[r/x][s/x][up/r/x][up/s/x]` and zsh `[r/x][s/x][up/r/x]`.
			"however the link was reached", `printf "[%s]" */**/x`, `[r/x]`,
		},
		{
			// A **literal** component is joined onto the path and stat'd
			// rather than listed, so it reaches through the same link in the
			// same pattern — the row that keeps this from being read as
			// "drop every link from the set".
			"a literal still reaches through one", `printf "[%s]" */x`, `[r/x][s/x]`,
		},
		{
			// And with no `**` in the word at all the link is read like any
			// other directory, with the option still on. This is the row
			// that makes it the pattern's property rather than the option's.
			"a pattern with no crossing in it reads the link",
			`printf "[%s]" s/*`, `[s/x]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, physical)
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestAComponentBehindAStarStarLooksInsideALinkedLevelWhereTheDialectSaysSo
// is #3176, and it is the half the option above was measured too narrow for.
// That one says whether a linked level is **named** by `**/`; this one says
// whether the component *behind* a `**` may look **inside** one. The two are
// not the same answer: the panel column that names such a level for `**/`
// and then refuses to look inside it exists, which is what makes this a row.
//
// Measured 2026-09-18 in this fixture, which holds a link `s` to `r` one
// level down from the walk's start: `./**/x` is `./r/x ./s/x` in bash 5.3.20
// under `shopt -s globstar` and `./r/x` in ksh93u+ under `set -o globstar`
// and in zsh — while `**/` names `s/` in the first two and not in the third.
//
// The walk is untouched either way, and the assertion below says so: with the
// option on, a link to the directory's own ancestor is still never descended
// through, so the answer is finite and is the same one.
func TestAComponentBehindAStarStarLooksInsideALinkedLevelWhereTheDialectSaysSo(t *testing.T) {
	dir := starStarLinkDir(t)
	for _, tc := range []struct {
		name string
		sees bool
		src  string
		want string
	}{
		// The reproduction. A leading `**` is the one shape the option is
		// not asked of, so the pattern writes the `.` that makes the
		// component a component behind one — which is the whole measured
		// difference in the column that has this on.
		{"looking inside", true, `printf "[%s]" ./**/x`, `[./r/x][./s/x]`},
		{"not looking inside", false, `printf "[%s]" ./**/x`, `[./r/x]`},
		// And the exemption, which is measured rather than chosen: the very
		// first component of the word, with one separator behind it, is
		// answered the other way in the same shell over the same files.
		{"a star-star that begins the word", true, `printf "[%s]" **/x`, `[r/x]`},
		{"the same, with the option off", false, `printf "[%s]" **/x`, `[r/x]`},
		// The set the next component is offered is wider; the set the walk
		// enters is not. `up` points at the directory holding it, so a walk
		// that had started following links would not come back at all.
		{"the walk stays bounded", true, `printf "[%s]" ./**/y`, `[./up/y][./y]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := withOption(ComponentBehindStarStarSeesLinkedLevels, tc.sees,
				withOption(StarStarSeesLinkedDirectories, true, crossing(dir)))
			out, _ := run(t, tc.src, opt)
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}
