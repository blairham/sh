// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason parsestatus_test.go is: what is asserted is that
// an unexported builder reads a dialect's answer.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether a construct the parser refused still asks for another line is the
// dialect's answer, carried to the prompt rather than decided there.
//
// A dialect's answer dropped on the floor in this builder looks exactly like a
// dialect that did not answer, which is why it is asserted here and not only
// through a session: three of the four shells refuse at once and the fourth
// waits, so a wiring that always passed false would pass every test written
// against the majority (#1893).
func TestTheFrontEndCarriesWhetherAPromptAsksAgain(t *testing.T) {
	for _, answer := range []bool{false, true} {
		sh := Shell{
			Name:      "testsh",
			Semantics: interp.Semantics{PromptAsksAgainAfterARefusedToken: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).AskAgainAfterARefusedToken; got != answer {
			t.Errorf("the prompt was told %v, want %v", got, answer)
		}
	}
}
