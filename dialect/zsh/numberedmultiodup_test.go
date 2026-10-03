// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestNumberedDuplicationsJoinUnderMultios pins a write duplication onto a
// descriptor above two joining the descriptor's earlier targets, as a file
// does. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5155), the shape
// E01options' "multios with nullexec" row is written in.
func TestNumberedDuplicationsJoinUnderMultios(t *testing.T) {
	cases := []struct{ src, out, errs string }{
		{"exec 3>&1 3>&2; print -u 3 x", "x\n", "x\n"},
		{"print -u 3 x 3>&1 3>&2", "x\n", "x\n"},
		{"{ print -u 3 x } 3>&1 3>&2", "x\n", "x\n"},
		{"exec 3>&1; exec 3>&2; print -u 3 x", "", "x\n"},
		{"unsetopt multios; exec 3>&1 3>&2; print -u 3 x", "", "x\n"},
	}
	for _, c := range cases {
		out, _, errs := runZshUTF8(t, c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%s: got out %q err %q, want out %q err %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
