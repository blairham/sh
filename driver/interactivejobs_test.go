// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// jobProbe starts a background job, records its pid where the test can read
// it, and waits for it.
//
// `wait` is the whole of the synchronization and there is no sleep to lose a
// race with: after it the job has ended, so both notices are owed. Which
// *line* the finishing notice follows is left undecided on purpose — the job
// may well have ended before `wait` was reached — and it does not have to be
// decided, because neither `echo` nor `wait` writes to the error stream. The
// transcript this test asserts on is the same either way.
//
// The pid goes to a file rather than to the output, so the expected
// announcement can be built with the number actually in it and the whole
// rendered line asserted rather than a fragment of one.
//
// And it is read *after* the wait, which is deliberate: by then the job has
// ended and its notice has been written, so this also pins that `$!` outlives
// both. It did not — the notice dropped the job and `$!` read the job — and
// the first run of this test on a machine fast enough to finish `sleep 0`
// before the next line is what found it.
const jobProbe = `sleep 0 &
wait
echo "$!" > %s
echo end
`

// interactiveJobs runs `-i script` with a terminal where the case asks for one
// and returns the error stream, the output stream, and the pid the job got.
//
// The terminal goes on standard *input* and the streams asserted on are
// buffers. That is the arrangement #793 had to learn: a probe whose answer
// goes to the terminal it just installed leaves the test reading nothing and
// passing whichever way the shell decided.
func interactiveJobs(
	t *testing.T, sem interp.Semantics, withTerminal bool, argv ...string,
) (errs, out, pid string) {
	t.Helper()
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "pid")
	src := fmt.Sprintf(jobProbe, pidPath)
	path := filepath.Join(dir, "probe.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	sh := shell()
	sh.Semantics = sem
	if withTerminal {
		_, tty := terminal(t)
		sh.Stdin = tty
	} else {
		sh.Stdin = openFile(t, os.DevNull)
	}
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	// `@SCRIPT@` is the probe's path and `@SOURCE@` its text, so a case can
	// hand the same program to `-i script` and to `-i -c` and compare the
	// two routes rather than two programs.
	args := []string{"testsh"}
	for _, a := range argv {
		a = strings.ReplaceAll(a, "@SCRIPT@", path)
		args = append(args, strings.ReplaceAll(a, "@SOURCE@", src))
	}
	if code := driver.MainArgs(sh, args); code != 0 {
		t.Fatalf("status %d, want 0 (stderr %q)", code, e.String())
	}
	b, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	return e.String(), o.String(), strings.TrimSpace(string(b))
}

// announcing is the semantics vector of a dialect that says both things about
// a job it runs from a named script: ksh93's and zsh's answers.
//
// FinishedJobNoticeNeedsAPrompt is part of those answers and is set here
// rather than left at the preset's, which says the opposite. Both shells write
// the `Done` row on a route that draws no prompt, so a vector standing in for
// them has to say so; the preset claims less because bash is the one shell
// that waits for a prompt and a core made of what the panel agrees on cannot
// write a row bash would not.
func announcing() interp.Semantics {
	sem := interp.PosixSemantics()
	sem.AnnouncesBackgroundJob = interp.Yes
	sem.InteractiveScriptAnnouncesJobs = interp.Yes
	sem.FinishedJobNoticeNeedsAPrompt = interp.No
	return sem
}

// An interactive shell running a named script has somebody to tell about its
// jobs, where the dialect says so.
//
// Measured 2026-09-05 through a pseudo-terminal with a scratch HOME and a
// scratch HISTFILE, on `sh -i script.sh`: ksh93u+ writes `[1]\t<pid>` and then
// `[1] +  Done  sleep 0.3 &`, and zsh 5.9.2 writes `[1] <pid>` and then
// `[1]  + done  sleep 0.3`. Both to the error stream, and both between the
// commands rather than at the end.
func TestAnInteractiveScriptAnnouncesItsJobs(t *testing.T) {
	errs, out, pid := interactiveJobs(t, announcing(), true, "-i", "@SCRIPT@")
	want := fmt.Sprintf("[1] %s\n[1]+  Done                    sleep 0\n", pid)
	if errs != want {
		t.Errorf("wrote %q to the error stream, want %q", errs, want)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q — the script did not run", out, "end\n")
	}
}

// And the dialect that announces the end and never the beginning gets exactly
// that, which is why the start is a second axis and not this one.
//
// dash's shape: measured on the same invocation it writes
// `[1] + Done                       sleep 0.3` and nothing at all as the job
// starts.
func TestAnInteractiveScriptMayAnnounceOnlyTheEnd(t *testing.T) {
	sem := announcing()
	sem.AnnouncesBackgroundJob = interp.No
	errs, _, _ := interactiveJobs(t, sem, true, "-i", "@SCRIPT@")
	want := "[1]+  Done                    sleep 0\n"
	if errs != want {
		t.Errorf("wrote %q to the error stream, want %q", errs, want)
	}
}

// The dialect that stays quiet stays quiet, and an unanswered axis is quiet
// too — decided once at startup, so refusing there would land ahead of every
// `-i script.sh` under a preset that has not chosen.
//
// bash's shape: all three members of the panel — 5.3.15, 3.2.57 and 3.2 run as
// `sh` — write neither line on this route, with a terminal and without one.
func TestAnInteractiveScriptMayAnnounceNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer interp.Answer
		preset bool
	}{
		{name: "a dialect that says nothing", answer: interp.No},
		{name: "and an unanswered axis is quiet", answer: interp.Unspecified},
		// The preset's own answer, read from the preset rather than written
		// into the case: XCU says nothing about a notice on this route, so
		// the base claims less and stays silent — which is the intersection
		// as well, the panel being quiet here only if bash is.
		{name: "and so is the preset, which is asked rather than told", preset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := announcing()
			if tc.preset {
				sem = interp.PosixSemantics()
				sem.AnnouncesBackgroundJob = interp.Yes
			} else {
				sem.InteractiveScriptAnnouncesJobs = tc.answer
			}
			errs, out, _ := interactiveJobs(t, sem, true, "-i", "@SCRIPT@")
			if errs != "" {
				t.Errorf("wrote %q to the error stream, want nothing", errs)
			}
			if out != "end\n" {
				t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
			}
		})
	}
}

// The notice rides on the monitor, which is measured rather than chosen: with
// no terminal anywhere, dash and zsh leave the monitor off and say nothing
// about the job either, while ksh93 runs the monitor without one and still
// announces both ends.
//
// So the same vector says both things depending only on whether the dialect
// needed a terminal for the monitor it was going to run.
func TestTheNoticeRidesOnTheMonitor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		needs interp.Answer
		quiet bool
	}{
		{"a dialect that needed a terminal has no monitor and says nothing", interp.Yes, true},
		{"and the one that needs none runs it and announces anyway", interp.No, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := announcing()
			sem.InteractiveMonitorNeedsATerminal = tc.needs
			errs, _, pid := interactiveJobs(t, sem, false, "-i", "@SCRIPT@")
			want := fmt.Sprintf("[1] %s\n[1]+  Done                    sleep 0\n", pid)
			if tc.quiet {
				want = ""
			}
			if errs != want {
				t.Errorf("wrote %q to the error stream, want %q", errs, want)
			}
		})
	}
}

// A script that is not interactive is told nothing, whatever the axis says.
// Unanimous: no shell in the panel announces a job to `sh script.sh`, with a
// terminal or without one.
func TestAScriptThatIsNotInteractiveIsToldNothing(t *testing.T) {
	errs, out, _ := interactiveJobs(t, announcing(), true, "@SCRIPT@")
	if errs != "" {
		t.Errorf("wrote %q to the error stream, want nothing", errs)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
	}
}

// The route is the whole of what the first axis names, and `-i -c` is not it:
// the same program, the same vector, the same terminal, and nothing is said
// until the *second* axis is answered.
//
// Measured, and it is the reason the axis names the route rather than the
// terminal: `bash -i -c` announces a job starting while `bash -i script.sh`
// announces nothing, so a shell that read one axis on both routes would be
// wrong about one of them. The columns are opposites — bash alone is quiet on
// a named script, dash and ash alone are quiet here — so neither predicts the
// other.
func TestACommandStringReadsItsOwnAxisAndNotTheScriptOne(t *testing.T) {
	// The script route's answer is Yes throughout, which is what makes this a
	// test of the second field rather than of silence: everything below holds
	// with the first axis saying announce.
	errs, out, _ := interactiveJobs(t, announcing(), true, "-i", "-c", "@SOURCE@")
	if errs != "" {
		t.Errorf("wrote %q to the error stream, want nothing — `-i script.sh` is not `-i -c`", errs)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
	}
}

// And with its own axis answered it announces, on the route the first axis
// does not name.
//
// ksh93's and zsh's shape: measured 2026-09-12 through a pseudo-terminal, on
// `-i -c` running a job held open on a fifo the string releases and then reaps
// with `wait`. ksh93 writes `[1]\t<pid>` and then `[1] +  Done  { … } &`; zsh
// writes `[1] <pid>` and then `[1]  + done  { … }`. No sleep in the probe, so
// neither line is racing the scheduler.
func TestAnInteractiveCommandStringAnnouncesItsJobs(t *testing.T) {
	sem := announcing()
	sem.InteractiveCommandStringAnnouncesJobs = interp.Yes
	errs, out, pid := interactiveJobs(t, sem, true, "-i", "-c", "@SOURCE@")
	want := fmt.Sprintf("[1] %s\n[1]+  Done                    sleep 0\n", pid)
	if errs != want {
		t.Errorf("wrote %q to the error stream, want %q", errs, want)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q — the string did not run", out, "end\n")
	}
}

// bash's shape, which is the whole reason the finished notice is a field of
// its own: the start and never the end.
//
// Measured 2026-09-12, bash 5.3.15 under `-i -c` writes `[1] <pid>` and no
// `Done` row — with `wait`, with `wait %1`, and with a whole second of `sleep`
// after the job had died. The same bash handed the same program on a pipe,
// where `-i` draws a prompt between the lines, writes `[1]+  Done` at the
// prompt after `wait`. So it is the prompt it is waiting for, not the route,
// and bash 3.2.57 writes the row on both — which is why "bash" needs a field
// here to have one answer at all.
func TestAFinishedJobNoticeMayWaitForAPromptThatNeverComes(t *testing.T) {
	sem := announcing()
	sem.InteractiveCommandStringAnnouncesJobs = interp.Yes
	sem.FinishedJobNoticeNeedsAPrompt = interp.Yes
	errs, out, pid := interactiveJobs(t, sem, true, "-i", "-c", "@SOURCE@")
	want := fmt.Sprintf("[1] %s\n", pid)
	if errs != want {
		t.Errorf("wrote %q to the error stream, want %q — the start and no `Done` row", errs, want)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
	}
}

// The same field on the other route, because it is a fact about the notice
// and not about how the program arrived: a dialect that waits for a prompt
// writes no `Done` row from a named script either.
//
// No shell in the panel is in this corner — bash is the only Yes and it is
// also the only shell that announces nothing at all on the script route — so
// this is the field being read where it is reachable rather than a column
// being reproduced. It is worth pinning because the alternative implementation
// is a check on the route, and a check on the route would pass every test
// above and answer this one wrongly.
func TestAScriptRouteObeysTheSameFinishedNoticeAxis(t *testing.T) {
	sem := announcing()
	sem.FinishedJobNoticeNeedsAPrompt = interp.Yes
	errs, _, pid := interactiveJobs(t, sem, true, "-i", "@SCRIPT@")
	want := fmt.Sprintf("[1] %s\n", pid)
	if errs != want {
		t.Errorf("wrote %q to the error stream, want %q", errs, want)
	}
}

// dash's and ash's shape, and the half of the grid that makes the new axis
// necessary rather than convenient: they announce on the script route and say
// nothing whatever here.
//
// Measured 2026-09-12 through a pseudo-terminal on `-i -c`, and the silence is
// a real answer rather than an absent one — `$-` is `mi` in dash and `cmi` in
// ash, so both are interactive with the monitor running and still write
// neither line.
func TestACommandStringMayAnnounceNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer interp.Answer
	}{
		{name: "a dialect that says nothing", answer: interp.No},
		// Decided once at startup, so an unanswered axis reads as the quiet
		// answer rather than refusing ahead of every `-i -c` under a preset
		// that has not chosen.
		{name: "and an unanswered axis is quiet", answer: interp.Unspecified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := announcing()
			sem.AnnouncesBackgroundJob = interp.No // dash and ash both
			sem.InteractiveCommandStringAnnouncesJobs = tc.answer
			errs, out, _ := interactiveJobs(t, sem, true, "-i", "-c", "@SOURCE@")
			if errs != "" {
				t.Errorf("wrote %q to the error stream, want nothing", errs)
			}
			if out != "end\n" {
				t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
			}
		})
	}
}

// And a command string that is not interactive is told nothing, whatever
// either axis says. Unanimous: no shell in the panel announces a job to a
// plain `-c`, with a terminal or without one.
func TestACommandStringThatIsNotInteractiveIsToldNothing(t *testing.T) {
	sem := announcing()
	sem.InteractiveCommandStringAnnouncesJobs = interp.Yes
	errs, out, _ := interactiveJobs(t, sem, true, "-c", "@SOURCE@")
	if errs != "" {
		t.Errorf("wrote %q to the error stream, want nothing", errs)
	}
	if out != "end\n" {
		t.Errorf("wrote %q to the output stream, want %q", out, "end\n")
	}
}
