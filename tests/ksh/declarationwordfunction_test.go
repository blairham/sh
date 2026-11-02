// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// **A function named after a declaration word is what the bare word runs
// here, where one can be defined at all**: ksh93 has no declaration reserved
// words, so `local` and `declare` — neither of them a command here, `whence
// -v` says `not found` for both — are ordinary function names, while `typeset`, `export` and `readonly`
// refuse the definition as `invalid function name` and end the script at 1.
// The control for zsh's TestADeclarationReservedWordOutranksAFunction
// (#7128). Measured 2026-11-02 on ksh93u+ (`/bin/ksh -c` under `env -i
// PATH=/usr/bin:/bin`).
func TestADeclarationWordFunctionHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		word, want string
		status     int
	}{
		{"local", "fn in rc=0 x=", 0},
		{"declare", "fn in rc=0 x=", 0},
		{"typeset", "invalid function name", 1},
		{"export", "invalid function name", 1},
		{"readonly", "invalid function name", 1},
	} {
		src := "function " + c.word + " { echo fn; }; f() { " + c.word + ` x=1; echo "in rc=$? x=$x"; }; f`
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			out, st := runKsh(t, t.TempDir(), src)
			if got := strings.Join(strings.Fields(out), " "); !strings.Contains(got, c.want) || st != c.status {
				t.Errorf("%s\ngot  %q at %d\nwant %q at %d", src, got, st, c.want, c.status)
			}
		})
	}
}
