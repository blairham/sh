// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An `-o` or `+o` name no option answers to is refused as the command line is
// read (#5144): `no such option: NAME` at status 1, said first, and nothing
// else happens — no emulation, no other option, no `-c` string.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-30. On main before this change every
// row but the controls reported something else first, or ran the code, or
// left an option set.
func TestABadEmulateOptionNameStopsTheCall(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{
			"before a surplus operand", "emulate zsh -o fixallmybugs 'print ran'",
			"no such option: fixallmybugs|st=1",
		},
		{
			"the +o form", "emulate zsh +o fixallmybugs 'print ran'",
			"no such option: fixallmybugs|st=1",
		},
		{
			"the -c string does not run", "emulate zsh -o fixallmybugs -c 'print ran'",
			"no such option: fixallmybugs|st=1",
		},
		{
			"the first bad name is the one said", "emulate zsh -o bad1 -o bad2 'print ran'",
			"no such option: bad1|st=1",
		},
		{
			"before a -c with no string", "emulate zsh -o bad -c",
			"no such option: bad|st=1",
		},
		{
			"before an -o with no name", "emulate zsh -o fixallmybugs -o",
			"no such option: fixallmybugs|st=1",
		},
		{
			"before a word that names no emulation", "emulate bogusmode -o fixallmybugs 'print ran'",
			"no such option: fixallmybugs|st=1",
		},
		{
			"the name as written", "emulate zsh -o NO_SUCH 'x'",
			"no such option: NO_SUCH|st=1",
		},
		{
			"a valid name beside it is not set",
			"emulate zsh -o nullglob -o bad; [[ -o nullglob ]] && print ng-leak",
			"no such option: bad|st=1",
		},
		{
			"the emulation is not entered",
			"setopt nullglob; emulate sh -o fixallmybugs; [[ -o nullglob ]] && print ng-kept; emulate",
			"no such option: fixallmybugs|ng-kept|zsh|st=0",
		},
		// Left to right: a surplus operand before the bad name is what is
		// reported, and a spelling of a real name is no bad name at all.
		{
			"control: the operand comes first", "emulate zsh 'print ran' -o bad",
			"unknown argument print ran|st=1",
		},
		{
			"control: a no- spelling", "emulate zsh -o no_nomatch -c '[[ -o nomatch ]] || print nm-off'",
			"nm-off|st=0",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src+"\nprint st=$?\n")
			var got []string
			for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				// The diagnostic's prefix names the route, which is not
				// what these rows are about.
				if i := strings.Index(l, "emulate:"); i >= 0 {
					l = l[i:]
					l = l[strings.Index(l[len("emulate:"):], ":")+len("emulate:")+2:]
				}
				got = append(got, l)
			}
			if g := strings.Join(got, "|"); g != row.want {
				t.Errorf("got %q, want %q (raw %q)", g, row.want, out)
			}
		})
	}
}
