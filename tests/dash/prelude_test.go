// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// Saying nothing is dash's answer rather than a gap. A script asking which
// shell it is under gets nothing from dash, and that silence is how it tells
// dash from the three that answer — so filling it in would make this dialect
// wrong in the one way that is hardest to notice.
func TestDashNamesItselfInNothing(t *testing.T) {
	if p := dash.Prelude(); p != "" {
		t.Errorf("prelude = %q, want nothing at all", p)
	}
}
