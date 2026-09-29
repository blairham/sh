// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `set -C` here protects **files**, and nothing protects the name a `{var}`
// redirection writes into. This is the No side of
// interp.Semantics.NoclobberProtectsAnFdVariable.
//
// The row exists because the Yes side is a single column, and a rule with one
// column is indistinguishable from an unconditional one: a mutation that drops
// the axis and refuses everywhere survives a suite that grades zsh alone.
//
// Measured 2026-09-29 on bash 5.3.20 (`/opt/homebrew/bin/bash`), a script file
// in a fresh directory:
//
//	set -C; exec {m}>f1; exec {m}>f2; echo "st=$? m=$m"
//	    st=0 m=11, nothing on standard error
//
// The second descriptor lands on the same number because the first was
// released, which is itself the point: this shell overwrites the name without
// consulting what was in it.
func TestNoclobberDoesNotProtectAnFdVariableHere(t *testing.T) {
	out, errs := runBashSplit(t, "set -C\nexec {m}>f1\nexec {m}>f2\necho \"st=$? m=$m\"\n")
	if out != "st=0 m=11\n" || errs != "" {
		t.Errorf("out %q err %q, want the name overwritten in silence at 0", out, errs)
	}
	// The control, which is what keeps this from being "`set -C` does
	// nothing here": the file rule is alive in the same script.
	t.Run("while the file rule is alive in the same shell", func(t *testing.T) {
		out, errs := runBashSplit(t, "set -C\necho hi > f\necho hi > f\necho \"st=$?\"\n")
		if out != "st=1\n" || errs == "" {
			t.Errorf("out %q err %q, want the second write refused", out, errs)
		}
	})
}
