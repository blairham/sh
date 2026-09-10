// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `~` exclusion against the filesystem, which is the half that is *not*
// the matcher's: `pat~excl` is split off the whole field before it is cut
// into components, so the right side is compared with the whole word the left
// side produced rather than with one file's name (#1719).
//
// Measured on zsh 5.9.2, 2026-09-10, over the fixture below. It is the shape
// every real call site writes — `vcs_info` sweeps `$fpath` with
// `$dir/VCS_INFO_get_data_*~*(~|.zwc)(N)` — and refusing it by name put
// eighteen lines on the standard error of one interactive startup.
//
// The fixture discriminates on purpose. `p_git` matches the left side alone,
// `p_git.zwc` and `p_hg~` match both sides, and `other` matches neither, so a
// walk that dropped the exclusion, applied it to the wrong subject, or
// answered nothing at all gives three different wrong answers here.
func exclusionFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"d", "d/sub", "d/gxdir"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Lowercase throughout: a macOS filesystem folds case and a Linux one
	// does not, so a fixture that told two names apart by case would answer
	// differently on the two platforms this is tested on.
	for _, name := range []string{
		"d/p_git", "d/p_git.zwc", "d/p_hg~", "d/other",
		"d/sub/p_svn", "d/sub/p_svn.zwc", "d/gxdir/deep",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAGlobExclusionIsMatchedAgainstTheWholeWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The startup's own pattern, one directory down. The two files the
		// right side takes out are the ones a compiled or backed-up function
		// leaves beside the function itself.
		{`echo d/p_*~*(~|.zwc)`, "d/p_git"},
		// The decisive row: `*d*` matches nothing in any of these *names*
		// and matches every one of the *words*, because each begins with
		// `d/`. A walk running the right side against a file's name alone
		// would answer three files here.
		{`echo d/p_*~*d*(N)`, ""},
		// And the same exclusion from inside the directory keeps them, which
		// is the other half of the same claim: the subject is the word as
		// the pattern spelled it, not a path it could be cleaned into.
		{`cd d; echo p_*~*d*`, "p_git p_git.zwc p_hg~"},
		// A right side naming a directory: `/` is an ordinary character in
		// it, so it takes nothing out of words that have no `d/` in them.
		{`cd d; echo p_*~d/*`, "p_git p_git.zwc p_hg~"},
		// `**` crosses levels on the left and the exclusion still reads the
		// whole word: everything under `sub` goes, and `sub` appears in the
		// word rather than in the name.
		{`echo d/**/p_*~*sub*(N)`, "d/p_git d/p_git.zwc d/p_hg~"},
		// An exclusion inside a group is not a top-level one, and stays the
		// single component's own question.
		{`echo d/(*~sub)/*`, "d/gxdir/deep"},
		// The exclusion is taken out before the qualifiers narrow what is
		// left, rather than after.
		{`echo d/*~*(~|.zwc)(N/)`, "d/gxdir d/sub"},
		// A `~` straight after a `/` leaves the left side's last component
		// empty, and no file is named nothing. The trailing slash that means
		// "directories only" is the one at the end of the word.
		{`echo d/~*zzzz*(N)`, ""},
		// A `~` on its own does not make a word a pattern at all: no left
		// side is globbed and the eleven characters are printed as written.
		{`echo d/p_git~zzz`, "d/p_git~zzz"},
		// A left side with no metacharacter of its own is still a pattern
		// once the right side has one, and the exclusion can empty it.
		{`echo d/p_git~*git*(N)`, ""},
		{`echo d/p_git~*zzzz*`, "d/p_git"},
	} {
		out, st := runZsh(t, exclusionFixture(t), "setopt extendedglob\n"+tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// Everything the exclusion took out is still a miss, which is the same
// complaint an unmatched pattern earns and not a quieter one — measured,
// `no matches found` naming the whole field, `~` and all.
func TestAnExclusionThatEmptiesAPatternIsAMiss(t *testing.T) {
	out, st := runZsh(t, exclusionFixture(t), "setopt extendedglob\necho d/p_*~*d*\necho after")
	if !strings.Contains(out, "no matches found: d/p_*~*d*") || st == 0 {
		t.Errorf("an emptied exclusion = %q (status %d), want a miss naming the field", out, st)
	}
	if strings.Contains(out, "after") {
		t.Errorf("the script carried on past an unmatched pattern: %q", out)
	}
}
