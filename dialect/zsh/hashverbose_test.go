// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `hash -v` reports every entry the call **answered**, in the same
// `name=value` shape the bare listing writes — #4964.
//
// Measured 2026-09-27 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`) with `-f`. Three
// of the rows below are the ones that decide what the letter means, and each
// rules out a reading that would otherwise pass:
//
//   - **a query prints.** `hash -d a=/tmp; hash -dv a` writes `a=/tmp`
//     although the second call adds nothing, so this is not "report what was
//     added".
//   - **a failure prints nothing extra.** `hash -v` on a name that resolves to
//     nothing is the ordinary complaint at 1 and no entry line, so the letter
//     never invents a line for a call that did not succeed.
//   - **`-L` does not shape it.** `hash -dvL a=/tmp` is `a=/tmp` where
//     `hash -dL` with no operand is `hash -d a=/tmp`, so `-L` shapes a
//     *listing* and the per-operand report is not one. A verbose report that
//     inherited `asCommands` from the listing beside it would fail here and
//     nowhere else in this file.
func TestHashVerboseReportsTheEntriesItAnswered(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want string // `%s` is the scratch directory
		st   int
	}{
		{"a search is remembered and reported", "hash -v g\n", "g=%s/g\n", 0},
		{"one line per operand", "hash -v g h\n", "g=%s/g\nh=%s/h\n", 0},
		{"the assignment form is reported", "hash -v foo=/bin/ls\n", "foo=/bin/ls\n", 0},
		{"an entry already there still prints", "hash g\nhash -v g\n", "g=%s/g\n", 0},
		{"the named-directory table too", "hash -dv a=/tmp b=/var\n", "a=/tmp\nb=/var\n", 0},
		{"a query of one that is there prints", "hash -d a=/tmp\nhash -dv a\n", "a=/tmp\n", 0},
		{
			"a command that resolves to nothing is the complaint alone",
			"hash -v nosuchcommandhere\n",
			"zsh:hash:1: no such command: nosuchcommandhere\n", 1,
		},
		{
			"and so is a directory name that is not there",
			"hash -dv nosuchnamehere\n",
			"zsh:hash:1: no such directory name: nosuchnamehere\n", 1,
		},
		{"no operands is the listing, unchanged", "hash g\nhash -v\n", "g=%s/g\n", 0},
		{"a clear happens first, so there is nothing to report", "hash g\nhash -rv\n", "", 0},
		{"`-L` does not shape the report", "hash -dvL a=/tmp\n", "a=/tmp\n", 0},
		{"and it still shapes the listing beside it", "hash -d a=/tmp\nhash -dL\n", "hash -d a=/tmp\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			hashTool(t, dir, "g")
			hashTool(t, dir, "h")
			// The scratch directory is the one thing a want cannot spell, so
			// `%s` stands for it — every occurrence, since a row may name two
			// entries.
			want := strings.ReplaceAll(tc.want, "%s", dir)
			out, st := runZshHash(t, dir, tc.src)
			if out != want || st != tc.st {
				t.Errorf("out = %q status = %d, want %q at %d", out, st, want, tc.st)
			}
		})
	}
}
