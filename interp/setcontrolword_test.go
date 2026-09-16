// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestSetTakesTwoWordsThatAreNotOptionNames is the prefix rule on its own.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, which is the one shell in the
// panel whose `set` reads a `--name` word as anything at all. The two words
// abbreviate and the *option names* beside them do not — `--g`, `--gl` and
// `--glo` are each `bad option(s)` in the same shell, in the same run — so
// the abbreviation belongs to this pair and is not a property of the
// namespace.
func TestSetTakesTwoWordsThatAreNotOptionNames(t *testing.T) {
	for _, tc := range []struct {
		word, want string
	}{
		{"default", "default"},
		{"state", "state"},
		// Every prefix down to one letter, which is what was measured:
		// `set --d` puts errexit out and `set --s` writes the state line.
		{"d", "default"},
		{"de", "default"},
		{"defa", "default"},
		{"defaul", "default"},
		{"s", "state"},
		{"st", "state"},
		{"stat", "state"},
		// Not a prefix, so not one of them — the word falls through to the
		// option namespace and is refused there.
		{"", ""},
		{"x", ""},
		{"defaults", ""},
		{"stater", ""},
		// Case is not folded, which is that shell's answer for an option
		// name as well: `--DEFAULT` and `--GLOBSTAR` are both refused.
		{"DEFAULT", ""},
		{"Default", ""},
		{"S", ""},
		// Hyphens and underscores are **not** taken out here. ksh93 does
		// fold them — `set --de-fault` and `set --de_fault` both work there,
		// as `set --glob_star` does — but that is its option namespace's
		// rule on every route and not this pair's, and this shell folds
		// neither today. See issue 3155, where the rule is recorded whole.
		{"de-fault", ""},
		{"de_fault", ""},
		{"s-t-a-t-e", ""},
	} {
		t.Run(tc.word, func(t *testing.T) {
			got, ok := SetControlWord(tc.word)
			if tc.want == "" {
				if ok {
					t.Errorf("%q = %q, want no match", tc.word, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("%q = %q %v, want %q true", tc.word, got, ok, tc.want)
			}
		})
	}
}
