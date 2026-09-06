// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason sessionidentity_test.go is: the thing asserted is
// that an unexported builder reads a field, and the prompt it builds exists
// only where there is a terminal.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// What a binary contributes to a prompt reaches the prompt.
//
// The note on frontEnd says why this is worth a test of its own: the editor
// and the prompt are built only where there is a terminal, so a field dropped
// on the way across looks exactly like a binary that contributed nothing, and
// two mutations of this wiring survived everything when it was inline.
func TestAPromptProviderReachesTheFrontEnd(t *testing.T) {
	mine := repl.PromptProviderFunc(func(repl.PromptInfo) string { return "mark " })
	sh := Shell{Name: "testsh", PromptProviders: []repl.PromptProvider{mine}}.
		withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)

	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if len(front.PromptProviders) != 1 {
		t.Fatalf("the prompt carries %d providers, want 1", len(front.PromptProviders))
	}
	if got := front.PromptProviders[0].Prompt(repl.PromptInfo{}); got != "mark " {
		t.Errorf("the provider that arrived draws %q, want %q", got, "mark ")
	}
}

// And a binary that contributes none leaves the prompt as it was.
func TestAShellWithoutProvidersCarriesNoneToTheFrontEnd(t *testing.T) {
	sh := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	if got := sh.frontEnd(r, "testsh", sh.Diagnostics).PromptProviders; len(got) != 0 {
		t.Errorf("the prompt carries %d providers, want none", len(got))
	}
}
