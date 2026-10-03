// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAReplacementsSlashIsFoundWithoutRegardToQuotes pins zsh's reading of
// `${name/pattern/replacement}`: the `/` is found without regard to quotes,
// only a backslash protecting one, and each half is read with the quote it
// was cut inside of (#5151, the chunk of D04parameter.ztst that suite marks
// as failing). Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestAReplacementsSlashIsFoundWithoutRegardToQuotes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=a/b; print -r -- ${x//'/'/'\'} ${x//'/'/X} ${x//"/"/X} "${x/'/'/X}"`, "a/b a/b a/b a/b\n"},
		{`x="a'b/c"; print -r -- ${x//'/'/X}`, "a/Xb/c\n"},
		{`x='a"b'; print -r -- ${x//"/"/X}`, "a/Xb\n"},
		{`x=a/b; print -r -- ${x//\//X} ${x//\//'\'}`, "aXb a\\b\n"},
		{`x=ab; print -r -- ${x//b/'/'} ${x//b/"/"}`, "a/ a/\n"},
		{`v="'"; printf '[%s]' ${v/$'\''/x}; echo`, "[x]\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
