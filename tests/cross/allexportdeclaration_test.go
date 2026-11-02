// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set -a` and a declaration**: a local given a value is marked in every
// column, and a name a declaration with no value brings into being is marked
// as Semantics.AllexportMarksAGlobalDeclaredWithoutAValue and
// AllexportMarksALocalDeclaredWithoutAValue say. Each script holds the issue's
// rows for its shell, and `x` is the control: it holds a value before `set
// -a`, so a declaration of it brings nothing into being and nothing marks it.
//
// Measured 2026-11-01 under `-c` with `env -i PATH=/usr/bin:/bin`: bash
// 5.3.20, zsh 5.9.2 under `-f`, dash 0.5.12, ksh93u+ and BusyBox ash in the
// pinned alpine image, each column's output here byte for byte (#7283).
func TestAllexportReachesADeclaration(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ src, want string }{
		"bash": {
			`x=1; set -a; f() { local sa; declare -p sa; local sb=1; declare -p sb; local sc=; declare -p sc; }; f; declare gd; declare -p gd; declare x; declare -p x`,
			"declare -- sa\ndeclare -x sb=\"1\"\ndeclare -x sc=\"\"\ndeclare -x gd\ndeclare -- x=\"1\"\n",
		},
		// The `env` row is the one the mark must not move: zsh marks the
		// valueless local and still tells a child nothing about it.
		"zsh": {
			`x=1; set -a; f() { local sa; print -r -- ${(t)sa}; env | grep -c "^sa="; local sb=1; print -r -- ${(t)sb}; typeset tc; print -r -- ${(t)tc}; }; f; typeset gd; print -r -- ${(t)gd}; typeset x; print -r -- ${(t)x}`,
			"scalar-local-export\n0\nscalar-local-export\nscalar-local-export\nscalar-export\nx=1\nscalar\n",
		},
		// dash's local starts as the outer binding, so its `x` is not
		// brought into being; BusyBox ash's starts unset, and is.
		"dash": {
			`x=1; set -a; f() { local sa; local sb=1; local x; export -p | grep -E " (sa|sb|x)"; }; f`,
			"export sa\nexport sb='1'\n",
		},
		"ash": {
			`x=1; set -a; f() { local sa; local sb=1; local x; export -p | grep -E " (sa|sb|x)"; }; f`,
			"export sa\nexport sb='1'\nexport x\n",
		},
		"ksh": {
			`x=1; set -a; typeset gd; function f { typeset sa; typeset sb=1; typeset -p sa sb; }; f; typeset -p gd; set +a; gd=5; env | grep -c ^gd=`,
			"typeset -x sb=1\n0\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, "PATH=/usr/bin:/bin\n"+tc.src+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
