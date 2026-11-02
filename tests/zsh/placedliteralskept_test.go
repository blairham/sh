// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The control for bash's literal kept past `local` refusing at the top level
// (#7103): zsh's `local` there refuses nothing, and declares the array as
// `typeset` would. Measured 2026-11-02 against zsh 5.9.2 from a script file
// under `env -i PATH=/usr/bin:/bin`.
func TestLocalAtTheTopLevelDeclaresItsLiteral(t *testing.T) {
	t.Parallel()
	src := "local -a q=(1 2)\necho \"st=$?\"\ntypeset -p q\n"
	want := "st=0\ntypeset -a q=( 1 2 )\n"
	if got := runZshScriptFile(t, src, "f.sh"); got != want {
		t.Errorf("%s\ngot  %q\nwant %q", src, got, want)
	}
}
