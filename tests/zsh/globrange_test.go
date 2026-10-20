// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `[n,m]` is a glob qualifier that selects **by position in the match list**
// rather than by anything about a file — #4968.
//
// Measured 2026-09-28 on zsh 5.9.2 with `-f`, in the directory this test
// builds: five regular files `a b c d e` and a directory `dir`, which sort to
// `a b c d dir e`.
//
// Three rows carry rules the others would let through:
//
//   - **An empty selection is not a miss.** `*([9])` over six matches writes
//     no word at status 0, where `zz*([1])` — a pattern that matched nothing —
//     is the ordinary `no matches found`. So this cannot be written as a test
//     every file fails: that answer is one sentence and one status away.
//   - **The tests run first and the pick second.** `*(^.[1,2])` is the
//     directory alone, because `^.` leaves one name for the subscript to pick
//     the first two of.
//   - **The subscript is arithmetic**, not digits: `*([i])` with `i=2` and
//     `*([1+1])` both name the second match, and a blank side is the
//     empty-subscript refusal rather than a zero.
func TestTheGlobRangeQualifierSelectsByPosition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want string
		st   int
	}{
		{"a range is the n-th through the m-th", "print -r -- *([2,4])\n", "b c d\n", 0},
		{"one expression is one element", "print -r -- *([1])\n", "a\n", 0},
		{"a negative counts from the end", "print -r -- *([-1])\n", "e\n", 0},
		{"and reaches the end from a positive start", "print -r -- *([2,-1])\n", "b c d dir e\n", 0},
		{"both ends may be negative", "print -r -- *([-2,-1])\n", "dir e\n", 0},
		{"the far end is clamped", "print -r -- *([3,99])\n", "c d dir e\n", 0},
		{"and so is the near one", "print -r -- *([0,3])\n", "a b c\n", 0},
		{"a reversed range selects nothing", "print -r -- *([4,2])\nprint st=$?\n", "\nst=0\n", 0},
		{"element zero is nothing", "print -r -- *([0])\nprint st=$?\n", "\nst=0\n", 0},
		{"and so is a position past the end", "print -r -- *([9])\nprint st=$?\n", "\nst=0\n", 0},
		{"an emptied selection is not a miss", "echo A *([9]) B\nprint st=$?\n", "A B\nst=0\n", 0},
		{"a pattern that matched nothing still is", "print -r -- zz*([1])\n", "zsh:1: no matches found: zz*([1])\n", 1},
		{"the tests run first and the pick second", "print -r -- *(^.[1,2])\n", "dir\n", 0},
		{"two subscripts compose, left to right", "print -r -- *([1,3][2])\n", "b\n", 0},
		{"the subscript is a parameter", "i=2\nprint -r -- *([i])\n", "b\n", 0},
		{"and an expression", "print -r -- *([1+1])\n", "b\n", 0},
		{"blanks in it are the evaluator's", "print -r -- *([ 2 , 3 ])\n", "b c\n", 0},
		{
			"a blank side is the empty-subscript refusal",
			"print -r -- *([1,])\n",
			"zsh:1: bad math expression: empty string\n", 1,
		},
		{
			"a subscript that never closes is invalid",
			"print -r -- *([1,2)\n",
			"zsh:1: invalid subscript\n", 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, name := range []string{"a", "b", "c", "d", "e"} {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(dir, "dir"), 0o755); err != nil {
				t.Fatal(err)
			}
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("out = %q status = %d, want %q at %d", out, st, tc.want, tc.st)
			}
		})
	}
}
