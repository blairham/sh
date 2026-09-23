// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `typeset +a` and `+A` are refused here where the name is an array, and taken
// in silence where it is not.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20:
//
//	typeset -A a; a[x]=1; typeset +A a   `a: cannot destroy array variables
//	                                    in this way`, status 1, table intact
//	typeset -a b; b[0]=9; typeset +a b   the same sentence
//	typeset -A e; typeset +A e           the same, on an empty table
//	s=plain; typeset +A s                status 0, silent, `s` untouched
//	typeset +A nosuchname                status 0, silent
//
// The last two rows are what make this a policy rather than a boolean: ksh93
// refuses those too and zsh refuses none of them. Ignoring the letter, which is
// what this shell did, is an answer no column was measured giving — the table
// stayed and the script that took the attribute off to reuse the name as a
// scalar carried on with a table nothing said it still had (#4241).
func TestTheArrayAttributeWillNotComeOffAnArrayHere(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refuses         bool
	}{
		{
			name: "a populated table", refuses: true,
			src:  `typeset -A a; a[x]=1; typeset +A a; printf '[%s][%s]' "$?" "${a[x]}"`,
			want: "[1][1]",
		},
		{
			name: "a populated indexed array", refuses: true,
			src:  `typeset -a b; b[0]=9; typeset +a b; printf '[%s][%s]' "$?" "${b[0]}"`,
			want: "[1][9]",
		},
		{
			name: "an empty table", refuses: true,
			src:  `typeset -A e; typeset +A e; printf '[%s]' "$?"`,
			want: "[1]",
		},
		{
			name: "a scalar",
			src:  `s=plain; typeset +A s; printf '[%s][%s]' "$?" "$s"`,
			want: "[0][plain]",
		},
		{
			name: "a name that does not exist",
			src:  `typeset +A nosuchname; printf '[%s]' "$?"`,
			want: "[0]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != 0 {
				t.Errorf("= %q status %d, want it to end %q", out, st, tc.want)
			}
			said := strings.Contains(out, "cannot destroy array variables in this way")
			if said != tc.refuses {
				t.Errorf("= %q, refusal said %v, want %v", out, said, tc.refuses)
			}
		})
	}
}

// The policy, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheArrayAttributeRemovalIsThisDialectsPolicy(t *testing.T) {
	if got := bash.Semantics().ArrayAttributeRemoval; got != interp.ArrayAttributeRemovalRefusedForAnArray {
		t.Errorf("ArrayAttributeRemoval = %v, want refused for an array", got)
	}
}
