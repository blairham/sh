// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// PS4 has a value here in every invocation, which is what makes the trace
// prefix a parameter a script can read and extend rather than a string only
// the tracer knows. Measured 2026-09-17, `env -i` with LC_ALL=C and nothing
// inherited, on `-c` and under `-i` alike (#2928).
func TestTheTracePrefixHasADefaultValue(t *testing.T) {
	if got := dash.PromptStyle().DefaultTrace; got != "+ " {
		t.Errorf("default PS4 = %q, want %q", got, "+ ")
	}
}
