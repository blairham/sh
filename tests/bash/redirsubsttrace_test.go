// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A process substitution written as a redirection's target has its body
// traced here, as one written as a word does — zsh is the shell that writes
// those lines nowhere. Measured 2026-10-16 on bash 5.3.20 under `-c` (#6799).
// See interp.Semantics.RedirectTargetSubstitutionBodyIsTraced.
func TestARedirectionTargetsBodyIsTraced(t *testing.T) {
	out, errs, _ := runAlias(t, `set -x; cat < <(echo hi); read v < <(echo x)`)
	if want := "+ cat\n++ echo hi\n+ read v\n++ echo x\n"; out != "hi\n" || errs != want {
		t.Errorf("got %q and %q, want %q and %q", out, errs, "hi\n", want)
	}
}
