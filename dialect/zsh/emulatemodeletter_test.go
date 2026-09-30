// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Every mode word names an emulation (#5254): one leading `r` is dropped, and
// the first letter of what is left decides — `s` and `b` are sh, `k` is ksh,
// `c` is csh, anything else is zsh. Case-sensitive, and not a path.
//
// Every row starts from **csh**, because from zsh "the word is zsh" and "the
// mode is unchanged" print the same thing — which is how the old reading was
// measured and believed. Each row sets the mode twice, once bare and once with
// `-c`, so a word that is passed over shows twice. Measured on zsh 5.9.2,
// `-f`, 2026-09-30; on main before this change every row but the controls
// read `csh` and did not run the `-c` string.
func TestEveryEmulateModeWordNamesAnEmulation(t *testing.T) {
	rows := []struct{ word, want string }{
		{"bash", "sh"},
		{"b", "sh"},
		{"bfoo", "sh"},
		{"rbash", "sh"},
		{"rsh", "sh"},
		{"'s h'", "sh"},
		{"ksh93", "ksh"},
		{"k", "ksh"},
		{"rk", "ksh"},
		{"cfoo", "csh"},
		{"rcsh", "csh"},
		{"fish", "zsh"},
		{"BASH", "zsh"},
		{"Sh", "zsh"},
		{"r", "zsh"},
		{"rr", "zsh"},
		{"rrsh", "zsh"},
		{"Rsh", "zsh"},
		{"''", "zsh"},
		{"/bin/sh", "zsh"},
		// The four names were right already, and stay right.
		{"sh", "sh"},
		{"ksh", "ksh"},
		{"csh", "csh"},
		{"zsh", "zsh"},
	}
	for _, row := range rows {
		t.Run(row.word, func(t *testing.T) {
			src := "emulate csh; emulate " + row.word + "; echo st=$?; emulate\n" +
				"emulate csh; emulate " + row.word + " -c 'echo ran; emulate'\n"
			out, st := runZsh(t, t.TempDir(), src)
			want := "st=0\n" + row.want + "\nran\n" + row.want + "\n"
			if st != 0 || out != want {
				t.Errorf("out %q status %d, want %q", out, st, want)
			}
		})
	}
}
