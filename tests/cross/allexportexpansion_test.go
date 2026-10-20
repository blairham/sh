// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set -a` reaches a name an expansion or arithmetic assigns, except in
// ksh93** — Semantics.AllexportMarksAnExpansionsAssignment. Measured
// 2026-10-15 under `-c`, reading a child's environment after `set +a`: bash
// 5.3.20, zsh 5.9.2, dash 0.5.12 and BusyBox ash export `w`, `v` and `m`, and
// ksh93u+ exports none of them. `x=9` is the control, exported in every
// column (#6441).
func TestAllexportReachesAnExpansionsAssignment(t *testing.T) {
	src := "PATH=/usr/bin:/bin\nset -a\nx=9\n: ${w:=1} ${v=2}\n: $((m=3))\nset +a\nenv | grep -E '^(x|w|v|m)=' | sort\n"
	all := "m=3\nv=2\nw=1\nx=9\n"
	for name, want := range map[string]string{
		"bash": all,
		"zsh":  all,
		"dash": all,
		"ash":  all,
		"ksh":  "x=9\n",
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
