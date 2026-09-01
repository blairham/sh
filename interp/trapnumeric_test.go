// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// The bug, from the outside: a signal named by a number the hand-written map
// happened not to list was refused as though the number meant nothing.
func TestATrapTakesAnySignalNumberTheHostHas(t *testing.T) {
	// 5 is TRAP and 11 is SEGV on every platform this builds for, so the
	// numbers can be written down here without asking the host.
	for _, src := range []string{
		`trap 'echo caught' 5`,
		`trap 'echo caught' 11`,
		`trap '' 5`,
		`trap - 5`,
		// The one the wild script actually writes.
		`trap 'echo x' 0 2 3 5 13 15`,
	} {
		if out, st := run(t, src+`; echo "st=$?"`, nil); st != 0 || strings.Contains(out, "st=0") == false {
			t.Errorf("%s: %q status %d, want it accepted", src, out, st)
		}
	}
}

// A number and a name are the same trap, not two — so the second setting
// replaces the first rather than adding to it.
func TestANumberAndANameAreOneTrap(t *testing.T) {
	// Two ways of seeing it: the listing has one entry rather than two, and
	// firing the signal runs one handler rather than both.
	out, _ := run(t, "trap 'echo one' 5\ntrap 'echo two' TRAP\ntrap\n", nil)
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("listing %q has %d entries, want one", out, n)
	}
	if strings.Contains(out, "echo one") || !strings.Contains(out, "echo two") {
		t.Errorf("listing %q, want only the second handler", out)
	}
	out, _ = run(t, "trap 'echo one' 5\ntrap 'echo two' TRAP\nkill -TRAP $$\necho after\n", nil)
	if out != "two\nafter\n" {
		t.Errorf("firing said %q, want only the second handler and then after", out)
	}
}

// A number the host has no signal for is still refused, which is the half a
// derived table could have lost.
func TestAnImpossibleSignalNumberIsStillRefused(t *testing.T) {
	for _, src := range []string{`trap 'echo x' 99`, `trap 'echo x' 1000`} {
		if out, st := run(t, src, nil); st == 0 || out == "" {
			t.Errorf("%s: %q status %d, want a complaint and a failure", src, out, st)
		}
	}
}

// KILL is refused by both spellings alike. Refusing it at all is a deliberate
// divergence — the panel accepts `trap … KILL` and then never fires it — and
// the point here is that the number does not get a different answer from the
// name, which is what it used to get.
func TestKillIsRefusedTheSameWayByEitherSpelling(t *testing.T) {
	byName, stName := run(t, `trap 'echo x' KILL`, nil)
	byNumber, stNumber := run(t, `trap 'echo x' 9`, nil)
	if stName != stNumber {
		t.Errorf("KILL status %d, 9 status %d, want the same", stName, stNumber)
	}
	if strings.Replace(byNumber, "9:", "KILL:", 1) != byName {
		t.Errorf("KILL said %q, 9 said %q, want the same complaint", byName, byNumber)
	}
}
