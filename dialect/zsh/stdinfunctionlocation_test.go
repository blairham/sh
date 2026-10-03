// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFunctionBodyOnStandardInputIsLocatedByTheFunction pins that the
// trimmed shape standard input gives a diagnostic stops at a function's door:
// inside a body the function and its line are named, as in a script, and at
// the top the route's shape stands (#5151, the front of D04parameter.ztst).
// Measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -fs`, and `-fis`
// alike).
func TestAFunctionBodyOnStandardInputIsLocatedByTheFunction(t *testing.T) {
	const src = "g() {\n  cd /nonexistent\n  : ${zz:?unset1}\n}\ng\n"
	if got, want := zshStderrOnStdin(t, "zsh", src),
		"g:cd:1: no such file or directory: /nonexistent\ng:2: zz: unset1\n"; got != want {
		t.Errorf("inside a body: got %q, want %q", got, want)
	}
	const top = "cd /nonexistent2\nh() { cd /nonexistent3; }\nh\nk() {\n  :\n  print ${1:?none3}\n}\nk\n"
	if got, want := zshStderrOnStdin(t, "zsh", top),
		"cd: no such file or directory: /nonexistent2\n"+
			"h:cd: no such file or directory: /nonexistent3\n"+
			"k:2: 1: none3\n"; got != want {
		t.Errorf("at the top, a one-line body, and a parameter: got %q, want %q", got, want)
	}
}
