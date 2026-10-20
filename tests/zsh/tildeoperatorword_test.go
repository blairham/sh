// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestALeadingTildeInASplitOperatorWordIsOneField pins that the directory a
// leading `~` in a `-` or `+` word became does not split with the word's text
// under `emulate sh` (#5151, a chunk of D04parameter.ztst). Measured
// 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestALeadingTildeInASplitOperatorWordIsOneField(t *testing.T) {
	const setup = "l() { print -rn -- \"$#:\"; for w; print -rn -- \"<$w>\"; print }\nf() { emulate -L sh; local HOME='/a b'; "
	for _, tc := range []struct{ src, want string }{
		{`l ${1:-~}; }; f`, "1:</a b>\n"},
		{`l ${1:-~/x y}; }; f`, "2:</a b/x><y>\n"},
		// The controls: a quoted tilde is no tilde, and the written text
		// still splits around an expansion.
		{`l ${1:-"~"} ${1:-p q}; }; f`, "3:<~><p><q>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
