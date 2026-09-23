// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestACoprocessIsNotPublishedUnderAFrozenName — a coprocess whose name, or
// whose `_PID` companion, is readonly publishes **neither**: the refusal is
// written once for the frozen name, the coprocess still runs, the status is 0,
// and the readonly keeps what the script gave it.
//
// This shell wrote the refusal twice and then stored the descriptors over the
// readonly anyway — `declare -ar RO=([0]="63" [1]="60")` where the script had
// said `declare -r RO="x"`. A construct that modifies a readonly is the shape
// this tree cares about most.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree (#4178).
func TestACoprocessIsNotPublishedUnderAFrozenName(t *testing.T) {
	// The array's name is the frozen one: the refusal names it, and the value
	// the script froze is still there afterwards.
	out, errs := runBashSplitFatal(t, "declare -r RO=x\ncoproc RO { echo hi; }\necho \"st=$?\"\ndeclare -p RO\nwait\n")
	if want := "st=0\ndeclare -r RO=\"x\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if n := strings.Count(errs, "RO: readonly variable"); n != 1 {
		t.Errorf("stderr = %q, want the refusal exactly once, got %d", errs, n)
	}
	// And the companion is the frozen one: the *array* is then never brought
	// into being, which is what says the two go together.
	out, errs = runBashSplitFatal(t, "declare -r R2_PID=7\ncoproc R2 { echo hi; }\necho \"st=$?\"\ndeclare -p R2\nwait\n")
	if !strings.HasPrefix(out, "st=0\n") {
		t.Errorf("got %q, want it to start st=0", out)
	}
	if !strings.Contains(errs, "R2_PID: readonly variable") {
		t.Errorf("stderr = %q, want the companion named", errs)
	}
	if !strings.Contains(errs, "R2: not found") {
		t.Errorf("stderr = %q, want the array never declared", errs)
	}
	// The control: with neither name frozen the ends are published as before.
	out, _ = runBashSplitFatal(t, "coproc OK { echo hi; }\nread -r l <&${OK[0]}\necho \"[$l]\"\nwait\n")
	if want := "[hi]\n"; out != want {
		t.Errorf("control: got %q, want %q", out, want)
	}
}
