// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A call that unsets OPTIND and assigns it again** —
// Semantics.OptindUnsetInACallStaysUnbound. Measured 2026-10-15 under `-c`
// on ksh93u+, zsh 5.9.2 and bash 5.3.20; the `function` word is ksh93's
// form, which zsh and bash read as their own (#6633).
func TestAnOptindUnsetInACallStaysUnbound(t *testing.T) {
	const src = `getopts a o -a; function f { unset OPTIND; OPTIND=3; }; f; echo "[${OPTIND-unset}]"; function g { OPTIND=3; }; g; echo "[${OPTIND-unset}]"` + "\n"
	for name, want := range map[string]string{
		"ksh":  "[3]\n[3]\n",
		"zsh":  "[1]\n[1]\n",
		"bash": "[3]\n[3]\n",
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
