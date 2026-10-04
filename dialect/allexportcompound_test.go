// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set -a` marks an array in zsh and in nobody else** —
// Semantics.AllexportMarksACompound. Measured 2026-10-03: under `set -a`,
// zsh 5.9.2 lists `A=(1)` as `typeset -ax A=( 1 )` and a declared table as
// `typeset -Ax`, where bash 5.3.20 lists `declare -a A=([0]="1")` unmarked.
// A scalar is marked in both, which is the control.
func TestAllexportMarksAnArrayOnlyInZsh(t *testing.T) {
	src := "set -a\nA=(1)\ntypeset -A H=(k v)\nF=x\ntypeset -p A H F\n"
	for name, want := range map[string]string{
		"zsh":  "typeset -ax A=( 1 )\ntypeset -Ax H=( [k]=v )\nexport F=x\n",
		"bash": "declare -a A=([0]=\"1\")\ndeclare -A H=([k]=\"v\" )\ndeclare -x F=\"x\"\n",
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
