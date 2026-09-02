// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An assignment with no command name asks two questions in one statement, and
// the order between them is the whole of it: `$?` on the right-hand side names
// the command *before* the assignment, and the assignment then reports a
// status of its own. Answering the second first made `E=$?` read 0 — the most
// common idiom in shell, silently returning success.
func TestAnAssignmentReadsTheStatusBeforeSettingIts(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the right-hand side sees the previous status", `false; E=$?; echo "E=$E"`, "E=1\n"},
		{"and the assignment reports its own", `false; E=$?; echo "?=$?"`, "?=0\n"},
		// The second read sees the assignment's success, not the failure —
		// which is what makes this an ordering question rather than a
		// question about saving `$?` somewhere.
		{"only the first reads it", `false; E=$?; F=$?; echo "E=$E F=$F"`, "E=1 F=0\n"},
		{"a function body is no different", `f(){ false; E=$?; echo "E=$E"; }; f`, "E=1\n"},
		{"twice in one word", `false; x=${?}${?}; echo "$x"`, "11\n"},
		{"an append reads it too", `false; E+=$?; echo "E=$E"`, "E=1\n"},

		// The other half of the rule, and the reason the status cannot just
		// be zeroed after the fact either.
		{"a substitution reports through the assignment", `true; x=$(false); echo "st=$?"`, "st=1\n"},
		{"and a plain assignment reports success", `false; y=1; echo "st=$?"`, "st=0\n"},
		{"even when the substitution succeeds", `false; x=$(true); echo "st=$?"`, "st=0\n"},
		// The case a status comparison cannot tell apart: what the
		// substitution reported is what was already there. Nothing about the
		// status afterwards distinguishes it from no substitution at all.
		{"a substitution reporting what was already there", `false; x=$(false); echo "st=$?"`, "st=1\n"},
		{"the last of several is the one", `false; x=$(false) y=$(true); echo "st=$?"`, "st=0\n"},
		// Per statement, not once for the shell: an earlier substitution
		// anywhere must not make a later plain assignment keep whatever
		// status was lying around.
		{"a substitution in an earlier statement does not carry", `x=$(true); false; y=1; echo "st=$?"`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// An assignment that stops the script keeps the status that stopped it. The
// success an assignment reports is reported only when it happened — so the
// success is decided after the fatal check and not before, which is the one
// ordering the change above can still get wrong.
func TestAFatalAssignmentKeepsItsOwnStatus(t *testing.T) {
	sem := PosixSemantics()
	_, st := run(t, `readonly r=1; r=2`, func(r *Runner) { r.Semantics = &sem })
	if st != 2 {
		t.Errorf("status %d, want the status that stopped the script", st)
	}
}
