// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.HandlersRunAsASubstitutionEnds: when the handler of a trapped
// signal that another process sends while a command substitution runs runs
// (#7175).

// substHandlers is the test vector with the axis answered a.
func substHandlers(a Answer) Semantics {
	s := testSemantics()
	s.HandlersRunAsASubstitutionEnds = a
	// The background job that sends the signal starts a job table of its
	// own, which the standard leaves to the dialect.
	s.SubshellJobTable = SubshellJobsKept
	return s
}

// landsInASubstitution has the script's background job — another process, in
// every shell this models — send USR1 while the first substitution waits on p,
// and then let it go on. The substitution after it writes `E` as it is
// expanded, so where `T` falls against `E` and `Y` says when the handler ran.
// The one the signal lands in ends at 3, after an `$(exit 4)` that follows
// `false`, so `$?` in the handler says whose status it was shown: 3 is the
// substitution's, 4 is the status from before it, and 1 would be the
// statement's.
const landsInASubstitution = `mkfifo p q; trap 'echo "T st=$?"' USR1; ` +
	`(read y <q; kill -USR1 $$; echo >p) & ` +
	`false; echo Y "$(exit 4)" "$(echo >q; read z <p; exit 3)" "$(echo E >&2)"; echo "after st=$?"; wait`

// TestAHandlerRunsAsASubstitutionEndsWhereTheDialectSays: yes runs it before
// the rest of the command's words are expanded, showing `$?` as the
// substitution left it, or the status from before the substitution — not the
// one from before the statement — where the dialect shows a handler an
// earlier status; no runs it after the command. Measured 2026-10-31 with INT
// from another process: ksh93u+ shows 3 and zsh 5.9.2 shows 4.
func TestAHandlerRunsAsASubstitutionEndsWhereTheDialectSays(t *testing.T) {
	earlier := substHandlers(Yes)
	earlier.SignalHandlerSeesEarlierStatus = Yes
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"yes", substHandlers(Yes), "T st=3\nE\nY   \nafter st=0\n"},
		{"yes, showing the earlier status", earlier, "T st=4\nE\nY   \nafter st=0\n"},
		{"no", substHandlers(No), "E\nY   \nT st=0\nafter st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if out, st := run(t, landsInASubstitution, withSem(tc.sem)); out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestAHandlerRunAsASubstitutionEndsCanLeaveTheShell: an `exit` in it ends the
// shell there, and nothing after the substitution is expanded or run.
// Measured 2026-10-31: `trap 'echo T; exit 3' INT; echo Y "$(/bin/kill -INT
// $$)" "$(echo E >&3)"` writes `T` alone and exits 3 in zsh 5.9.2 and ksh93u+.
func TestAHandlerRunAsASubstitutionEndsCanLeaveTheShell(t *testing.T) {
	src := `mkfifo p q; trap 'echo T; exit 3' USR1; (read y <q; kill -USR1 $$; echo >p) & ` +
		`echo Y "$(echo >q; read z <p)" "$(echo E >&2)"; echo after`
	if out, st := run(t, src, withSem(substHandlers(Yes))); out != "T\n" || st != 3 {
		t.Errorf("got %q at %d, want %q at 3", out, st, "T\n")
	}
}

// TestTheSubstitutionHandlerMomentIsAskedOnlyWhereItDecides: unanswered, the
// axis is refused by name where a handler is waiting for a signal from
// elsewhere as a substitution ends — the command is given up at 2 and the
// handler runs before the next one — and it is asked nowhere else: not where
// nothing arrives, not for the substitution's own `kill $$`, and not inside a
// subshell, whose arrivals the shell takes once the subshell is done.
func TestTheSubstitutionHandlerMomentIsAskedOnlyWhereItDecides(t *testing.T) {
	const asked = "when the handler of a signal that lands during a command substitution runs: " + shellsDisagree
	out, _ := run(t, `mkfifo p q; trap 'echo "T st=$?"' USR1; (read y <q; kill -USR1 $$; echo >p) & `+
		`echo Y "$(echo >q; read z <p)"; echo "after st=$?"; wait`, withSem(substHandlers(Unspecified)))
	if strings.Count(out, asked) != 1 || !strings.HasSuffix(out, "T st=2\nafter st=2\n") {
		t.Errorf("got %q, want the refusal naming the question once, then the handler at 2", out)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"nothing arrives", `trap 'echo T' USR1; echo Y "$(echo x)"; echo after`, "Y x\nafter\n"},
		{
			"the substitution's own kill",
			`trap 'echo T' USR1; echo Y "$(kill -USR1 $$)"; echo after`,
			"Y \nT\nafter\n",
		},
		{
			"in a subshell",
			`mkfifo p q; trap 'echo T' USR1; (read y <q; kill -USR1 $$; echo >p) & ` +
				`(echo Y "$(echo >q; read z <p)"; echo in); echo after; wait`,
			"Y \nin\nT\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if out, st := run(t, tc.src, withSem(substHandlers(Unspecified))); out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0 and nothing asked", out, st, tc.want)
			}
		})
	}
}
