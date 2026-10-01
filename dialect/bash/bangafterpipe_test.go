// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A `!` after a bar is refused at the `!`, in this dialect's words (#5256).
// Measured 2026-09-30 over `-c` on bash 5.3.20, which writes these lines byte for
// byte under its own name; the command ran here as one named `!` before.
func TestABangAfterABarIsRefusedInThisDialectsWords(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo | ! true", "bash: -c: line 1: syntax error near unexpected token `!'\nbash: -c: line 1: `echo | ! true'\n"},
		{"! true | ! true", "bash: -c: line 1: syntax error near unexpected token `!'\nbash: -c: line 1: `! true | ! true'\n"},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Fatalf("%q parsed, want a refusal", tc.src)
		}
		if got := bash.Diagnostics().ParseDiagnostic("bash", "-c", err, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}
