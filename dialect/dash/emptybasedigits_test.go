// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestDashHasNoNamedBaseToAskAbout is the verdict
// Semantics.ArithEmptyBaseDigits names for this column.
//
// The axis asks what `8#` comes to. Here there is no `8#` to ask about: dash
// has no named bases at all, so the construct is refused whole and an answer
// on the axis would be a value nothing could reach. That is why this column's
// entry is `unpinned dash:` rather than a policy.
//
// **The control is the row with digits in it**, and it is what makes the
// verdict a measurement rather than a shrug: `$(( 8#7 ))` is refused too, so
// the refusal is the *construct* and not the empty digit run. A column that
// merely disliked the empty spelling would take this one.
//
// Measured 2026-09-28 against `/bin/dash` from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and stdin at `/dev/null`;
// both rows are `arithmetic expression: expecting EOF` there (#5061).
func TestDashHasNoNamedBaseToAskAbout(t *testing.T) {
	for _, src := range []string{
		// The empty spelling the axis is about.
		"8#",
		// And the same spelling with digits, which is the control.
		"8#7",
		"16#ff",
	} {
		t.Run(src, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, "echo $(( "+src+" ))")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(out, "expecting EOF") {
				t.Errorf("$(( %s )) = %q, want the whole construct refused", src, out)
			}
		})
	}
}
