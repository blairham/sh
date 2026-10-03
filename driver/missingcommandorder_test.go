// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// `-c` with nothing behind it is judged after the option words, the way a
// script operand that will not open is: every shell in the panel names a bad
// letter in `-cq` rather than the missing command string. Ours named the
// string, because the route was resolved before any option was applied.
func TestAMissingCommandStringIsJudgedAfterTheOptions(t *testing.T) {
	sh := shell()
	_, errs, code := runArgs(t, sh, "testsh", "-cq")
	if strings.Contains(errs, "requires an argument") {
		t.Errorf("stderr = %q, want the letter refused before the missing string", errs)
	}
	if code == 0 {
		t.Errorf("status 0, want a refusal")
	}

	// And with nothing to refuse, the missing string is still reported.
	_, errs, code = runArgs(t, sh, "testsh", "-c")
	if errs != "testsh: -c requires an argument\n" || code != 2 {
		t.Errorf("got %q status %d, want the missing string at 2", errs, code)
	}
}

// The status of that refusal is the dialect's, on every option that takes an
// argument: one column exits 1 where the rest exit with the usage status.
func TestAMissingOptionArgumentExitsWithTheDialectsStatus(t *testing.T) {
	for _, argv := range [][]string{{"testsh", "-c"}, {"testsh", "-o"}} {
		sh := shell()
		sh.Diagnostics.InvocationMissingOptionArgumentStatus = 1
		if _, _, code := runArgs(t, sh, argv...); code != 1 {
			t.Errorf("%v: status %d, want the dialect's 1", argv, code)
		}
	}
}

// `-o` with no word behind it lists the options, in the dialect that reads it
// so, and the listing reads the state the bundle has reached by then — `e`
// was read before the `o` here, so errexit is on in it.
func TestABareOLetterListsTheOptionsWhereTheDialectSaysSo(t *testing.T) {
	sh := shell()
	sh.Semantics.InvocationBareOListsTheOptions = interp.Yes
	out, errs, code := runArgs(t, sh, "testsh", "-ceo")
	if !strings.Contains(out, "errexit") {
		t.Fatalf("stdout = %q, want the option listing", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "errexit") && !strings.HasSuffix(line, "on") {
			t.Errorf("listing row %q, want errexit on: `e` was read first", line)
		}
	}
	// And the missing command string is still refused after it.
	if errs != "testsh: -c requires an argument\n" || code != 2 {
		t.Errorf("got %q status %d, want the missing string after the listing", errs, code)
	}

	// Unanswered, it is the refusal it always was.
	sh = shell()
	out, _, code = runArgs(t, sh, "testsh", "-ceo")
	if out != "" || code == 0 {
		t.Errorf("unanswered: got %q status %d, want a refusal and no listing", out, code)
	}
}

// A refused letter read while errexit is already on is an ordinary failure in
// the dialect that says so — 1, the sentence alone — and the usage error it
// otherwise is when errexit came after it.
func TestALetterRefusedUnderErrexitIsAnOrdinaryFailure(t *testing.T) {
	sh := shell()
	sh.Semantics.InvocationLetterRefusedUnderErrexitFails = interp.Yes
	sh.Diagnostics.InvocationUsage = "Usage: %[1]s [option] ..."
	_, errs, code := runArgs(t, sh, "testsh", "-e", "-q", "-c", ":")
	if code != 1 || strings.Contains(errs, "Usage:") {
		t.Errorf("-e -q: got %q status %d, want the sentence alone at 1", errs, code)
	}
	_, errs, code = runArgs(t, sh, "testsh", "-q", "-e", "-c", ":")
	if code != 2 || !strings.Contains(errs, "Usage:") {
		t.Errorf("-q -e: got %q status %d, want the usage error at 2", errs, code)
	}
}

// A `--name` word the dialect lists as a spelling of a `set -o` name turns
// that name on, and one it does not list is refused as it always was.
func TestALongOptionNamingASetOptionTurnsItOn(t *testing.T) {
	sh := shell()
	sh.Semantics.LongOptionsNamingSetOptions = "xtrace"
	_, errs, code := runArgs(t, sh, "testsh", "--xtrace", "-c", "echo hi")
	if code != 0 || !strings.Contains(errs, "echo hi") {
		t.Errorf("got %q status %d, want the command traced", errs, code)
	}
	if _, _, code = runArgs(t, sh, "testsh", "--verbose", "-c", ":"); code == 0 {
		t.Errorf("--verbose: status 0, want a word nobody listed refused")
	}
}
