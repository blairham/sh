// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A redirection **target** that will not expand is **this shell's own failed
// expansion**, not the redirection's, and a failed expansion ends this shell
// at 2 — on every command shape.
//
// Measured 2026-09-26 on BusyBox v1.37.0 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b)
// (#4689).
func TestAFailedRedirectTargetEndsTheShellHere(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`: < $(( 1/0 ))`,
		`read x < $(( 1/0 ))`,
		`f() { echo RAN; }; f < $(( 1/0 ))`,
		`{ echo RAN; } < $(( 1/0 ))`,
	} {
		out, st := run(t, src+"\necho after\n")
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
// the **numbers** rather than in the reach, and this is the column where the
// two numbers differ: the body is the redirection's failure and reports **1**,
// where the target is this shell's own and exits **2** (#4689).
func TestTheTargetAndTheBodyPartCompanyInTheNumbersHere(t *testing.T) {
	t.Parallel()
	body, bodySt := run(t, "read x <<END\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
	if !strings.Contains(body, "after st=1") || bodySt != 0 {
		t.Errorf("a failing body = %q (status %d), want this shell carrying on at 1", body, bodySt)
	}
	target, targetSt := run(t, "read x < $(( 1/0 ))\necho \"after st=$?\"\n")
	if strings.Contains(target, "after") || targetSt != 2 {
		t.Errorf("a failing target = %q (status %d), want the shell ended at 2", target, targetSt)
	}
}

// And a failed **open** is not a failed expansion either: it reports 1 and
// carries this shell on, which is the row that says the target's fatality
// comes from the expansion rather than from the redirection.
func TestAFailedOpenOnARegularBuiltinIsNotFatalHere(t *testing.T) {
	t.Parallel()
	out, st := run(t, "read x < /nonexistent/f\necho \"after st=$?\"\n")
	if !strings.Contains(out, "after st=1") || st != 0 {
		t.Errorf("= %q (status %d), want this shell carrying on at 1", out, st)
	}
}

// The control: a target that expands is opened and its command runs.
func TestATargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	if out, st := run(t, "echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
