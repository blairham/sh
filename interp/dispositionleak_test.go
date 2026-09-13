// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

// The leak in #2446, in the shape it was found in.
//
// Two snippets in one process, where the first ignores a signal at the top
// level and the second needs that signal's default action. Nothing connects
// them but the process they share, which is exactly why the symptom landed
// somewhere else: four SIGPIPE failures in unrelated interp tests that read
// as a new flake family, and an agent that cleared its own branch by
// reverting the two files it had touched — an experiment that could not have
// shown anything, since the cause was a corpus row in internal/oracle.
//
// Measured at the level the leak was felt at rather than through
// signal.Ignored, because a child is what the leak reaches: a disposition of
// *ignored* survives exec, which is the whole of what `nohup` does, so the
// tests that failed were failing in processes they had started.
const signalProbe = "/bin/sh -c 'kill -INT $$; echo survived'\n"

// probeDies is the probe's answer when SIGINT still has its default action:
// the child is ended where it stands and prints nothing.
func probeDies(t *testing.T) bool {
	t.Helper()
	out, _ := runUnderBash(t, signalProbe)
	return !strings.Contains(out, "survived")
}

func TestAnIgnoreOneScriptSetDoesNotReachTheNextOnesChildren(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	// The probe is checked against a process nobody has touched first. A
	// probe that answers "default" whatever the disposition is would pass
	// this whole test while proving nothing, and passing for the wrong
	// reason is how the coprocess descriptor test got away with reading a
	// descriptor that is usually already shut.
	if !probeDies(t) {
		t.Fatalf("the probe cannot tell the two states apart: it reported a surviving child with SIGINT at its default")
	}

	// The first snippet: an ignore at the top level, which is the *process's*
	// disposition because that is what a shell at the top level is.
	runUnderBash(t, "trap '' INT\n")

	// The second: the same probe, which must still answer the same way.
	if !probeDies(t) {
		t.Error("`trap '' INT` outlived the script that set it: a later snippet's child inherited the ignore")
	}
}

// And the other direction, which is the half a naive restore gets wrong.
//
// A harness launched under `nohup` hands the shell a signal it was already
// ignoring. Handing that back as a *default* would be the shell rewriting its
// caller's environment on the way out — `nohup` still has to mean what it
// says once the script has finished.
func TestAShellPutsBackAnIgnoreItWasHandedRatherThanADefault(t *testing.T) {
	signal.Ignore(syscall.SIGUSR2)
	t.Cleanup(func() { signal.Notify(make(chan os.Signal, 1), syscall.SIGUSR2) })
	if !signal.Ignored(syscall.SIGUSR2) {
		t.Fatal("this test needs SIGUSR2 ignored to start with")
	}

	runUnderBash(t, "trap 'echo x' USR2\ntrap - USR2\n")

	if !signal.Ignored(syscall.SIGUSR2) {
		t.Error("a shell that was handed an ignored SIGUSR2 gave back a defaulted one")
	}
}
