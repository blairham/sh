// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAKeywordOutranksAFunctionOfItsName is the order a name's resolutions are
// reported in: the reserved word is read by the grammar before any function
// is looked up. Measured 2026-10-02 on bash 5.3.20 (#5267).
func TestAKeywordOutranksAFunctionOfItsName(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), "function time { :; }; type -t time; type -t -a time\n")
	if want := "keyword\nkeyword\nfunction\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
