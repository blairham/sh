// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnEmptyQualifierSectionCountsOnlyLast is #5995: a section with no test
// in it lets every name through when it is the last section, and is ignored
// anywhere else. Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh,
// -f) against the directory qualifierDir builds: `d1/`, `f1`, `f2`, a link
// `l1` and `.dot`.
func TestAnEmptyQualifierSectionCountsOnlyLast(t *testing.T) {
	dir := qualifierDir(t)
	for _, tc := range []struct{ src, want string }{
		{`print -r -- *(,/)`, "d1"},
		{`print -r -- *(N,/)`, "d1"},
		{`print -r -- *(on,/)`, "d1"},
		{`print -r -- *(D,.)`, ".dot f1 f2"},
		{`print -r -- *(^,/)`, "d1"},
		{`print -r -- *(/,,.)`, "d1 f1 f2"},
		{`print -r -- *([1],/)`, "d1"},
		// The last section, empty, still lets everything through — and the
		// section a `:` ends is the last one.
		{`print -r -- *(/,)`, "d1 f1 f2 l1"},
		{`print -r -- *(,)`, "d1 f1 f2 l1"},
		{`print -r -- *(.,/,)`, "d1 f1 f2 l1"},
		{`print -r -- *(/,:t)`, "d1 f1 f2 l1"},
		{`print -r -- *(.,/)`, "d1 f1 f2"},
	} {
		out, st := runZsh(t, dir, "export LC_ALL=C\ncd "+dir+"\n"+tc.src)
		if out != tc.want+"\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
