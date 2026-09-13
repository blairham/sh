// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// This shell does nothing about output that never ended its line.
//
// Measured 2026-09-12 through a pseudo-terminal with an rc file ending
// `printf 'LEFTOVER'`: the prompt is drawn straight onto it — `LEFTOVERP> ` —
// where the other shell with an editor marks the output and starts a row
// below. It has neither option and does not clear the rows under the prompt
// either.
//
// Asserted rather than left blank by default, because "this dialect gives no
// answer" and "this dialect answers no" look identical in the struct and only
// one of them is a measurement.
func TestThisShellDoesNotMarkUnfinishedOutput(t *testing.T) {
	style := bash.EditorStyle()
	if style.MarkUnfinishedOutputOption != "" {
		t.Errorf("marking option = %q, want none — this shell has no such option",
			style.MarkUnfinishedOutputOption)
	}
	if style.ReturnBeforeThePromptOption != "" {
		t.Errorf("return option = %q, want none", style.ReturnBeforeThePromptOption)
	}
	if style.ClearsBelowThePrompt {
		t.Error("the rows below the prompt are cleared; measured, this shell leaves them")
	}
}
