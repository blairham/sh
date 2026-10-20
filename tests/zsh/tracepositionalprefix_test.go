// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This is the one column that reads `1=X f` as an assignment at all, and it
// traces it like any other prefix. Ours performed the assignment and left the
// word out of the line, which made this the single position where `set -x`
// wrote a command and not what was assigned in front of it.
//
// Measured 2026-09-17 against zsh 5.9.2 over a script file, `env -i` with
// LC_ALL=C: `set -- p q; 1=X g` writes the assignment, this shell's second
// prefix, and then the command (#3157).
func TestAPositionalParameterInAPrefixIsTraced(t *testing.T) {
	out, st := answersRun(t, "g() { :; }\nset -x\nset -- p q\n1=X g\nset +x\n")
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if !strings.Contains(out, "1=X ") {
		t.Errorf("got %q, want the assignment in the line", out)
	}
}

// And the value reaches the store from the trace rather than being expanded a
// second time for it, which is what keeps a substitution in the prefix from
// running twice. Measured the same day: `1=$(...)` runs its command once
// under a trace, exactly as it does without one.
func TestATracedPositionalPrefixExpandsItsValueOnce(t *testing.T) {
	// Counted in a file rather than on the output, because the trace itself
	// echoes the substitution's own command and a count of the text would
	// count the line that reports the run as well as the run.
	src := "g() { :; }\nset -- p q\nset -x\n1=$(echo r >>ran; echo v) g\nset +x\n" +
		"echo param=$1 ran=$(grep -c r ran)\n"
	out, st := answersRun(t, src)
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	if !strings.Contains(out, "param=v ran=1") {
		t.Errorf("got %q, want `param=v ran=1` — the value stored and the substitution run once", out)
	}
}
