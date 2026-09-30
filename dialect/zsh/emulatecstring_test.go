// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// After the mode word, `emulate` reads its options the way `set` does (#5250):
// `-c` only marks the call, and the string it runs is the first word left
// once the options are over. See emulateArguments for the whole rule.
//
// Every row prints the status and then the mode. Measured on zsh 5.9.2, `-f`,
// 2026-09-30; on main before this change every row but the controls differed.
func TestEmulateReadsItsOptionsBeforeTheString(t *testing.T) {
	const ng = "'[[ -o nullglob ]] && print ng; print ran'"
	rows := []struct{ name, src, want string }{
		{
			"the string ends the options", "emulate zsh -c 'print ran' -o nullglob",
			"unknown argument -o|st=1|zsh",
		},
		{
			"a second -c after the string", "emulate zsh -c 'print ran' -c 'print two'",
			"unknown argument -c|st=1|zsh",
		},
		{
			"an option between -c and the string", "emulate zsh -c -o nullglob " + ng,
			"ng|ran|st=0|zsh",
		},
		{
			"+o between -c and the string", "emulate zsh -c +o nomatch '[[ -o nomatch ]] || print nm-off'",
			"nm-off|st=0|zsh",
		},
		{
			"-co: o takes the word c did not", "emulate zsh -co nullglob " + ng,
			"ng|ran|st=0|zsh",
		},
		{"-c twice runs once", "emulate zsh -c -c 'print ran'", "ran|st=0|zsh"},
		{
			"-o takes the rest of its word", "emulate zsh -onullglob -c " + ng,
			"ng|ran|st=0|zsh",
		},
		{"so -oc names c", "emulate zsh -oc", "no such option: c|st=1|zsh"},
		{"- ends the options", "emulate zsh -c - 'print ran'", "ran|st=0|zsh"},
		{"and is consumed", "emulate zsh - -c 'print ran'", "unknown argument -c|st=1|zsh"},
		{"-- ends them too", "emulate zsh -c -- 'print ran'", "ran|st=0|zsh"},
		{"a bare + too", "emulate zsh -c + 'print ran'", "ran|st=0|zsh"},
		{"nothing left for the string", "emulate zsh -c --", "string expected after -c|st=1|zsh"},
		{"+c is spelled -c", "emulate zsh +c", "string expected after -c|st=1|zsh"},
		{"+o is spelled -o", "emulate zsh +o", "string expected after -o|st=1|zsh"},
		{
			"-- before the mode, options after it", "emulate -- sh -o nullglob; [[ -o nullglob ]] && print ng",
			"ng|st=0|sh",
		},
		{"-- before the mode, -c after it", "emulate -- sh -c 'print ran'", "ran|st=0|zsh"},
		{
			"-L before the mode refuses -c", "emulate -L sh -c 'print ran'",
			"option -L incompatible with -c|st=1|zsh",
		},
		{
			"and +c", "emulate -LR sh +c 'print ran'",
			"option -L incompatible with -c|st=1|zsh",
		},
		{"after every other refusal", "emulate -L sh -c", "string expected after -c|st=1|zsh"},
		{
			"control: -L before the mode, no -c", "f() { emulate -L sh; [[ -o localoptions ]] && print lo=on; }; f",
			"lo=on|st=0|zsh",
		},
		{"control: the ordinary form", "emulate sh -o nullglob -c " + ng, "ng|ran|st=0|zsh"},
		{
			"control: a surplus operand after the string", "emulate zsh -c 'print ran' extra",
			"unknown argument extra|st=1|zsh",
		},
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
