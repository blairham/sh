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

// slashDir holds cx/dx, a plain file ax, a second directory ax_dir, a
// symbolic link to a directory and a symbolic link to a file — the five
// shapes the trailing slash has to tell apart.
func slashDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cx", "dx"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "ax_dir"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ax", "cx/ax", "cx/dx/ax"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("cx", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("ax", filepath.Join(dir, "symf")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestATrailingSlashStaysOnEveryMatch is #1350.
//
// `*/` is the standard spelling of "the directories here", and every shell in
// the panel answers it with the slash still attached: bash 5.3, that same
// binary as sh, bash 3.2, dash, ksh93 and zsh all print `ax_dir/ cx/ sym/`
// where this walk printed `ax_dir cx sym`. Unanimous, so it is the core that
// was wrong rather than any dialect, and the fix is one rule in the shared
// glob path.
//
// The wrong answer is quiet, which is the reason it lasted: `for d in */; do
// cd "$d"; done` works either way, and only something that joins the name to
// a second path or compares the two spellings — `rsync src/ dst`,
// `[ "$d" = "$other" ]` — can tell. So the assertions here are on the
// strings. A count is not evidence: three matches come back three matches
// whether or not any of them kept its slash.
func TestATrailingSlashStaysOnEveryMatch(t *testing.T) {
	dir := slashDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The reproduction. Directories only, and each with its slash — the
		// filtering was already right and only the slash was missing.
		{"bare", `printf "[%s]" */`, `[ax_dir/][cx/][sym/]`},
		// The same pattern without the slash, so the row above is read as a
		// difference the slash makes rather than as the listing.
		{"without the slash", `printf "[%s]" *`, `[ax][ax_dir][cx][sym][symf]`},
		// A literal prefix in front of it, and a slash in the middle as well
		// as at the end.
		{"under a literal component", `printf "[%s]" cx/*/`, `[cx/dx/]`},
		{"a slash in the middle too", `printf "[%s]" */*/`, `[cx/dx/][sym/dx/]`},
		{"a literal last component", `printf "[%s]" */dx/`, `[cx/dx/][sym/dx/]`},
		// A mid-pattern slash on its own changes nothing, which is what says
		// the rule is about the *end* of the word.
		{"a middle slash alone", `printf "[%s]" */*`, `[cx/ax][cx/dx][sym/ax][sym/dx]`},
		// Some matches are directories and some are not: `ax` is a file and
		// `ax_dir` is not, so the slash both filters and is written back.
		{"only the directory matches", `printf "[%s]" a*/`, `[ax_dir/]`},
		// A symbolic link to a directory is a directory here — all six
		// columns list `sym/`, and the link to a file is not listed at all.
		{"a link to a directory counts", `printf "[%s]" sym*/`, `[sym/]`},
		// Nothing matched, so the word stands exactly as written, slash and
		// all. The default is the pass-through dialects'; the erroring one
		// is its own axis and is asserted through the option below.
		{"a miss keeps the pattern whole", `printf "[%s]" zz*/`, `[zz*/]`},
		// Quoting the slash does not change it: the panel answers `*"/"`
		// and `*/` identically, so the slash is text either way.
		{"a quoted slash is the same slash", `printf "[%s]" *"/"`, `[ax_dir/][cx/][sym/]`},
		// The run is reproduced as written rather than normalized to one.
		// dash, ksh93 and zsh answer `cx//` and `cx///`; bash alone
		// collapses, which is a divergence the corpus records and this does
		// not implement.
		{"two slashes come back as two", `printf "[%s]" *//`, `[ax_dir//][cx//][sym//]`},
		{"three come back as three", `printf "[%s]" *///`, `[ax_dir///][cx///][sym///]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestATrailingSlashOnAnAbsolutePatternToo: the walk reports an absolute
// pattern's matches absolute and a relative one's relative, and those are two
// different lines of code — so the slash has to be written back on both.
func TestATrailingSlashOnAnAbsolutePatternToo(t *testing.T) {
	dir := slashDir(t)
	want := "[" + filepath.Join(dir, "cx") + "/]"
	if got := runIn(t, dir, `printf "[%s]" `+dir+`/c*/`); got != want {
		t.Errorf("absolute = %s, want %s", got, want)
	}
}

// TestATrailingSlashSurvivesTheStarStarSelfDirectory: `**` as the last
// component reports the directory it started from with a separator of its
// own, and a pattern ending in `/` asks for one too. Two sources, one slash:
// bash with the option and zsh without one both answer `cx/ cx/dx/` for
// `cx/**/`, never `cx// cx/dx/`.
func TestATrailingSlashSurvivesTheStarStarSelfDirectory(t *testing.T) {
	dir := slashDir(t)
	opt := withOption(StarStarCrossesDirectories, true,
		withOption(StarStarAloneCrossesDirectories, true, inDir(dir)))
	for _, tc := range []struct{ name, src, want string }{
		{"with a trailing slash", `printf "[%s]" cx/**/`, `[cx/][cx/dx/]`},
		// Without one, the self directory still carries the separator it
		// always did — the row that keeps the guard above from being read as
		// "drop the self slash".
		{"without one", `printf "[%s]" cx/**`, `[cx/][cx/ax][cx/dx][cx/dx/ax]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, opt)
			if out != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, out, tc.want)
			}
		})
	}
}

// TestATrailingSlashOnAMissThatIsAnError is a guard rather than evidence: it
// passed before the fix too. The walk now splits a trailing slash run off the
// field, and the diagnostic must keep naming the word as it was written —
// `zz*/` and not `zz*` — which is a thing the split could plausibly break and
// nothing else here would notice.
func TestATrailingSlashOnAMissThatIsAnError(t *testing.T) {
	dir := slashDir(t)
	out, _ := run(t, `printf "[%s]" zz*/`, func(r *Runner) {
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobNoMatchIsError = Yes
		r.Semantics = &sem
	})
	if want := "no matches found: zz*/"; !strings.Contains(out, want) {
		t.Errorf("diagnostic = %q, want it to hold %q", out, want)
	}
}
