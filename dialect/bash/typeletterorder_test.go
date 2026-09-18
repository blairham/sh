// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// There is no precedence between `type`'s `-t` and `-p`: the later letter
// decides (#3197). Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57.
//
// The probe name has to be one the shell resolves *twice* — a builtin and a
// file — since nothing with one resolution can tell the readings apart. A
// file of our own on an empty PATH is what makes that true on any machine:
// `cd` has an external file on one of the two this was measured on and not on
// the other.
func TestTypeLetterOrderDecides(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "shift")
	if err := os.WriteFile(file, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ opts, want string }{
		// With `-a`, the kind of every resolution or the path of the file
		// rows, whichever letter came last.
		{"-apt", "builtin\nfile\n"},
		{"-atp", file + "\n"},
		// Without it, the same rule over the one answer.
		{"-pt", "builtin\n"},
		{"-tp", ""},
		// `-P` forces the PATH search whatever shape the answer takes, so a
		// `-t` after it writes the kind of what the search left rather than
		// of every resolution.
		{"-atP", file + "\n"},
		{"-aPt", "file\n"},
		{"-tP", file + "\n"},
		{"-Pt", "file\n"},
		// A repeated letter is read like any other, so the last one still
		// decides.
		{"-aptp", file + "\n"},
		{"-atpt", "builtin\nfile\n"},
		{"-apPt", "file\n"},
		// And the letters alone, which nothing here may move.
		{"-t", "builtin\n"},
		{"-p", ""},
		{"-P", file + "\n"},
		{"-aP", file + "\n"},
	} {
		out, st := runBash(t, dir, "type "+tc.opts+" shift")
		if st != 0 || out != tc.want {
			t.Errorf("type %s: out %q status %d, want %q", tc.opts, out, st, tc.want)
		}
	}
}
