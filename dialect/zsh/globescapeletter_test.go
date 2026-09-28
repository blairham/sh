// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A backslash the script wrote in front of an ordinary letter is **quoting**
// and does not reach the field, and this is the column that says so where it
// is cheapest to break.
//
// #5012 gave ksh93's `~(K)` group a written `\d` back — the backslash now
// survives field construction so that the matcher can read the class — and
// the whole of what keeps that out of every other shell is the gate on the
// word carrying such a group. Widen it and this is what fails first: a field
// is only read as a pattern when a live metacharacter is in it, and the
// escape set here is narrow enough that a surviving backslash is then a
// character of its own rather than a quoting mark, so the glob asks for a
// name with a backslash in it and finds none.
//
// `echo a\db*` naming `adb` is zsh 5.9.2's answer and ksh93u+'s alike; what
// is being pinned is not the row but the reach of somebody else's rule.
func TestAWrittenBackslashIsQuotingAndNotAFieldCharacter(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"adb", "a1b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		// The live star is what makes the field a pattern, and so what makes
		// a stray backslash in it visible at all.
		{"a class letter beside a live star", `echo a\db*`, "adb\n"},
		{"and beside a live bracket", `echo a\d[b]`, "adb\n"},
		// The controls: the same word without the star spells a name, and
		// the same star without the backslash is an ordinary glob.
		{"with no star it spells the name", `echo a\db`, "adb\n"},
		{"and with no backslash it is a glob", `echo adb*`, "adb\n"},
		{"which matches more than one", `echo a?b*`, "a1b adb\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: dir}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
