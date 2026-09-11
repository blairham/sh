// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A math complaint raised by a builtin names it, in front of the sentence:
// `bash: line 1: let: 1+: arithmetic syntax error: …`. Measured against bash
// 5.3.15, 2026-09-11.
//
// The other dialect with `let` words the sentence differently and names it the
// same way; the one that does not name it at all is zsh, which puts a builtin
// in the *location* as a rule and makes this message the exception.
func TestAMathComplaintFromLetNamesTheBuiltin(t *testing.T) {
	for _, src := range []string{`let '1+'`, `let '1 @'`} {
		out, _ := answersRun(t, src)
		if !strings.Contains(out, "let: ") {
			t.Errorf("%s: got %q, want the builtin in front of the sentence", src, out)
		}
	}
}

// And the value is not kept: a byte the reader refuses leaves `let` with
// nothing here, so every one of these is the zero it calls false.
func TestLetKeepsNothingAfterARefusedByte(t *testing.T) {
	for _, src := range []string{`let '1 @'`, `let '0 @'`, `let '1+2 @'`, `let '@'`} {
		if _, st := answersRun(t, src); st != 1 {
			t.Errorf("%s: status %d, want 1", src, st)
		}
	}
}
