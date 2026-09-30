// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Before the mode word, `emulate` takes `-L` and `-R` and nothing that needs
// a word after it (#5247). `-o` and `-c` are refused as they are read, and a
// word starting with `+` is not a flag word at all but the mode word itself —
// so whatever follows it is a surplus operand.
//
// Every row prints the status and then the mode, so a row that refuses has to
// leave the emulation where it was. Measured on zsh 5.9.2, `-f`, 2026-09-30;
// on main before this change every row but the controls differed.
func TestEmulateBeforeTheModeWord(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{"-o", "emulate -o nullglob zsh", "bad option: -o|st=1|zsh"},
		{"-o with nothing after it", "emulate -o", "bad option: -o|st=1|zsh"},
		{"-o in a bundle, after R", "emulate -Ro nullglob sh", "bad option: -o|st=1|zsh"},
		{"-o in a bundle, before R", "emulate -oR sh", "bad option: -o|st=1|zsh"},
		{"-o after -L", "emulate -L -o nullglob sh", "bad option: -o|st=1|zsh"},
		{"-c", "emulate -c 'print ran' sh", "bad option: -c|st=1|zsh"},
		{"+o is the mode", "emulate +o nullglob zsh", "unknown argument nullglob|st=1|zsh"},
		{"+o alone is an unknown mode", "emulate +o", "st=0|zsh"},
		{"+R is the mode", "emulate +R sh", "unknown argument sh|st=1|zsh"},
		{"+c is the mode", "emulate +c 'print ran' sh", "unknown argument print ran|st=1|zsh"},
		{"+o after -L is the mode", "emulate -L +o nullglob sh", "unknown argument nullglob|st=1|zsh"},
		{
			"the +o option is unchanged", "setopt nullglob; emulate +o sh; [[ -o nullglob ]] && print ng",
			"unknown argument sh|ng|st=0|zsh",
		},
		// After the mode both forms are still what they were.
		{
			"control: -o after the mode", "emulate sh -o nullglob; [[ -o nullglob ]] && print ng",
			"ng|st=0|sh",
		},
		{
			"control: +o after the mode", "emulate sh +o nomatch; [[ -o nomatch ]] || print nm-off",
			"nm-off|st=0|sh",
		},
		{"control: -LR before the mode", "emulate -LR sh", "st=0|sh"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src+"\nprint st=$?\nemulate\n")
			var got []string
			for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				// The diagnostic's prefix names the route, which is not
				// what these rows are about: keep what follows `emulate:N: `.
				if i := strings.Index(l, "emulate:"); i >= 0 {
					if j := strings.Index(l[i+len("emulate:"):], ": "); j >= 0 {
						l = l[i+len("emulate:")+j+2:]
					}
				}
				got = append(got, l)
			}
			if g := strings.Join(got, "|"); g != row.want {
				t.Errorf("got %q, want %q (raw %q)", g, row.want, out)
			}
		})
	}
}
