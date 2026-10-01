// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A `!` after a bar is refused at the `!`, in this dialect's words (#5256).
// Measured 2026-09-30 over `-c` on /bin/dash, which writes these lines byte for
// byte; the command ran here as one named `!` before.
func TestABangAfterABarIsRefusedInThisDialectsWords(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo | ! true", "dash: 1: Syntax error: \"!\" unexpected\n"},
		{"! true | ! true", "dash: 1: Syntax error: \"!\" unexpected\n"},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Fatalf("%q parsed, want a refusal", tc.src)
		}
		if got := dash.Diagnostics().ParseDiagnostic("dash", "-c", err, tc.src); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}
