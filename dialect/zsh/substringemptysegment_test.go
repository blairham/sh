// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnEmptySubstringSegmentIsAnEmptyModifier pins that a substring range
// segment written with nothing in it is refused as an empty modifier, where a
// blank one is the empty string (#5151, a chunk of D04parameter.ztst).
// Measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAnEmptySubstringSegmentIsAnEmptyModifier(t *testing.T) {
	const setup = "str=rts\n"
	for _, tc := range []struct{ src, want string }{
		{`print ${str:0:}; echo st=$?`, "zsh:2: unrecognized modifier\n"},
		{`print "${str:1:}"; echo st=$?`, "zsh:2: unrecognized modifier\n"},
		{`print ${str:}; echo st=$?`, "zsh:2: unrecognized modifier\n"},
		{`print ${str::}; echo st=$?`, "zsh:2: unrecognized modifier\n"},
		// The controls: a blank segment, and one that holds a length.
		{`print -r -- "<${str:0: }>" ${str:1:1}; echo st=$?`, "<> t\nst=0\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
