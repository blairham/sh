// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestWarnCreateGlobalNamesANumberArithmeticMade pins the word the lint uses
// for a name arithmetic declares as it creates it. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5155).
func TestWarnCreateGlobalNamesANumberArithmeticMade(t *testing.T) {
	got, _ := runZshOnPath(t, t.TempDir(),
		"f(){ setopt warncreateglobal; (( g=1.5 )); let h=2; x=$(( q=3 )); (( z++ )); s=1 }; f")
	want := "f: numeric parameter g created globally in function f\n" +
		"f: numeric parameter h created globally in function f\n" +
		"f: numeric parameter q created globally in function f\n" +
		"f: scalar parameter x created globally in function f\n" +
		"f: numeric parameter z created globally in function f\n" +
		"f: scalar parameter s created globally in function f\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
