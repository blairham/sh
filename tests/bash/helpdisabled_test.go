// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A builtin `enable -n` has switched off is still a **help topic**, and the
// listing marks it with a `*`.
//
// Measured 2026-09-21 on bash 5.3.20 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, at COLUMNS 40 and 80, reading the character positions out of
// the bytes:
//
//	enable -n kill; help -s kill    the synopsis, status 0
//	enable -n kill; help kill       the same, status 0
//	enable -n history; help         `*history [-c] …` in the right column
//	enable -n cd; help              `*cd [-L|[-P [-e]]] [-@] [dir]` in the left
//
// The mark is a **column of its own** and not something prepended to the
// text: the left cell's takes the single space every row opens with, the
// right cell's takes the second of the two spaces between the columns, and
// each cell is cut to exactly the width it would have had. That is what the
// header sentence this shell does not reproduce is describing.
//
// This shell took the topic out of the table instead, reading `enable -n` as
// "the shell no longer has this builtin". A listing is column-major, so one
// topic fewer moved the whole of it: two switched off and the suite file's
// two listings lost a row apiece and every pairing after it (#2298).
func TestASwitchedOffBuiltinIsStillATopic(t *testing.T) {
	dir := t.TempDir()

	out, st := runBash(t, dir, "enable -n kill\nhelp -s kill\n")
	if st != 0 {
		t.Errorf("help -s of a switched-off builtin exited %d, want 0", st)
	}
	if !strings.HasPrefix(out, "kill: kill [-s sigspec") {
		t.Errorf("help -s kill wrote %q", out)
	}

	for _, tc := range []struct {
		name    string
		columns string
		off     string
		want    string
	}{{
		name:    "the right cell's mark takes the second separator space",
		columns: "80",
		off:     "history",
		want:    " ! PIPELINE                             *history [-c] [-d offset] [n] or hist>",
	}, {
		name:    "the left cell's mark takes the space the row opens with",
		columns: "80",
		off:     "cd",
		want:    "*cd [-L|[-P [-e]]] [-@] [dir]            readonly [-aAf] [name[=value] ...] o>",
	}, {
		// The narrow width is the discriminator for "a column of its own":
		// a mark written into the text would push the synopsis one
		// character further into its own truncation, and here there is
		// only one character of room to lose.
		name:    "and the cell is cut to the width it would have had",
		columns: "40",
		off:     "cd",
		want:    "*cd [-L|[-P [-e]]]>  readonly [-aAf] >",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			listing, _ := runBash(t, dir, "COLUMNS="+tc.columns+"\nenable -n "+tc.off+"\nhelp\n")
			lines := strings.Split(strings.TrimSuffix(listing, "\n"), "\n")
			found := false
			for _, line := range lines {
				if line == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("no row %q in the listing:\n%s", tc.want, listing)
			}
		})
	}

	// And nothing is marked when nothing is switched off, which is the
	// control: a listing that starred every row would pass the rows above.
	listing, _ := runBash(t, dir, "COLUMNS=80\nhelp\n")
	if strings.Contains(listing, "*") {
		t.Errorf("an ordinary listing carries a mark:\n%s", listing)
	}
	// The row count does not move either way, which is the failure the
	// marks are the smaller half of.
	off, _ := runBash(t, dir, "COLUMNS=80\nenable -n cd kill history\nhelp\n")
	if a, b := strings.Count(listing, "\n"), strings.Count(off, "\n"); a != b {
		t.Errorf("the listing is %d rows and %d with three builtins switched off", a, b)
	}
}
