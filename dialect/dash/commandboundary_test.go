// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The `command` word is a **boundary** here: a fatal error raised anywhere
// inside the builtin it ran unwinds as far as the word and stops there, so the
// text the builtin was running is abandoned and the script carries on.
//
// Measured 2026-09-13 from a script file under `env -i`, each row written
// twice so it is about the word rather than about the error. Without the word
// every one of these ends the script; with it every one reports the error's
// own status and reaches the line after (#2755).
//
// This is not the narrower thing `command` already did — taking the named
// builtin's specialness away (#2741) — because none of these errors is a
// special builtin's refusal at all except the first, and even that one is
// raised by a builtin the `command` did not name.
func TestTheCommandWordEndsAFatalErrorRaisedInsideIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		text  string
	}{
		{"a special builtin's refusal", "", "export -q"},
		{"`${x?word}` firing", "", `echo "${NOPE?bad}"`},
		{"a readonly reassignment", "readonly rv=1\n", "rv=2"},
		{"an unset name under `set -u`", "set -u\n", `echo "${NOPE}"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			// The control: bare, the error ends the script. Without this the
			// test would pass under a build that had simply stopped calling
			// the error fatal.
			out, st := runDash(t, dir, tc.setup+"eval '"+tc.text+"; echo inner'\necho after\n")
			if strings.Contains(out, "after") || st == 0 {
				t.Fatalf("bare %q gave %q at %d, want the script to stop", tc.text, out, st)
			}

			out, st = runDash(t, dir, tc.setup+"command eval '"+tc.text+"; echo inner'\necho \"st=$?\"\necho after\n")
			if !strings.Contains(out, "after") || st != 0 {
				t.Errorf("`command eval '%s'` gave %q at %d, want the script to carry on", tc.text, out, st)
			}
			if strings.Contains(out, "inner") {
				t.Errorf("`command eval '%s'` gave %q, want the rest of the `eval`'s text abandoned", tc.text, out)
			}
			if !strings.Contains(out, "st=2") {
				t.Errorf("`command eval '%s'` gave %q, want the error's own status 2 reported", tc.text, out)
			}
		})
	}
}

// And `${x?word}` is caught here even though this shell reads that operand as
// a request to *stop* rather than as an error everywhere else — the row above
// covers it, and this says why it is not a contradiction: the axis that makes
// the operand an exit request is answered yes, and the word still catches it.
func TestTheCommandWordEndsTheErrorOperandThisShellCallsAnExitRequest(t *testing.T) {
	if got := preset.Semantics().ParamErrorIsAnExitRequest; got != interp.Yes {
		t.Fatalf("ParamErrorIsAnExitRequest = %v, want yes — this row is about the operand that is not an ordinary error here", got)
	}
}

// What the boundary must not catch is a request to stop, which is unanimous
// across the panel: an `exit` inside it exits, a failed `exec` ends the shell,
// and errexit firing under it stops.
func TestTheCommandWordBoundaryLetsARequestToStopThrough(t *testing.T) {
	dir := t.TempDir()

	out, st := runDash(t, dir, "command eval 'exit 5'\necho after\n")
	if strings.Contains(out, "after") || st != 5 {
		t.Errorf("`command eval 'exit 5'` gave %q at %d, want the shell to exit 5", out, st)
	}

	out, st = runDash(t, dir, "set -e\ncommand eval false\necho after\n")
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("errexit under `command` gave %q at %d, want the script to stop", out, st)
	}

	out, st = runDash(t, dir, "command exec /nonexistent/prog\necho after\n")
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("`command exec /nonexistent/prog` gave %q at %d, want the shell to end", out, st)
	}
}
