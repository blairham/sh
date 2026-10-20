// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `typeset +A` takes the attribute off here and leaves the name an empty
// scalar, at status 0 and with nothing said.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C zsh f.sh`, standard input on the null device, zsh 5.9.2, with
// `typeset -A a; a[x]=1`: `typeset +A a` is status 0 and `typeset -p a` lists
// `typeset a=”` — where bash refuses the line and keeps the table. Ignoring
// the letter left the table standing here too, which is neither column's
// answer (#4241).
func TestTheArrayAttributeComesOffAndEmptiesTheNameHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a populated table",
			`typeset -A a; a[x]=1; typeset +A a; printf '[%s][%s][%s]' "$?" "$a" "${a[x]}"`,
			"[0][][]",
		},
		{
			"a scalar is left alone",
			`s=plain; typeset +A s; printf '[%s][%s]' "$?" "$s"`,
			"[0][plain]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The policy, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheArrayAttributeRemovalIsThisDialectsPolicy(t *testing.T) {
	if got := zsh.Semantics().ArrayAttributeRemoval; got != interp.ArrayAttributeRemovalEmptiesTheName {
		t.Errorf("ArrayAttributeRemoval = %v, want empties the name", got)
	}
}
