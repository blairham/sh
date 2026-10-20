// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `return abc` outside a function complains about the operand *and* about the
// place, in that order (#2762).
//
// Measured 2026-09-14 against bash 5.3.15, `( return abc ); echo "ret=$?"` in
// a script file under `env -i PATH=/usr/bin:/bin`:
//
//	return: abc: numeric argument required
//	return: can only `return' from a function or sourced script
//	ret=2
//
// This shell wrote the second line alone. The status agreed, which is why
// nothing in the tree noticed — a row graded on the status reads as a match
// when the text is half missing.
func TestARefusedReturnOperandIsReportedBeforeTheMissingPlace(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "( return abc )\necho \"ret=$?\"\n")
	operand := strings.Index(out, "numeric argument required")
	place := strings.Index(out, "can only `return'")
	if operand < 0 {
		t.Errorf("said %q, want the operand refused", out)
	}
	if place < 0 {
		t.Errorf("said %q, want the place refused too", out)
	}
	if operand >= 0 && place >= 0 && operand > place {
		t.Errorf("said %q, want the operand's complaint first", out)
	}
	if !strings.Contains(out, "ret=2") {
		t.Errorf("said %q, want ret=2", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// And the control the ordering rule needs: an operand this shell takes draws
// the place's complaint on its own, with nothing about a number in front of
// it. Without this a fix that always wrote both lines would pass the row
// above.
func TestAGoodReturnOperandOutsideAFunctionComplainsOnlyAboutThePlace(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "( return 7 )\necho \"ret=$?\"\n")
	if strings.Contains(out, "numeric argument required") {
		t.Errorf("said %q, want nothing about the operand", out)
	}
	if !strings.Contains(out, "can only `return'") {
		t.Errorf("said %q, want the place refused", out)
	}
	if !strings.Contains(out, "ret=2") {
		t.Errorf("said %q, want ret=2", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The other half of the same reordering: inside a function only the operand
// is wrong, so only the operand is named — and the call still hands 2 back
// rather than ending the script.
func TestARefusedReturnOperandInsideAFunctionNamesOnlyTheOperand(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "f(){ return abc; }\nf\necho \"r=$?\"\necho alive\n")
	if !strings.Contains(out, "numeric argument required") {
		t.Errorf("said %q, want the operand refused", out)
	}
	if strings.Contains(out, "can only `return'") {
		t.Errorf("said %q, want nothing about the place", out)
	}
	if !strings.Contains(out, "r=2") || !strings.Contains(out, "alive") {
		t.Errorf("said %q, want r=2 and the script carrying on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
