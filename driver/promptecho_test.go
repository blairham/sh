// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason promptrefuse_test.go is: what is asserted is
// that an unexported builder reads a dialect's answer.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether a line read from something that is not a terminal is written back
// is the dialect's answer, carried to the prompt rather than decided there.
//
// Asserted here as well as through a session for the reason the axis beside
// it is: an answer dropped on the floor in this builder looks exactly like a
// dialect that did not answer, and four of the five shells in the panel do
// not echo — so a wiring that always passed false would pass every test
// written against the majority while the one shell that echoes lost the
// left-hand side of every transcript it produced (#2298).
func TestTheFrontEndCarriesWhetherAPromptEchoesTheLine(t *testing.T) {
	for _, answer := range []bool{false, true} {
		sh := Shell{
			Name:      "testsh",
			Semantics: interp.Semantics{PromptEchoesTheLineWhereThereIsNoTerminal: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).EchoTheLineWithoutATerminal; got != answer {
			t.Errorf("the prompt was told %v, want %v", got, answer)
		}
	}
}

// And the word the session writes as it ends, carried the same way. One
// dialect in the panel has one and the rest write nothing, so the same
// majority argument applies: the empty string is what a dropped answer and a
// silent dialect both look like.
func TestTheFrontEndCarriesTheWordForLeavingASession(t *testing.T) {
	for _, answer := range []string{"", "exit"} {
		sh := Shell{
			Name:        "testsh",
			Diagnostics: interp.Diagnostics{LeavingAPromptSession: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).Leaving; got != answer {
			t.Errorf("the prompt was told %q, want %q", got, answer)
		}
	}
}
