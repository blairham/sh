// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// After the mode word, a letter is `set`'s option letter, from the table the
// caller's shell reads `set` by (#5248). See emulateLetter for the rule.
//
// Every row prints the status and what it reads. Measured on zsh 5.9.2, `-f`,
// 2026-09-30; on main before this change every row but the controls differed.
func TestEmulateReadsLettersAfterTheModeAsSetDoes(t *testing.T) {
	const read = "[[ -o longlistjobs ]] && print llj; [[ -o sunkeyboardhack ]] && print skh; " +
		"[[ -o localoptions ]] && print lo; [[ -o nullglob ]] && print ng; " +
		"[[ -o autopushd ]] && print apd; [[ -o markdirs ]] && print md; " +
		"[[ -o listtypes ]] || print nolt; [[ -o notify ]] || print nonotify; :"
	rows := []struct{ name, src, want string }{
		{"-R is longlistjobs, not strict", "emulate zsh -R -c '" + read + "'", "llj|st=0"},
		{"-L is sunkeyboardhack, not local", "f() { emulate zsh -L; " + read + "; }; f", "skh|st=0"},
		{"-N is autopushd", "emulate zsh -N -c '" + read + "'", "apd|st=0"},
		{"a bundle", "emulate zsh -NG -c '" + read + "'", "ng|apd|st=0"},
		{"+X turns listtypes off", "emulate zsh +X -c '" + read + "'", "nolt|st=0"},
		{"a digit", "emulate zsh +5 -c '" + read + "'", "nonotify|st=0"},
		{"a bad letter", "emulate zsh -j -c 'print ran'", "bad option: -j|st=1"},
		{
			"a fixed letter says so and goes on", "emulate zsh -Z -c 'print ran'",
			"can't change option: -Z|ran|st=0",
		},
		{"and its + form is silent", "emulate zsh +Z -c 'print ran'", "ran|st=0"},
		{"-s is fixed too", "emulate zsh -s -c 'print ran'", "can't change option: -s|ran|st=0"},
		{"said before a bad name", "emulate zsh -Zo bad", "can't change option: -Z|no such option: bad|st=1"},
		{
			"-b ends the options with its word", "emulate zsh -Gb -c '" + read + "'",
			"unknown argument -c|st=1",
		},
		{"and a letter before it still counts", "emulate zsh -Gb; [[ -o nullglob ]] && print ng", "ng|st=0"},
		{"the string may follow -b", "emulate zsh -cb 'print ran'", "ran|st=0"},
		// The letters every shell shares, in either table.
		{"-C is noclobber", "emulate zsh -C -c '[[ -o clobber ]] || print noclob'", "noclob|st=0"},
		{"-a under sh's table", "emulate sh; emulate zsh -a -c '[[ -o allexport ]] && print ae'", "ae|st=0"},
		// The table is the caller's, not the mode's.
		{"sh's table: -X is markdirs", "emulate sh; emulate zsh -X -c '" + read + "'", "md|st=0"},
		{"sh's table has no -G", "emulate sh; emulate zsh -G -c 'print ran'", "bad option: -G|st=1"},
		{"sh's table: +b is notify", "emulate sh; emulate zsh +b -c '" + read + "'", "nonotify|st=0"},
		// -L and -R before the mode are the builtin's own, unchanged.
		{"control: -L before the mode", "f() { emulate -L zsh; " + read + "; }; f", "lo|st=0"},
		{"control: -R before the mode", "setopt histignorespace; emulate -R zsh; [[ -o histignorespace ]] || print nohis", "nohis|st=0"},
		{"and -R after it does not reset", "setopt histignorespace; emulate zsh -R; [[ -o histignorespace ]] && print his", "his|st=0"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src+"\nprint st=$?\n")
			var got []string
			for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				// The diagnostic's prefix names the route: keep what
				// follows `emulate:N: `.
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

// A letter joins the sticky identity as the name it abbreviates (#5248):
// `-G` is `-o nullglob`, and `-F` is `+o glob`. The rows are #5144's, a
// caller that turns `alwayslastprompt` off, calls a sticky function that
// turns it on, and reads it — `on` is "the same emulation, not entered".
func TestAnEmulateLetterIsItsNameInTheStickyIdentity(t *testing.T) {
	rows := []struct{ name, outer, inner, want string }{
		{"-G is -o nullglob", "zsh -G", "zsh -o nullglob", "on"},
		{"-F is +o glob", "zsh -F", "zsh +o glob", "on"},
		{"a letter is a word", "zsh", "zsh -G", "off"},
		{"a fixed letter is not", "zsh", "zsh -Z", "on"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), stickyIsALP+stickyPair(row.outer, row.inner))
			// A fixed letter says so on stderr, each time it is read.
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if got := lines[len(lines)-1]; got != row.want {
				t.Errorf("got %q, want %q (raw %q)", got, row.want, out)
			}
		})
	}
}
