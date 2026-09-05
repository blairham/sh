// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestTheExitTrapFiresAfterARefusal is the sibling of
// TestTheExitTrapFiresAfterAParseFailure, and it is here because the two used
// to disagree.
//
// A refusal is the interpreter saying it has nothing to run rather than a
// command reporting that it failed: a builtin this shell must answer itself
// and no longer can comes back from RunPart as an error. It is an ending like
// any other and the front end skipped the EXIT trap for it alone — the one
// arm that disagreed with the rule beside it, and with interp's own Run,
// which has always run the trap on the same error.
//
// It survived mutation for the reason a rule with no exercised exception
// does: nothing in the corpus refuses, because the panel has nothing to
// compare a refusal against.
//
// A script rather than a command string, for the reason the parse-failure
// test uses one: the trap has to be set by a line that already ran.
func TestTheExitTrapFiresAfterARefusal(t *testing.T) {
	sh := shell()
	// The same answer the parse-failure test needs, and for the same reason:
	// a trap body is re-parsed when it fires, and how much of one runs is an
	// axis the core leaves open. It is not what this test is about.
	sh.Semantics.TrapBodyRunsWhatParsed = interp.Yes

	path := writeScript(t,
		"trap 'echo bye' EXIT\necho one\nenable -n umask\numask 077\necho unreached\n")
	out, errs, code := runArgs(t, sh, "testsh", path)
	if want := "one\nbye\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if !strings.Contains(errs, "umask") {
		t.Errorf("the refusal was not reported: stderr %q", errs)
	}
	if code != 2 {
		t.Errorf("exited %d, want 2", code)
	}
}

// A session is the other half of the same question and the answer is the
// opposite one: it holds the shell open across inputs, so a refusal ends the
// input and not the shell. The trap is due when the session closes and not
// before, and running it per input would run it once per prompt.
func TestARefusalDoesNotEndASession(t *testing.T) {
	s, out, errs := session(t)

	if code := s.Run(t.Context(), "trap 'echo bye' EXIT; echo one"); code != 0 {
		t.Errorf("the first input exited %d, want 0", code)
	}
	if code := s.Run(t.Context(), "enable -n umask; umask 077"); code != 2 {
		t.Errorf("the refusal exited %d, want 2", code)
	}
	if !strings.Contains(errs.String(), "umask") {
		t.Errorf("the refusal was not reported: stderr %q", errs.String())
	}
	if got := out.String(); got != "one\n" {
		t.Errorf("output %q before the session closed, want %q — the trap is not due yet", got, "one\n")
	}
	s.Close(t.Context())
	if want := "one\nbye\n"; out.String() != want {
		t.Errorf("output %q after Close, want %q", out.String(), want)
	}
}
