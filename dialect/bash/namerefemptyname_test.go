// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAnEmptyNameReferencesItself — the reference letter is the one thing that
// words an empty name differently: a declaration of one under it is refused
// as a reference aimed at itself, not as a bad name.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. This shell wrote the ordinary bad-name
// refusal for every row (#4178).
func TestAnEmptyNameReferencesItself(t *testing.T) {
	const self = "nameref variable self references not allowed"
	for _, tc := range []struct{ name, src, want string }{
		{"declare", "declare -n ''\n", "declare: : " + self},
		{"typeset", "typeset -n ''\n", "typeset: : " + self},
		{"with the readonly letter", "declare -rn ''\n", "declare: : " + self},
		{"with the global letter", "declare -gn ''\n", "declare: : " + self},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := runBashSplitFatal(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", errs, tc.want)
			}
		})
	}
	// Each operand earns its own, and the one beside it is still read.
	_, errs := runBashSplitFatal(t, "declare -n '' ''\n")
	if n := strings.Count(errs, self); n != 2 {
		t.Errorf("two empty operands: stderr = %q, want the sentence twice, got %d", errs, n)
	}
	// The controls. With a **value** the operand is an ordinary bad name in
	// both shells, and under a plus the letter words nothing — so it is the
	// name alone being empty, under the letter's on sign.
	for _, tc := range []struct{ name, src, want string }{
		{"with a value", "declare -n ''=x\n", "declare: `=x': not a valid identifier"},
		{"under a plus", "declare +n ''\n", "declare: `': not a valid identifier"},
		{"with no letter at all", "declare ''\n", "declare: `': not a valid identifier"},
		{"a bad name that is not empty", "declare -n 'a b'\n", "declare: `a b': not a valid identifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := runBashSplitFatal(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", errs, tc.want)
			}
			if strings.Contains(errs, self) {
				t.Errorf("stderr = %q, want no self-reference sentence", errs)
			}
		})
	}
}
