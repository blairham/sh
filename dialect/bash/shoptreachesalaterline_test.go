// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `shopt -s extglob` decides whether `@(a|b)` on a *later line* is a
// quantified group, and a sourced file and `eval` read their text a line at a
// time here — so the option reaches the lines after it in both, exactly as it
// reaches the lines after it in a script the front end is reading.
//
// It did not until #4591. The reader built one parser from the dialect it
// started on and never asked again, so a file whose first line turned the
// grammar on and whose second used it was a syntax error, while the front
// end's own loop had watched for the replacement all along. The option that
// took this reader past it is a different one in a different shell —
// `rcquotes`, which moves how a single-quoted run is read — and the
// mechanism is shared, so this is the row that says the fix is the reader's
// and not that option's.
//
// Measured on bash 5.3.20 (`/opt/homebrew/bin/bash`, `--norc --noprofile`),
// 2026-09-26, over a file holding `shopt -s extglob` and then `echo @(a|b)`:
// `. f` prints `@(a|b)` — the pattern matched no file and the word stands —
// and `eval` over the same two lines prints it too. This shell wrote
// “syntax error near unexpected token `('“ for both.
func TestShoptReachesALaterLineOfBorrowedText(t *testing.T) {
	const twoLines = "shopt -s extglob\necho @(a|b)\n"
	t.Run("a sourced file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "inc.sh")
		if err := os.WriteFile(path, []byte(twoLines), 0o600); err != nil {
			t.Fatal(err)
		}
		out, st := runBash(t, dir, ". "+path+"\n")
		if want := "@(a|b)\n"; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q at 0", out, st, want)
		}
	})
	t.Run("eval over the same two lines", func(t *testing.T) {
		out, st := runBash(t, t.TempDir(), "eval '"+twoLines+"'\n")
		if want := "@(a|b)\n"; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q at 0", out, st, want)
		}
	})
	// The control: with nothing turning the grammar on, the same word is
	// still a syntax error — so the rows above are the option arriving and
	// not the group having become unconditional.
	t.Run("and without the option it is still refused", func(t *testing.T) {
		out, st := runBash(t, t.TempDir(), "eval 'echo @(a|b)'\n")
		if st == 0 {
			t.Errorf("got %q status %d, want the group refused", out, st)
		}
	})
}
