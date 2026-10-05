// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// `SAVEHIST` is what decides whether a zsh session writes its history file,
// and how much of it the file keeps (#5902).
//
// It used to be read for `fc -W` and `fc -A` and nowhere else, so a session
// that ended wrote its lines whatever the variable said: `SAVEHIST=0`, the
// documented way to keep a session out of the file, appended every line, and
// a positive count bounded nothing because the file was cut at
// `HISTFILESIZE`, which is bash's.
//
// Measured 2026-10-04 against zsh 5.9.2 run exactly this way — `-d -i`, the
// lines on a pipe, a scratch `HOME` whose `.zshrc` sets the count and points
// `HISTFILE` at a file already holding `a1 a2 a3` — and through a pty with the
// same rc, which gives the same rows. The full table is on
// repl.historyFile.savedBy.
func TestSaveHistDecidesWhatTheSessionWrites(t *testing.T) {
	for _, tc := range []struct {
		name, setting, want string
	}{
		// Zero writes nothing and leaves the file as it was found — not
		// emptied, which is bash's reading of its own zero.
		{name: "zero", setting: "SAVEHIST=0", want: "a1\na2\na3\n"},
		// Never set is zero.
		{name: "unset", setting: ":", want: "a1\na2\na3\n"},
		// A count is the bound the file is cut to, and HISTSIZE is not.
		{name: "two", setting: "SAVEHIST=2", want: "echo hi\necho two\n"},
		{name: "four", setting: "SAVEHIST=4", want: "a2\na3\necho hi\necho two\n"},
		// HISTSIZE still decides which of the session's lines are appended
		// — the list holds one — but the file is cut at SAVEHIST, not there.
		{name: "four of a list of one", setting: "SAVEHIST=4; HISTSIZE=1", want: "a1\na2\na3\necho two\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := scratchHome(t)
			file := filepath.Join(home, "hf")
			writeHomeFile(t, home, "hf", "a1\na2\na3\n")
			writeHomeFile(t, home, ".zshrc", tc.setting+"; HISTFILE="+file+"\n")
			if _, errs, code := prompt(t, "echo hi\necho two\n", "zsh", "-d", "-i"); code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("the file holds %q, want %q", got, tc.want)
			}
		})
	}
}
