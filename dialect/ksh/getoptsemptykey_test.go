// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestGetoptsEmptyKeyWarningIsUnlocated: the empty-key warning `getopts`
// earns under `set -x` is behind the shell's name alone, where `read`'s is
// located as a builtin's (#6683). Measured 2026-10-16 on ksh93u+ 2012-08-01.
func TestGetoptsEmptyKeyWarningIsUnlocated(t *testing.T) {
	// On the second line, where a located complaint carries its number.
	out, _ := runKsh(t, t.TempDir(), "typeset -A a; set -x\ngetopts a 'a[]' -a\nread 'a[]' <<< x")
	want := "+ getopts a 'a[]' -a\nksh: warning: adding empty subscript\n+ read 'a[]'\n+ 0<<< x\nksh[3]: read: warning: adding empty subscript\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
