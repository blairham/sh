// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestLocalPrintPerformsAnArrayLiteralOperand — `local -p` with an operand
// that is an array literal declares it and lists it, as `declare -p` does,
// and the listing stays narrowed to the running call's own names. Measured
// 2026-10-15 on bash 5.3.20 under `env -i PATH=/usr/bin:/bin`: `g=1; f() {
// local -p y=(1 2) g; }; f` writes `declare -a y=([0]="1" [1]="2")` and then
// `local: g: not found` at 1. This word never reached the helper `declare
// -p` performs its operands through, so `y` was `not found` too (#6450).
func TestLocalPrintPerformsAnArrayLiteralOperand(t *testing.T) {
	out, errs := runBashSplit(t, "g=1\nf() { local -p y=(1 2) g; echo rc=$?; declare -p y; }\nf\n")
	want := "declare -a y=([0]=\"1\" [1]=\"2\")\nrc=1\ndeclare -a y=([0]=\"1\" [1]=\"2\")\n"
	if out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if wantErr := "bash: line 2: local: g: not found\n"; errs != wantErr {
		t.Errorf("said %q, want %q", errs, wantErr)
	}
}

// And the plain operand that carries a value is still `not found`, which is
// the column's reading of every other shape: the helper takes the literal and
// leaves this one to the listing.
func TestLocalPrintStillRefusesAValuedOperand(t *testing.T) {
	out, errs := runBashSplit(t, "f() { x=5; local -p x=7; echo rc=$?; declare -p x; }\nf\n")
	if want := "rc=1\ndeclare -- x=\"5\"\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if wantErr := "bash: line 1: local: x=7: not found\n"; errs != wantErr {
		t.Errorf("said %q, want %q", errs, wantErr)
	}
}
