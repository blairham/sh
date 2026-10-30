// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"strings"
	"testing"
)

// A shell that is its process hands an ignore to the commands it starts, which
// is the half of `trap` a library Runner cannot have.
//
// interp keeps the ignore of a Runner with no SetSignalDisposition to that
// Runner, because the only ignore a child inherits is the process's and taking
// it would deafen every other subscriber in the program (#7044). So the
// inheritance every shell in the panel gives — measured 2026-10-29, an ignored
// USR1 and then a child that raises it at itself survives in bash 5.3, bash
// 3.2, ksh93, zsh and dash, and a reset gives the next child the default
// again — rests on the front end filling the hook in, and this is the test
// that it does. Each shell is a re-execution of this binary, so the
// disposition it sets is its own process's and nobody else's.
//
// The probe is a child of a child. What dies of the signal is the grandchild,
// so the shell under test reaps a command that exited rather than one a signal
// ended, and no dialect's answer about that status is needed to read the row.
// Both marks are computed, because the child's notice of the grandchild's
// death quotes the grandchild's command line, and a mark written out in it
// would be found there whether or not anything survived.
func TestAShellThatIsItsProcessHandsAnIgnoreToItsChildren(t *testing.T) {
	dieRunningAsAShell()
	const probe = `/bin/sh -c '/bin/sh -c "kill -USR1 \$\$; echo alive-\$((6*7))"; echo probed-$((2*3))'`
	for _, tc := range []struct {
		name, src string
		survives  bool
	}{
		// The control, and the reason the other rows mean anything: with
		// nothing arranged the grandchild dies, so the probe can say so.
		{"nothing arranged", probe, false},
		{"ignored", "trap '' USR1; " + probe, true},
		{"ignored, then reset", "trap '' USR1; trap - USR1; " + probe, false},
		// A handled signal is reset to its default by exec, which is the
		// difference between a handler and an ignore that a child can see.
		{"handled", "trap ':' USR1; " + probe, false},
		// A subshell is a pretend process, and its ignore is its own: the
		// commands it starts inherit it, and nothing after it does (#7049).
		{"ignored in a subshell", "(trap '' USR1); " + probe, false},
		{"ignored in the subshell that starts it", "(trap '' USR1; " + probe + "); :", true},
		// And the mirror, which takes the ignore off for the length of the
		// start rather than putting one on.
		{"ignored, then reset in a subshell", "trap '' USR1; (trap - USR1; " + probe + "); :", false},
		// The replacement inherits what the shell ignored, and so does the
		// command the shell becomes at the end of its string (#7050).
		{"ignored, then exec", "trap '' USR1; exec " + probe, true},
		{"ignored, and not last", "trap '' USR1; " + probe + "; :", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, out := waitStatusOfAShell(t, "^TestAShellThatIsItsProcessHandsAnIgnoreToItsChildren$", tc.src)
			if !strings.Contains(out, "probed-6") {
				t.Fatalf("%s: the probe never finished, so it answered nothing: %q", tc.src, out)
			}
			if got := strings.Contains(out, "alive-42"); got != tc.survives {
				t.Errorf("%s: the grandchild survived = %v, want %v (wrote %q)", tc.src, got, tc.survives, out)
			}
		})
	}
}
