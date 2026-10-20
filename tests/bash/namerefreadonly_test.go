// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestTheReferenceLetterIsRefusedOverAFrozenName — the `n` letter over a name
// the script has made readonly is refused, exactly as the type and array
// letters already were.
//
// It is not a value shape: what the letter changes is where the name's reads
// and writes land, which a frozen name has more reason to refuse than a letter
// about the value's type. This shell took all three spellings at 0 and
// redirected a name the script had frozen.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree; ksh93u+ 2012-08-01 refuses the same shape
// in its own words. The `local -n` form already refused it — that path asks the
// same gate one operand loop over, which is what made the gap visible (#4178).
func TestTheReferenceLetterIsRefusedOverAFrozenName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"with no value", "declare -r A=x\ndeclare -n A\necho \"st=$?\"\n", "st=1\n"},
		{"with a value", "declare -r B=x\ndeclare -n B=y\necho \"st=$?\"\n", "st=1\n"},
		{"and readonly again with it", "declare -r C=x\ndeclare -nr C=x\necho \"st=$?\"\n", "st=1\n"},
		// The controls, which say it is the letter and not the declaration:
		// the readonly letter over the same name is taken, the reference
		// letter under a plus is taken, and the letter over a name nothing has
		// frozen is taken.
		{"the readonly letter again", "declare -r E=x\ndeclare -r E\necho \"st=$?\"\n", "st=0\n"},
		{"the letter under a plus", "declare -r G=x\ndeclare +n G\necho \"st=$?\"\n", "st=0\n"},
		{"over a name nothing froze", "declare -n I=x\necho \"st=$?\"\n", "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBashSplitFatal(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// And the sentence is the one the other letters already give.
	_, errs := runBashSplitFatal(t, "declare -r A=x\ndeclare -n A\n")
	if want := "declare: A: readonly variable"; !strings.Contains(errs, want) {
		t.Errorf("stderr = %q, want it to hold %q", errs, want)
	}
}
