// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A letter written under a plus selects nothing in a listing here, and the
// minus letters beside it select as they would alone. Measured 2026-10-03 on
// bash 5.3.20 over `qa=1; export qb=2; typeset -i qc=3`, the listing narrowed
// to those three names. See Semantics.PlusLetterSelectsAListing.
func TestAPlusLetterSelectsNothingInAListing(t *testing.T) {
	const table = "qa=1; export qb=2; typeset -i qc=3; "
	const narrow = " | grep -E '^(declare -[-a-z]+ )?q[abc]='"
	for _, tc := range []struct{ src, want string }{
		{"typeset +x", "qa=1\nqb=2\nqc=3\n"},
		{"typeset +i", "qa=1\nqb=2\nqc=3\n"},
		{"typeset +xi", "qa=1\nqb=2\nqc=3\n"},
		{"typeset +gx", "qa=1\nqb=2\nqc=3\n"},
		{"typeset +x -i", "declare -i qc=\"3\"\n"},
		{"typeset -i +x", "declare -i qc=\"3\"\n"},
		{"typeset +x +p", "declare -- qa=\"1\"\ndeclare -x qb=\"2\"\ndeclare -i qc=\"3\"\n"},
		{"typeset -x", "declare -x qb=\"2\"\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, table+tc.src+narrow)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}

func TestAPlusLetterSelectingNothingIsThisDialectsAnswer(t *testing.T) {
	out, _ := answersRun(t, "export qb=2; typeset +x | grep -c '^qb=2$'")
	if strings.TrimSpace(out) != "1" {
		t.Errorf("= %q, want the valued row the bare listing writes", out)
	}
}
