// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// **A function named after a declaration builtin is what the bare word
// runs here**: bash has no declaration reserved words, so the function
// shadows the builtin as it shadows any other. The control for zsh's
// TestADeclarationReservedWordOutranksAFunction (#7128). Measured 2026-11-02
// on bash 5.3 (`/opt/homebrew/bin/bash -c` under `env -i PATH=/usr/bin:/bin`).
func TestADeclarationBuiltinIsShadowedByAFunction(t *testing.T) {
	t.Parallel()
	for _, w := range []string{"typeset", "declare", "export", "local", "readonly"} {
		src := "function " + w + " { echo fn; }; f() { " + w + ` x=1; echo "in rc=$? x=$x"; }; f`
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			out, st := runBash(t, t.TempDir(), src)
			if got := strings.Join(strings.Fields(out), " "); got != "fn in rc=0 x=" || st != 0 {
				t.Errorf("%s\ngot  %q at %d\nwant %q at 0", src, got, st, "fn in rc=0 x=")
			}
		})
	}
}
