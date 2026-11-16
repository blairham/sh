// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A copy whose EXIT trap runs ahead of a signal it sent itself still dies of
// that signal after an `exit` in the trap, unless the dialect lets the `exit`
// override the death. That is the answer the shell at the top already gives,
// now asked of the copy as well (#7403).
//
// Measured 2026-11-04, each shell in a process group of its own. bash 5.3.20:
// `( trap 'echo E; exit 5' EXIT; kill -USR1 $BASHPID; echo after ); echo st=$?`
// writes E and st=158, and `x=$( … )` the same. ksh93u+, in parentheses
// forked by `ulimit -t unlimited` and signaled by a child's `kill -USR1
// $PPID`, writes E and st=5. A bare `exit` saves nothing in either. A
// `(exit 4)` inside the trap is that copy's own and leaves the death alone.
//
// The body names itself through a builtin that reads the group the body
// leads, which is the number `$BASHPID` gives a script. The rows are read
// with both answers, so each must move with the axis or hold still.
func TestACopysExitInItsExitTrapOverridesItsOwnDeathInOneDialect(t *testing.T) {
	usr1 := strconv.Itoa(128 + int(syscall.SIGUSR1))
	for _, tc := range []struct {
		name, trap string
		yes, no    string
	}{
		{"exit 5", `echo E; exit 5`, "E\nst=5\n", "E\nst=" + usr1 + "\n"},
		{"exit 0", `echo E; exit 0`, "E\nst=0\n", "E\nst=" + usr1 + "\n"},
		{"a bare exit", `echo E; exit`, "E\nst=" + usr1 + "\n", "E\nst=" + usr1 + "\n"},
		{
			"an exit in a copy inside the trap", `echo E; (exit 4); echo in=$?`,
			"E\nin=4\nst=" + usr1 + "\n", "E\nin=4\nst=" + usr1 + "\n",
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				want := tc.no
				if answer == Yes {
					want = tc.yes
				}
				src := `( trap '` + tc.trap + `' EXIT; selfpid; kill -USR1 $p; echo after ); echo "st=$?"`
				sem := exitTrapDeathSem(answer, No)
				// Whether the death is remarked on is a question of its own,
				// and the status is what these rows are about.
				sem.ReportsACommandKilledBySignal = No
				got := runCopyExitTrapDeath(t, src, sem)
				if !strings.HasSuffix(got, want) || strings.Contains(got, "after") {
					t.Errorf("%s\n got %q\nwant it to end %q, with no `after`", src, got, want)
				}
			})
		}
	}
}

// runCopyExitTrapDeath runs src under sem with a placeholder for the bodies'
// groups and a `selfpid` builtin that sets `p` to the group the body running
// it leads. The run is given a generous bound, and a run that reached it
// fails the test rather than reporting what it had written so far.
func runCopyExitTrapDeath(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf output
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH(),
		ProcessAnchor: anchorCommand(t),
	})
	r.Register("selfpid", func(body *Runner, _ context.Context, _ []string) int {
		pgid, ok := body.SubshellProcessGroup()
		if !ok {
			return 1
		}
		body.SetVar("p", strconv.Itoa(pgid))
		return 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := r.Run(ctx, f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	settle(r)
	if ctx.Err() != nil {
		t.Fatalf("%s: still running after 20s", src)
	}
	return buf.String()
}
