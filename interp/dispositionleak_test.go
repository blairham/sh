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
var signalProbe = childRaising("INT")

// childRaising is that probe for one signal, so the two shapes of this
// question — an ignore a script installed and an ignore it was born with —
// ask it the same way rather than each spelling a child of its own.
func childRaising(name string) string {
	return "/bin/sh -c 'kill -" + name + " $$; echo survived'\n"
}

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

// #2507, which is the same asymmetry seen from inside a single script rather
// than across two of them.
//
// `trap - SIG` after `trap ” SIG` is a script asking for the ignore to go
// away, and until the fix the *shell's* answer was right while the
// *process's* was not: the trap table dropped the entry, signal.Reset left
// SIG_IGN standing, and the next child inherited an ignore the script had
// just disowned. The probe has to be a child for that reason — asking this
// shell what it thinks of SIGINT answers correctly against the bug.
//
// Both halves are asserted in one test because either alone can be satisfied
// by an implementation that is wrong: a shell that never ignores anything
// passes the reset half, and the shell that shipped passes the control half.
func TestAResetGivesBackAnIgnoreBeforeTheNextChildInheritsIt(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	// The probe against a process nobody has touched, for the reason the
	// test above gives: a probe that cannot tell the two states apart would
	// pass this whole test while proving nothing.
	if !probeDies(t) {
		t.Fatalf("the probe cannot tell the two states apart: it reported a surviving child with SIGINT at its default")
	}

	out, _ := runUnderBash(t, "trap '' INT\ntrap - INT\n"+signalProbe)
	if strings.Contains(out, "survived") {
		t.Errorf("a child started after `trap - INT` inherited the ignore `trap '' INT` installed: %q", out)
	}

	// The control, and the half that says this is a reset rather than a
	// shell that has stopped ignoring anything at all. Unanimous in the
	// panel: with no reset, the child is the ignore's to inherit.
	out, _ = runUnderBash(t, "trap '' INT\n"+signalProbe)
	if !strings.Contains(out, "survived") {
		t.Errorf("a child started under `trap '' INT` was not handed the ignore: %q", out)
	}
}

// The other side of the guard, and the reason there is one.
//
// A shell *started* with a signal ignored is a different question from a
// shell that ignored one itself, and the panel splits on it: measured
// 2026-09-12 through `trap ” INT; exec <shell> -c 'trap - INT; …'`, zsh
// hands the inherited ignore back to its children and bash 5.3, bash 3.2,
// ksh93 and dash all keep it. That split already has a name —
// Semantics.QuitResetRestoresTheDefault asks it of the one signal a shell may
// be born ignoring — so the core answers it nowhere, and `trap -` reaches
// only the ignore this script installed.
//
// `nohup` is the shape that makes it concrete: a reset that dropped an
// inherited ignore would be a script switching its caller's decision off from
// the inside, for every child it starts afterwards.
func TestAResetLeavesAloneAnIgnoreTheShellWasBornWith(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh to raise a signal at itself: %v", err)
	}
	probe := childRaising("USR2")
	// The discriminating half first, with SIGUSR2 untouched: the probe has
	// to be able to report a dead child before its report of a live one
	// means anything.
	if out, _ := runUnderBash(t, probe); strings.Contains(out, "survived") {
		t.Fatalf("the probe cannot tell the two states apart: a surviving child with SIGUSR2 at its default: %q", out)
	}

	signal.Ignore(syscall.SIGUSR2)
	t.Cleanup(func() { signal.Notify(dispositionDrain, syscall.SIGUSR2) })

	out, _ := runUnderBash(t, "trap - USR2\n"+probe)
	if !strings.Contains(out, "survived") {
		t.Errorf("`trap - USR2` took away an ignore the shell was handed, and its child lost it too: %q", out)
	}
}

// dispositionDrain is this file's own undo for an ignore it installed, and it
// is a drain for the measured reason interp's own is: signal.Reset does not
// undo signal.Ignore, and os/signal never blocks on delivery, so a full
// channel drops the arrival rather than holding it.
var dispositionDrain = make(chan os.Signal, 1)
