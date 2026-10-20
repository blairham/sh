// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestFunctionArgZeroIsReadAtTheCall pins that `functionargzero` decides
// whether a frame names `$0` as the frame is entered: a frame entered with it
// off leaves `$0` to the frame below. Measured 2026-10-02 on zsh 5.9.2 under
// `-f` (#5155).
func TestFunctionArgZeroIsReadAtTheCall(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"f(){ print $0 }; g(){ unsetopt functionargzero; f }; g", "g\n"},
		{"() { unsetopt functionargzero; f(){ print $0 }; f }", "(anon)\n"},
		{"f(){ print $0 }; () { setopt localoptions nofunctionargzero; f; }; f", "(anon)\nf\n"},
		{"f(){ unsetopt functionargzero; print $0 }; f", "f\n"},
		{"unsetopt functionargzero; f(){ setopt functionargzero; g }; g(){ print $0 }; f", "g\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
