// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A modifier's and a parameter flag's delimiter may be a character of the
// locale's of more than one byte, taken whole. Through the front end, so the
// parse it makes before there is a runner is told too. See
// interp.Runner.CharacterLength and interp.modifierDelimiter.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc` (#5153).
func TestAWideDelimiter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a modifier's delimiter", "foo=picobarn; print ${foo:s£bar£rod£:s¥rod¥stick¥}", "picostickn\n"},
		{"beside an ASCII one", "foo=a£b; print ${foo:s/£/X/} ${foo:s:£:Y:}", "aXb aYb\n"},
		{"on every element", "a=(x.c y.c); print ${a:s£.c£.o£}", "x.o y.o\n"},
		{"a flag's delimiter", "foo=bar; print ${(r£5££X£)foo}; print ${(l«10««Y««HI«)foo}", "barXX\nYYYYYHIbar\n"},
		{"join and split", "a=(x y); print ${(j£-£)a} ${(s£,£)${:-p,q}}", "x-y p q\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshInLocale(t, "en_US.UTF-8", c.src); got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
