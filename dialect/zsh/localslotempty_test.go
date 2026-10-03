// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnEmptyValueForALocalArraySlotIsNoElement pins that a declaration
// giving one of the shell's own array slots an empty value makes an array of
// no elements, where the same assignment outside a declaration is one empty
// element (#5151, a chunk of D04parameter.ztst). Measured 2026-10-03 on zsh
// 5.9.2 under `-f`.
func TestAnEmptyValueForALocalArraySlotIsNoElement(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`f(){ local path=; print $#path "[$path]" ${(t)path}; }; f`, "0 [] array-local-tied-special\n"},
		{`f(){ local fpath=''; typeset cdpath=""; print $#fpath $#cdpath; }; f`, "0 0\n"},
		// The controls: a value is one element, and a global is not a
		// declaration.
		{`f(){ local path=/x; print $#path; }; f; path=; print $#path`, "1\n1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
