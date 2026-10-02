// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedEmptyElementPadsAsOneUnit pins that the empty element a nested
// expansion hands back takes one unit of a padded field while printing as
// nothing. Measured 2026-10-01 and 2026-10-02 on zsh 5.9.2 under `-f` and
// `LC_ALL=C` (#5345).
func TestANestedEmptyElementPadsAsOneUnit(t *testing.T) {
	const show = "show() { print -rn -- \"$#:\"; for w; print -rn -- \"<$w>\"; print }\nb=(x '' y)\n"
	for _, tc := range []struct{ src, want string }{
		{`show "${(@l:1:)${b[@]}}"`, "3:<x><><y>\n"},
		{`show "${(@l:3:)${b[@]}}"`, "3:<  x><  ><  y>\n"},
		{`show "${(@r:2::-:)${b[@]}}"`, "3:<x-><-><y->\n"},
		{`show "${(@l:2::L:r:2::R:)${b[@]}}"`, "3:<LLxR><LLR><LLyR>\n"},
		// The controls: a name's own empty element and one a pattern
		// operator produced are empty.
		{`show "${(@l:3:)b}"`, "3:<  x><   ><  y>\n"},
		{`show "${(@l:3:)${b[@]}#x}"`, "3:<   ><   ><  y>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), show+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
