// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A word beginning `%?` is globbed like any other** — the `?` is a
// pattern and matches `%a`. Measured 2026-10-01 against ksh93u+, in a directory
// holding `%xb1`, `%?b2`, `%?`, `%a` and `b%a`; zsh is the one column that
// reads the spelling as a job spec instead. See
// interp.Semantics.JobSpecQuestionMarkIsLiteral.
func TestAJobSpecQuestionMarkGlobs(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `: > '%xb1'; : > '%?b2'; : > '%?'; : > '%a'; : > 'b%a'
echo %?; echo %?*`)
	if want := "%? %a\n%? %?b2 %a %xb1\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
