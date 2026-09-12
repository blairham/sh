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
func crossing(dir string) func(*Runner) {
	return withOption(StarStarCrossesDirectories, true, inDir(dir))
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
