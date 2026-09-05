// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, because the thing being asserted is that two *unexported*
// builders read one field. Both are reachable only from here, and asserting it
// through behavior would mean driving a prompt with a store and an audit file
// at once to observe a value that either matches or does not.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The prompt and the interpreter are given the same identity.
//
// This is the join the identity exists for: what a session records about a
// command it ran, and what the event stream records about the same command,
// have to name one session. They are assembled in two places from one Shell,
// which is exactly the shape that lets two values drift apart — and they did,
// before this: the prompt's block store made an id of its own and the event
// stream carried none, so the two files described the same commands and could
// not be lined up.
func TestThePromptAndTheRunnerShareTheSession(t *testing.T) {
	sh := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	if sh.Session == "" {
		t.Fatal("withDefaults left the run with no identity")
	}
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if r.Session != sh.Session {
		t.Errorf("the runner has session %q, want the shell's %q", r.Session, sh.Session)
	}
	if front.Session != sh.Session {
		t.Errorf("the prompt has session %q, want the shell's %q", front.Session, sh.Session)
	}
}

// The identity is made once, so two Shells defaulted from one are still one
// run only if the caller says so.
//
// withDefaults runs on every route and must not mint a second id for a Shell
// that already has one, because both routes call it and a prompt that
// re-defaulted would end up recording under a name its own Runner never saw.
func TestDefaultingTwiceKeepsTheFirstIdentity(t *testing.T) {
	first := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	again := first.withDefaults([]string{"testsh"})
	if again.Session != first.Session {
		t.Errorf("defaulting again changed the session from %q to %q",
			first.Session, again.Session)
	}
}
