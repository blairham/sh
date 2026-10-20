// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `BASH_ALIASES` is the alias table written as an association, and it is a
// view in both directions: a read follows the table as it is now, and an
// assignment defines an alias.
func TestBashAliasesIsTheAliasTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"one key",
			`alias q=echo; echo "[${BASH_ALIASES[q]}]"`,
			"[echo]\n",
		},
		{
			"the count and the keys",
			`alias q=echo; echo "${#BASH_ALIASES[@]} ${!BASH_ALIASES[@]}"`,
			"1 q\n",
		},
		{
			// A key nothing aliased is empty and is not in the roster,
			// which is what makes `${BASH_ALIASES[x]:-d}` take the default.
			"a key nothing aliased",
			`echo "[${BASH_ALIASES[nosuch]}] ${#BASH_ALIASES[@]}"`,
			"[] 0\n",
		},
		{
			"the listing",
			`alias q=echo; declare -p BASH_ALIASES`,
			`declare -A BASH_ALIASES=([q]="echo" )` + "\n",
		},
		{
			// It is a view and not a snapshot: read, alias, read again in
			// one shell gives two different answers.
			"a later alias is there",
			`echo "${#BASH_ALIASES[@]}"; alias q=echo; echo "${#BASH_ALIASES[@]}"`,
			"0\n1\n",
		},
		{
			// The write half. An assignment defines an alias, measured.
			"an assignment defines an alias",
			`BASH_ALIASES[w]=date; alias`,
			"alias w='date'\n",
		},
		{
			// And an unset of one element does *not* remove the alias,
			// which is bash's answer and not an omission here.
			"an unset element leaves the alias",
			`alias q=echo; unset "BASH_ALIASES[q]"; alias; echo "st=$?"`,
			"alias q='echo'\nst=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// A subshell's alias is the subshell's, which comes free from the view being
// a function of the runner rather than a table copied into the name.
func TestBashAliasesFollowsTheSubshell(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `( alias q=echo; echo "in=${#BASH_ALIASES[@]}" ); echo "out=${#BASH_ALIASES[@]}"`)
	if !strings.Contains(out, "in=1") || !strings.Contains(out, "out=0") {
		t.Errorf("out = %q, want the subshell to have kept its own", out)
	}
}
