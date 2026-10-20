// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestShFileExpansionPutsEqualsFirst pins where `=cmd` sits among a word's
// expansions, with `shfileexpansion` off and on, and what `nomatch` off does
// to a lookup that fails. Measured 2026-10-02 on zsh 5.9.2 under `-f`
// (#5155). PATH is /bin, so `=ls` is `/bin/ls`.
func TestShFileExpansionPutsEqualsFirst(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print =ls ={ls,}", "/bin/ls /bin/ls =\n"},
		{"print ={cat,ls} {=ls,b}", "/bin/cat /bin/ls /bin/ls b\n"},
		{"unsetopt nomatch; setopt shfileexpansion; print =ls ={ls,}", "/bin/ls =ls =\n"},
		{"setopt shfileexpansion; print {=ls,b}", "=ls b\n"},
		{"foo='=ls'; print ${~foo}", "/bin/ls\n"},
		{"setopt shfileexpansion; foo='=ls'; print ${~foo}", "=ls\n"},
		{"HOME=/h; setopt shfileexpansion; foo='~/x'; print ${~foo}", "~/x\n"},
		{"unsetopt nomatch; print =nosuchxyz; print after", "=nosuchxyz\nafter\n"},
		{"unsetopt nomatch; print ={nosuchxyz,ls}", "=nosuchxyz /bin/ls\n"},
		{"f(){ print ={ls,}; }; f; f", "/bin/ls =\n/bin/ls =\n"},
		{"setopt shfileexpansion; for x in ={ls,}; do print $x; done; print after", "zsh:1: {ls,} not found\n"},
		{"for x in ={ls,}; do print $x; done", "/bin/ls\n=\n"},
	}
	for _, c := range cases {
		if got, _ := runZsh(t, "/bin", c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
