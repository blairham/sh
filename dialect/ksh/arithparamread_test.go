// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// An unreadable `${...}` inside an arithmetic expression is refused while the
// script is read, as it is outside one, even in a branch never taken.
// Measured 2026-10-03 on ksh93u+.
func TestAnExpansionInArithmeticIsReadWithTheScript(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`if false; then echo $(( ${a b} )); fi; echo r`, "ksh: syntax error at line 1: ` ' unexpected\n"},
		{`if false; then (( ${a b} )); fi; echo r`, "ksh: syntax error at line 1: ` ' unexpected\n"},
		{`a=(x y z); echo "[$(( ${${a[@]}[(I)zz]} == 0 ))]"`, "ksh: syntax error at line 1: `!' unexpected\n"},
		// The controls: readable expansions run as before.
		{`a=(1 2); x=3; echo $(( ${a[1]} + ${#a[@]} + ${x:-2} ))`, "7\n"},
	} {
		var out, errs bytes.Buffer
		driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", c.src})
		if got := out.String() + errs.String(); got != c.want {
			t.Errorf("%s: %q, want %q", c.src, got, c.want)
		}
	}
}
