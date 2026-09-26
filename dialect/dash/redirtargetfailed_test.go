// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A redirection **target** that will not expand is **this shell's own failed
// expansion**, not the redirection's, and a failed expansion ends this shell
// at 2 — on every command shape, including the ones a failed *open* leaves
// alive. Measured 2026-09-26 on dash 0.5.12 (#4689).
func TestAFailedRedirectTargetEndsTheShellHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, src := range []string{
		`: < $(( 1/0 ))`,
		`read x < $(( 1/0 ))`,
		`f() { echo RAN; }; f < $(( 1/0 ))`,
		`{ echo RAN; } < $(( 1/0 ))`,
	} {
		out, st := runDash(t, dir, src+"\necho after\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("%s = %q, want the command left unrun", src, out)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the shell ended", src, out)
		}
		if st != 2 {
			t.Errorf("%s: status = %d, want 2", src, st)
		}
	}
}

// The same pair TestAFailedTargetIsNotAFailedHeredocBodyHere draws, read in
// the **numbers** rather than in the reach: this column answers the body Yes,
// so a failing body on `read x` reports the redirection's 2 and carries on,
// and answers the target No, so the same failure exits 2 as a fatal error.
// One field cannot say both, which is why there are two (#4689).
func TestTheTargetAndTheBodyPartCompanyInTheNumbersHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body, bodySt := runDash(t, dir, "read x <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(body, "after st=2") || bodySt != 0 {
		t.Errorf("a failing body = %q (status %d), want this shell carrying on at 2", body, bodySt)
	}
	target, targetSt := runDash(t, dir, "read x < $(( 1/0 ))\necho \"after st=$?\"\n")
	if strings.Contains(target, "after") || targetSt != 2 {
		t.Errorf("a failing target = %q (status %d), want the shell ended at 2", target, targetSt)
	}
}

// And a failed **open** is not a failed expansion either: `read x <
// /nonexistent/f` carries this shell on at 2, which is the row that says the
// target's fatality comes from the expansion rather than from the redirection.
func TestAFailedOpenOnARegularBuiltinIsNotFatalHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runDash(t, dir, "read x < /nonexistent/f\necho \"after st=$?\"\n")
	if !strings.Contains(out, "after st=2") || st != 0 {
		t.Errorf("= %q (status %d), want this shell carrying on at 2", out, st)
	}
}

// The control: a target that expands is opened and its command runs.
func TestATargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runDash(t, dir, "echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
