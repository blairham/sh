// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `~` in a listed value or a listed key is quoted wherever it stands here,
// as it is in ksh93 and unlike bash. Measured 2026-09-17 on zsh 5.9.2 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, from a script file:
// `v='a~b'` and `w='b~'` from a bare `set`, and the key `['a~b']` from a
// `typeset -p`.
//
// The guard that bash's Semantics.ListedTildeIsBareWhereItCannotExpand did not
// leak into this column (#2298).
func TestAListedTildeIsQuotedWhereverItStands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value with a tilde inside it", `v='a~b'; set | while IFS= read -r l; do case $l in v=*) echo "$l";; esac; done`, "v='a~b'\n"},
		{"a value ending in a tilde", `w='b~'; set | while IFS= read -r l; do case $l in w=*) echo "$l";; esac; done`, "w='b~'\n"},
		{"a value a tilde opens", `u='~b'; set | while IFS= read -r l; do case $l in u=*) echo "$l";; esac; done`, "u='~b'\n"},
		{
			"a key with a tilde inside it",
			`k='a~b'; typeset -A m; m[$k]=1; typeset -p m`,
			"typeset -A m=( ['a~b']=1 )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
