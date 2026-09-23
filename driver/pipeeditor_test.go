// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason promptecho_test.go is: what is asserted is that
// an unexported builder reads a dialect's answer.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether the editing keys are read where there is no terminal is the
// dialect's answer, carried to the prompt rather than decided there.
//
// The same majority argument as the echo beside it, and the same silence: three
// of the four shells with a prompt here read every key as text, so a wiring
// that always passed false would pass every test written against them while the
// one shell with an editor lost `C-r`, the arrows and every other binding
// (#4249).
func TestTheFrontEndCarriesWhetherTheEditorReadsKeysWithoutATerminal(t *testing.T) {
	for _, answer := range []bool{false, true} {
		sh := Shell{
			Name:      "testsh",
			Semantics: interp.Semantics{EditorReadsKeysWhereThereIsNoTerminal: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).EditorWithoutATerminal; got != answer {
			t.Errorf("the prompt was told %v, want %v", got, answer)
		}
	}
}
