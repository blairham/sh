// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A bracket expression written across a separator is **not a bracket** here,
// so a word holding nothing else live is not a pattern at all (#4158).
//
// The two options are each other's control and are what makes the claim
// checkable: `nullglob` deletes a pattern that matched nothing and `failglob`
// refuses one, and `[a/b]` survives both — which only a word that was never a
// pattern can do. A quoted separator leaves the bracket a bracket even here,
// which is the row that keeps this about the live one.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from script
// files, on bash 5.3.20 and 5.3.15, in a directory holding `keep` and no
// single-character name:
//
//	written                                    answer
//	shopt -s nullglob; set -- [a/b]            one field, `[a/b]`
//	shopt -s nullglob; set -- [a\/b]           no fields
//	shopt -s nullglob; set -- [zQ]             no fields
//	shopt -s failglob; echo [a/b]              [a/b]
//	shopt -s failglob; echo [a\/b]             no match: [a/b]
//
// See interp.Semantics.BracketHoldingASlashIsStillABracket, answered No here
// and Yes in zsh and ksh93.
func TestABracketHoldingASeparatorIsNotAPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, src, want string }{
		{
			"a live separator leaves the word alone under nullglob",
			`shopt -s nullglob; set -- [a/b]; printf '[%s]' "$#" "$@"`, `[1][[a/b]]`,
		},
		{
			"a quoted separator leaves a bracket, which matches nothing",
			`shopt -s nullglob; set -- [a\/b]; printf '[%s]' "$#" "$@"`, `[0]`,
		},
		{
			"the control: a bracket with nothing to match",
			`shopt -s nullglob; set -- [zQ]; printf '[%s]' "$#" "$@"`, `[0]`,
		},
		{
			"and failglob does not refuse what was never a pattern",
			`shopt -s failglob; printf '[%s]' [a/b]`, `[[a/b]]`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, dir, c.src)
			if out != c.want || st != 0 {
				t.Errorf("%q said %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
