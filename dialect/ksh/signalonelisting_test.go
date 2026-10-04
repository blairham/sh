// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration written with nothing but a sign lists no attribute words
// here: `typeset -` is the `set` listing and `typeset +` the bare names, where
// the bare `typeset` is the attributed names with their words. Measured
// 2026-10-03 on ksh93u+; see Semantics.SignAloneListingCarriesAttributeWords.
func TestASignAloneListsNoAttributeWords(t *testing.T) {
	const table = "qa='a b'; typeset -i qb=2; typeset -x qc=3; "
	const narrow = " | grep -E '^(integer |export )?q[abc]'"
	for _, tc := range []struct{ src, want string }{
		{"typeset -", "qa='a b'\nqb=2\nqc=3\n"},
		{"typeset +", "qa\nqb\nqc\n"},
		{"typeset", "integer qb\nexport qc\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, table+tc.src+narrow)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}
