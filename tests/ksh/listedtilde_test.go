// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `~` in a listed value or a listed key is quoted wherever it stands here,
// which is bash's position rule refused. Measured 2026-09-17 on ksh93u+
// 2012-08-01 under `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, from a
// script file: `v='a~b'` and `w='b~'` from a bare `set` — filtered by a
// `case` rather than by `grep`, since a preset test has no PATH — and the key
// `['a~b']` from a `typeset -p` of a keyed table.
//
// The column bash's Semantics.ListedTildeIsBareWhereItCannotExpand parts from, and
// the guard that the answer did not leak out of that dialect (#2298).
func TestAListedTildeIsQuotedWhereverItStands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value with a tilde inside it", `v='a~b'; set | while IFS= read -r l; do case $l in v=*) echo "$l";; esac; done`, "v='a~b'\n"},
		{"a value ending in a tilde", `w='b~'; set | while IFS= read -r l; do case $l in w=*) echo "$l";; esac; done`, "w='b~'\n"},
		{"a value a tilde opens", `u='~b'; set | while IFS= read -r l; do case $l in u=*) echo "$l";; esac; done`, "u='~b'\n"},
		{
			"a key with a tilde inside it",
			`k='a~b'; typeset -A m; m[$k]=1; typeset -p m`,
			"typeset -A m=(['a~b']=1)\n",
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

// The leading unquoted `~` of an associative subscript *is* expanded here, as
// it is in bash: `typeset -A m; m[~/k]=v` stores under `$HOME/k` and
// `${m[~/k]}` reads it back. Measured 2026-09-17 on ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, from a script file —
// Semantics.SubscriptKeyExpandsALeadingTilde, where zsh is the column that
// takes the characters (#2298).
func TestASubscriptsLeadingTildeNamesTheHomeDirectory(t *testing.T) {
	// HOME is set by the script: the preset harness gives a run no home of
	// its own, and a `~` with nothing to expand to is left alone in every
	// column, which would make the row pass for the wrong reason.
	const src = `HOME=/h; typeset -A m; m[~/k]=v; ` +
		`case "${!m[@]}" in "$HOME"/k) echo "key is the path";; *) echo "key is ${!m[@]}";; esac; ` +
		`typeset -A n; n[$HOME/k]=stored; echo "read: [${n[~/k]}]"; ` +
		`typeset -A o; o["~/k"]=q; case "${!o[@]}" in '~/k') echo "quoted stays";; *) echo "quoted moved";; esac`
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "key is the path\nread: [stored]\nquoted stays\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
