// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Where `$_` stops after a declaration** —
// Semantics.UnderscoreStopsAtADeclarationsAssignment. Measured 2026-10-03:
// zsh 5.9.2 leaves it on the last word in front of the first assignment
// operand, array literals included, and bash 5.3.20 on the command's last
// word as written.
func TestUnderscoreAfterADeclaration(t *testing.T) {
	src := `export y=2; echo "a=$_"; export z y=2; echo "b=$_"; typeset -a A=(1 2); echo "c=$_"; builtin export w=1; echo "d=$_"` + "\n"
	for name, want := range map[string]string{
		"zsh":  "a=export\nb=z\nc=-a\nd=w=1\n",
		"bash": "a=y=2\nb=y=2\nc=A\nd=w=1\n",
	} {
		out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if out != want {
			t.Errorf("%s: got %q, want %q", name, out, want)
		}
	}
}
