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
		Name: "sh", Stdout: out, Stderr: &errs, Env: testPATH(),
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

// An interrupt that ended a child ends the script in one dialect.
//
// SIGINT alone — measured across QUIT, TERM, HUP, USR1 and PIPE, every one
// of which every shell carries on from — and it ends the whole script rather
// than the construct around it.
//
// Driven through the caller's own wait rather than by sending a real signal.
// A child that kills itself with SIGINT is not reliable inside this test
// binary: `trap ” INT` elsewhere in the package calls signal.Ignore for the
// whole process, a child inherits that, and then it does not die at all —
// which is the hazard trap_test.go already writes down. Delivery is covered
// by the corpus, which runs the built shell as its own process.
func TestAnInterruptThatEndedAChild(t *testing.T) {
	for _, c := range []struct {
		name    string
		ends    Answer
		src     string
		carried bool
		status  int
	}{
		{"carried on from", No, "/usr/bin/true\necho after\n", true, 0},
		{"or the script ends there", Yes, "/usr/bin/true\necho after\n", false, 130},
		{
			// Not the loop, the script: what comes after the loop is
			// abandoned too.
			"and from inside a loop it is still the script", Yes,
			"for i in 1 2 3; do\n/usr/bin/true\necho loop\ndone\necho after\n", false, 130,
		},
		{
			"where the loop otherwise runs through", No,
			"for i in 1 2 3; do\n/usr/bin/true\necho loop\ndone\necho after\n", true, 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := interruptRun(t, c.src, c.ends, syscall.SIGINT, nil)
			if carried := strings.Contains(out, "after"); carried != c.carried {
				t.Errorf("out = %q; carried on = %v, want %v", out, carried, c.carried)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Only an interrupt. Every other signal is carried on from by every shell,
// so the dialect that stops has nothing to say about them.
func TestOnlyAnInterruptEndsTheScript(t *testing.T) {
	for _, sig := range []syscall.Signal{
		syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGPIPE,
	} {
		out, st := interruptRun(t, "/usr/bin/true\necho after\n", Yes, sig, nil)
		if !strings.Contains(out, "after") {
			t.Errorf("%v: out = %q, want the script to carry on", sig, out)
		}
		if st != 0 {
			t.Errorf("%v: status = %d, want 0", sig, st)
		}
	}
}

// A shell with no answer refuses rather than guessing, and the refusal does
// not become the status — the same rule the other two refusals here follow.
func TestAnInterruptWithNoAnswerRecorded(t *testing.T) {
	out, _ := interruptRun(t, "/usr/bin/true\necho \"st=$?\"\n", Unspecified, syscall.SIGINT, nil)
	if !strings.Contains(out, "interrupt") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
	if !strings.Contains(out, "st=130") {
		t.Errorf("said %q, want the signal's status rather than the refusal's", out)
	}
}

// The shell does not exit with 130 — it dies of the interrupt itself, so a
// parent sees a process a signal ended rather than one that exited.
//
// The golden record is what said so: it recorded an exit code of -1 for the
// dialect that does this, which is what os/exec reports for a process killed
// by a signal. An exit with 130 would have recorded 130.
func TestAnInterruptEndsTheShellBySignalRatherThanByExiting(t *testing.T) {
	var got syscall.Signal
	var asked bool
	out, st := interruptRun(t, "/usr/bin/true\necho after\n", Yes, syscall.SIGINT,
		func(sig syscall.Signal) error {
			got, asked = sig, true
			return nil
		})
	if !asked {
		t.Fatalf("the shell exited instead of dying by the signal (out %q)", out)
	}
	if got != syscall.SIGINT {
		t.Errorf("died by %v, want SIGINT", got)
	}
	// And the status is still set, because the driver may not get the chance
	// before the kernel does.
	if st != 128+int(syscall.SIGINT) {
		t.Errorf("status = %d, want %d", st, 128+int(syscall.SIGINT))
	}
}

func interruptRun(t *testing.T, src string, ends Answer, sig syscall.Signal, die func(syscall.Signal) error) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ChildInterruptEndsTheScript = ends
	sem.ReportsACommandKilledBySignal = Yes
	sem.SignalDeathStatusIsTwoFiftySix = No
	r := &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{Location: LocationTightLine},
		Name: "sh", Stdout: &buf, Stderr: &buf,
		// The caller's own wait, which is told the signal directly. No real
		// one is sent, so nothing here depends on this process's signal
		// dispositions.
		WaitForCommand: func(int) (Wait, error) {
			return Wait{Killed: true, Signal: sig}, nil
		},
		DieBySignal: die,
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}

// One signal in one shell is written with neither the location nor the
// process id — the words and the command alone.
//
// Reproduced rather than endorsed: bash 3.2 writes the full prefix for
// SIGTERM as it does for every other signal, and no other shell in the panel
// treats it apart, so this looks like a regression. The dialect is bash 5.3,
// and that is what bash 5.3 does.
func TestOneSignalMayBeWrittenBare(t *testing.T) {
	dg := func() Diagnostics {
		return Diagnostics{
			Location:                            LocationTightLine,
			KilledCommandNotice:                 "%5[1]d %-27[2]s%[3]s",
			KilledCommandNoticeBareForTerminate: "%-27[1]s%[2]s",
			SignalDescriptions: map[syscall.Signal]string{
				syscall.SIGTERM: "Boom",
				syscall.SIGUSR1: "Bang",
			},
		}
	}
	for _, c := range []struct {
		name  string
		sig   syscall.Signal
		bare  bool
		words string
	}{
		{"the one that is", syscall.SIGTERM, true, "Boom"},
		{"and one that is not", syscall.SIGUSR1, false, "Bang"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := killedWatched(t, dg(), c.sig)
			if !strings.Contains(got, c.words) {
				t.Fatalf("said %q, want the words for the signal", got)
			}
			// The location and the process id go together: either the notice
			// carries both or it carries neither.
			if located := strings.Contains(got, "sh:"); located == c.bare {
				t.Errorf("said %q; located = %v, want %v", got, located, !c.bare)
			}
			if bare := !strings.HasPrefix(got, "sh:"); bare != c.bare {
				t.Errorf("said %q; bare = %v, want %v", got, bare, c.bare)
			}
		})
	}
	// And with no answer recorded the ordinary notice stands, which is what
	// the other three dialects want.
	d := dg()
	d.KilledCommandNoticeBareForTerminate = ""
	if got := killedWatched(t, d, syscall.SIGTERM); !strings.HasPrefix(got, "sh:") {
		t.Errorf("said %q, want the ordinary notice without an answer here", got)
	}
}

// killedWatched reports a command ended by sig, through the caller's own wait
// so that no real signal is involved.
func killedWatched(t *testing.T, dg Diagnostics, sig syscall.Signal) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	sem.ReportsACommandKilledBySignal = Yes
	sem.ChildInterruptEndsTheScript = No
	sem.SignalDeathStatusIsTwoFiftySix = No
	r := &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
		WaitForCommand: func(int) (Wait, error) {
			return Wait{Killed: true, Signal: sig}, nil
		},
	}
	f, err := syntax.Parse("/usr/bin/true\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}
