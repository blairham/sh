// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `%h` and its synonym `%!` draw the **newest** history event's number.
//
// Measured 2026-09-26 on zsh 5.9.2, `-f`, against the same three-line file
// this dialect's `fc -R` reads:
//
//	print -P '%h'                          0
//	fc -R seed; print -P '%h'              3    and `fc -l` numbers them 1..3
//	fc -R seed; HISTSIZE=2; print -P '%h'  3
//
// The empty case is the one that pins which end of the list is read: a shell
// counting the number the *next* line will take answers 1 there, which is
// bash's reading of its own `\!` and is why the arithmetic is this package's
// rather than interp's.
//
// The trimmed row is the second half of it. `HISTSIZE=2` drops the oldest
// entry, and the number stays 3 because an entry keeps the number it was
// given — so this reads the list's numbering rather than its length, and a
// rule written as `len(entries)` would answer 2.
func TestTheHistoryNumberEscapeDrawsTheNewestEvent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"an empty list is event zero", "print -P '%h'\n", "0\n"},
		{"and the synonym reads the same", "print -P '%!'\n", "0\n"},
		{"a loaded list is its last number", "fc -R seed\nprint -P '%h'\n", "3\n"},
		{"the synonym there too", "fc -R seed\nprint -P '%!'\n", "3\n"},
		{"and the listing agrees with it", "fc -R seed\nfc -l\nprint -P '%h'\n", "    1  one\n    2  two\n    3  three\n3\n"},
		{"a trimmed list keeps the number", "fc -R seed\nHISTSIZE=2\nprint -P '%h'\n", "3\n"},
		{"the expansion route draws it as well", "print -rP \"[${(%):-%h}]\"\n", "[0]\n"},
		// Script lines are not events, which is what says the count is the
		// list's and not the shell's own progress through the file.
		{"a script's own lines are not events", "print -P '%h'\nprint -P '%h'\n", "0\n0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "seed"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			out, st := fcRun(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
