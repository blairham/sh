// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A `=~` pattern that will not compile is a match that did not happen, at 1,
// and ksh93 says nothing about it. Measured 2026-10-03 on 93u+ for `[`, `(`,
// `a{1`, a trailing backslash, `a{2,1}`, `*a` and `+`.
func TestAPatternThatWillNotCompileIsSilentlyFalse(t *testing.T) {
	for _, pat := range []string{"[", "(", "a{1", `\`, "a{2,1}", "*a", "+"} {
		out, errs, _ := runKshScript(t, "p='"+pat+"'; [[ a =~ $p ]]; echo \"st=$?\"\n")
		if out != "st=1\n" || errs != "" {
			t.Errorf("%q: got %q / %q, want st=1 and nothing said", pat, out, errs)
		}
	}
}

// A dash inside an option bundle of `print` is an inert letter: `print -n-r x`
// writes `x` with no newline. Measured 2026-10-03 on ksh93u+.
func TestADashInsideAPrintBundleIsInert(t *testing.T) {
	out, errs, _ := runKshScript(t, "print -n-r x; echo \"\"; print - -n x; print ---\n")
	if out != "x\n-n x\n---\n" || errs != "" {
		t.Errorf("got %q / %q, want x, -n x and ---", out, errs)
	}
}

// `unset` with nothing to unset is the usage line at 2, and it ends the
// script, `unset` being special. Measured 2026-10-03 on ksh93u+.
func TestUnsetWithNothingToUnsetIsAUsageError(t *testing.T) {
	out, errs, code := runKshScript(t, "unset; echo \"1=$?\"\n")
	if out != "" || errs != "Usage: unset [-nfv] name...\n" || code != 2 {
		t.Errorf("got %q / %q / %d, want the usage line alone and 2", out, errs, code)
	}
}
