// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestGlobalExportDecidesTheExportLettersScope pins GLOBAL_EXPORT and `+g`:
// with the option on, `typeset -x` in a function reaches the global; off, it
// is a local; `+g` is a local either way. Measured 2026-10-02 on zsh 5.9.2
// under `-f` (#5155).
func TestGlobalExportDecidesTheExportLettersScope(t *testing.T) {
	const fns = "show(){ print $v ${(t)v}; }; set1(){ typeset -x v=in; show }; set2(){ typeset +g -x v=in; show }; v=out\n"
	for _, tc := range []struct{ src, want string }{
		{"unsetopt globalexport; set1; show", "in scalar-local-export\nout scalar\n"},
		{"setopt globalexport; set1; show", "in scalar-export\nin scalar-export\n"},
		{"set2; show", "in scalar-local-export\nout scalar\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), fns+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
