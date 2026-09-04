// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A signal that ends a command is said out loud, because nothing else says
// it: the status carries the number and the output looks exactly as it would
// have if the command had simply finished.
//
// The words for the signal are fixed here rather than taken from the machine,
// so that what is being checked is the shape of the notice and not what this
// platform calls the signal.
//
// SIGUSR1 rather than a more typical death because its default action ends
// the process without dumping core. SIGABRT does dump one, on a machine where
// that is turned on — and a test run inside a Linux container left a 420K
// `core` in the package directory, which all but went into a commit.
func killedRun(t *testing.T, src string, dg Diagnostics, answer Answer) (string, int) {
	t.Helper()
	if dg.SignalDescriptions == nil {
		dg.SignalDescriptions = map[syscall.Signal]string{syscall.SIGUSR1: "Boom"}
	}
	if dg.Location == LocationNone {
		dg.Location = LocationTightLine
	}
	var errs strings.Builder
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = answer
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String(), r.ExitStatus()
}

// A command dying of a signal that is not one of the two ordinary ones.
const killedSrc = "true\n/bin/sh -c 'kill -USR1 $$'\n"

func TestACommandKilledBySignalIsReported(t *testing.T) {
	got, status := killedRun(t, killedSrc, Diagnostics{}, Yes)
	// The line, the process id, the words and the command, in that order.
	want := regexp.MustCompile(`^sh:2: +\d+ Boom +/bin/sh -c 'kill -USR1 \$\$'\n$`)
	if !want.MatchString(got) {
		t.Errorf("notice = %q, want it to match %v", got, want)
	}
	if status != 128+int(syscall.SIGUSR1) {
		t.Errorf("status = %d, want %d", status, 128+int(syscall.SIGUSR1))
	}
}

// The dialect that says nothing at all. Measured with a terminal as well as
// without one, so this is not the prompt-only rule a background job follows.
func TestAShellMayReportNothingAboutASignal(t *testing.T) {
	got, status := killedRun(t, killedSrc, Diagnostics{}, No)
	if got != "" {
		t.Errorf("said %q, want nothing", got)
	}
	if status != 128+int(syscall.SIGUSR1) {
		t.Errorf("status = %d, want %d", status, 128+int(syscall.SIGUSR1))
	}
}

// Refusing to say something is not refusing to run: the status a signal made
// is not the wording question's to change.
func TestRefusingToReportLeavesTheStatusAlone(t *testing.T) {
	got, status := killedRun(t, killedSrc, Diagnostics{}, Unspecified)
	if !strings.Contains(got, "killed by a signal") {
		t.Errorf("refusal = %q, want it to name the axis", got)
	}
	if status != 128+int(syscall.SIGUSR1) {
		t.Errorf("status = %d, want %d — the refusal must not become the status", status, 128+int(syscall.SIGUSR1))
	}
}

// The two nothing remarks on. Both are how a command is *meant* to end — ^C
// is a person stopping something, and a broken pipe is `yes | head` ending
// the thing writing into it — and a shell that announced them would be
// announcing them constantly.
func TestTheTwoOrdinaryDeathsAreNotReported(t *testing.T) {
	for _, c := range []struct{ name, sig string }{
		{"an interrupt", "INT"},
		{"a broken pipe", "PIPE"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "true\n/bin/sh -c 'kill -" + c.sig + " $$'\n"
			if got, _ := killedRun(t, src, Diagnostics{}, Yes); got != "" {
				t.Errorf("said %q, want nothing", got)
			}
		})
	}
}

// One dialect prints the words alone: no process id, no command, and — alone
// among everything it says — no name and no line in front of them.
func TestTheNoticeCanBeWordedWithoutALocation(t *testing.T) {
	dg := Diagnostics{
		KilledCommandNotice:           "%[2]s",
		KilledCommandNoticeUnprefixed: true,
	}
	if got, _ := killedRun(t, killedSrc, dg, Yes); got != "Boom\n" {
		t.Errorf("notice = %q, want %q", got, "Boom\n")
	}
}

// The command is written back out from the tree, not quoted from the source.
//
// Which is what makes this message answerable at all without keeping the text
// of every statement: runs of spaces collapse, a redirection gains the space
// its operator is printed with, and a comment is gone entirely.
func TestTheNoticeWritesTheCommandBackOut(t *testing.T) {
	src := "true\n   /bin/sh    -c   'kill -USR1 $$'   >/dev/null   # note\n"
	got, _ := killedRun(t, src, Diagnostics{}, Yes)
	const want = `/bin/sh -c 'kill -USR1 $$' > /dev/null`
	if !strings.Contains(got, want) {
		t.Errorf("notice = %q, want it to contain %q", got, want)
	}
	if strings.Contains(got, "note") {
		t.Errorf("notice = %q, want the comment gone", got)
	}
}

// A signal the shell has no words of its own for falls back to the machine's,
// which is where the other two dialects get every one of theirs.
//
// Asserted by prefix because the words are the platform's: this machine
// writes the number after them and another does not.
func TestASignalWithNoWordsOfItsOwnTakesTheMachineWords(t *testing.T) {
	dg := Diagnostics{SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"}}
	got, _ := killedRun(t, "true\n/bin/sh -c 'kill -KILL $$'\n", dg, Yes)
	if !regexp.MustCompile(` Killed`).MatchString(got) {
		t.Errorf("notice = %q, want the machine's words for KILL, capitalized", got)
	}
}

// The other path a child is reaped by.
//
// A shell that watches its own children — which is what job control needs,
// and the only kind of wait that can see a command that *stopped* — is told
// the signal directly rather than through an error. It has to say the same
// thing, and the two paths are far enough apart in the runner to be worth a
// test that only one of them can satisfy.
func TestTheNoticeAlsoComesFromAWatchedWait(t *testing.T) {
	var errs strings.Builder
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = Yes
	sem.SignalDeathStatusIsTwoFiftySix = No
	dg := Diagnostics{
		Location:           LocationTightLine,
		SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
	}
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
		WaitForCommand: func(int) (Wait, error) {
			return Wait{Killed: true, Signal: syscall.SIGUSR1}, nil
		},
	}
	f, err := syntax.Parse("/usr/bin/true\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "Boom") {
		t.Errorf("notice = %q, want the watched path to report it too", errs.String())
	}
	if !strings.Contains(errs.String(), "/usr/bin/true") {
		t.Errorf("notice = %q, want the command written back out", errs.String())
	}
}

// The words a machine uses are the machine's, and one of the two this is
// tested on writes the number after them.
//
// Named for the platform rather than for a shell, because that is what it
// varies with: every shell that takes its words from the host prints the
// number here and none of them prints it there.
func TestTheMachineWordsCarryTheNumberWhereTheMachineDoes(t *testing.T) {
	got, _ := killedRun(t, "true\n/bin/sh -c 'kill -KILL $$'\n",
		Diagnostics{SignalDescriptions: map[syscall.Signal]string{}}, Yes)
	// Two-sided on purpose. Asserting only that the words are there passes
	// on both machines whichever the code does, because "Killed: 9" contains
	// "Killed" — so the number's *absence* is what has to be checked where
	// it should be absent.
	numbered := strings.Contains(got, "Killed: 9")
	if !strings.Contains(got, "Killed") {
		t.Fatalf("notice = %q, want the machine's words for KILL", got)
	}
	if want := runtime.GOOS == "darwin"; numbered != want {
		t.Errorf("notice = %q: number after the words = %v, want %v on %s",
			got, numbered, want, runtime.GOOS)
	}
}

// An element of a pipeline whose status the pipeline does not take.
//
// Three of the panel say nothing about one — `sh -c 'kill …' | cat` is
// silent in bash and ksh93, and the same command as the *last* element is
// not. dash says the same thing wherever the element stands.
func TestASignalEndingAPipelineElementThatIsNotTheLast(t *testing.T) {
	const src = "true\n/bin/sh -c 'kill -USR1 $$' | cat\necho after\n"
	for _, c := range []struct {
		name   string
		any    Answer
		spoken bool
	}{
		{"passed over", No, false},
		{"remarked on wherever it stands", Yes, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var errs strings.Builder
			sem := PosixSemantics()
			sem.ReportsACommandKilledBySignal = Yes
			sem.ReportsAnyKilledPipelineElement = c.any
			sem.LastPipelineElementInCurrentShell = No
			dg := Diagnostics{
				Location:           LocationTightLine,
				SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
			}
			out := &strings.Builder{}
			r := &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh",
				Stdout: out, Stderr: &errs,
			}
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if spoken := strings.Contains(errs.String(), "Boom"); spoken != c.spoken {
				t.Errorf("said %q; remarked on = %v, want %v", errs.String(), spoken, c.spoken)
			}
			// Either way the pipeline runs to the end and the script goes on.
			if !strings.Contains(out.String(), "after") {
				t.Errorf("stdout = %q, want the script to carry on", out.String())
			}
		})
	}
}

// The last element is the one whose status the pipeline takes, so it is
// remarked on whatever the answer above is — which is what makes the two
// halves of the question distinguishable.
func TestTheLastPipelineElementIsAlwaysRemarkedOn(t *testing.T) {
	for _, any := range []Answer{Yes, No} {
		var errs strings.Builder
		sem := PosixSemantics()
		sem.ReportsACommandKilledBySignal = Yes
		sem.ReportsAnyKilledPipelineElement = any
		sem.LastPipelineElementInCurrentShell = No
		dg := Diagnostics{
			Location:           LocationTightLine,
			SignalDescriptions: map[syscall.Signal]string{syscall.SIGUSR1: "Boom"},
		}
		r := &Runner{
			Semantics: &sem, Diagnostics: &dg, Name: "sh",
			Stdout: &strings.Builder{}, Stderr: &errs,
		}
		f, err := syntax.Parse("true\necho hi | /bin/sh -c 'kill -USR1 $$'\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errs.String(), "Boom") {
			t.Errorf("any=%v: said %q, want the last element remarked on", any, errs.String())
		}
	}
}

// Refusing to *say* something about a mid-pipeline element must not change
// what that element's status was.
//
// Visible through `pipefail`, which is the one place a mid-pipeline status
// reaches the result at all: a shell with no answer recorded still has to
// report the signal's status rather than the refusal's.
func TestRefusingToRemarkOnAPipelineElementLeavesItsStatusAlone(t *testing.T) {
	var errs strings.Builder
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = Yes
	// Deliberately unrecorded: this is the refusal path.
	sem.ReportsAnyKilledPipelineElement = Unspecified
	sem.LastPipelineElementInCurrentShell = No
	sem.PipefailOption = Yes
	sem.SignalDeathStatusIsTwoFiftySix = No
	out := &strings.Builder{}
	r := &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{Location: LocationTightLine},
		Name: "sh", Stdout: out, Stderr: &errs,
	}
	f, err := syntax.Parse("set -o pipefail\n/bin/sh -c 'kill -USR1 $$' | cat\necho \"st=$?\"\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	want := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1))
	if !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q — the refusal must not become the element's status", out.String(), want)
	}
}
