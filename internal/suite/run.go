// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// A suite file is not a script that happens to be shell. It starts shells,
// backgrounds them, stops them, puts them on pipes and waits for them, and a
// good fraction of what it is testing is what happens when one does not come
// back. So the two things that matter about running one are that it is given
// a directory of its own to ruin, and that when it does not finish, the thing
// killed is the whole tree and not the shell at the top of it.

// DefaultTimeout is how long one file gets.
//
// Generous on purpose, and the reason is in what a timeout *means* on each
// side. A dialect binary killed here is a published hang — a real finding,
// and the one a user meets. The reference killed here is a harness fault: the
// shell that wrote the file does not hang on it, so a kill says the machine
// was loaded or the bound was too tight. Those are different findings and
// conflating them would turn a busy laptop into a bug report. A bound loose
// enough that the oracle never reaches it keeps that distinction real.
const DefaultTimeout = 90 * time.Second

// Outcome is one file run under one shell.
type Outcome struct {
	Output string
	Status int
	// TimedOut says the process group was killed. It is not a status: a
	// killed run has no result, and scoring one would put a number on a file
	// nobody finished.
	TimedOut bool
	// Failed says the shell never started — the binary is missing, is not
	// executable, or the path given was wrong. It is kept apart from Status
	// for the reason TimedOut is: a shell that did not run disagreed with
	// nothing, and grading it would report a harness fault as a defect in
	// the shell. That is not hypothetical. Before [Shell] existed, a
	// relative -bin path was resolved against the per-run directory rather
	// than against the caller's, every run of every column failed to start,
	// and the report read "0/10 strict" for four dialects — a wrong answer
	// with no sign in it that nothing had been measured.
	Failed bool
}

// runFile runs one suite file under one shell, in dir, and returns what came
// out.
//
// stdout and stderr are one stream because the suite's own ordering between
// them is part of what is being compared: a file that prints a diagnostic
// between two results says something different if the diagnostic moves.
func runFile(ctx context.Context, s Suite, shell, dir, name string, env []string, timeout time.Duration) Outcome {
	cmd := exec.Command(shell, argv(s, name)...)
	cmd.Dir = dir
	cmd.Env = env
	// Left nil on purpose: nil is the empty input, so a file that reads gets
	// an immediate end rather than waiting for a person who is not there.
	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	// A backstop only. The group kill below is what actually ends a run; this
	// bounds the wait if a process escaped the group.
	//
	// Generous, for the reason the timeout above is generous and with a
	// sharper edge. A suite file *deliberately* leaves background jobs
	// running when it ends — that is a thing about shells worth testing —
	// and every one of them holds the output pipe the harness is reading. A
	// short delay does not measure a shell that leaks: it measures how long
	// the file's own `sleep` was against a bound nobody chose on purpose,
	// and what it writes down is a -1, which reads as a run that died on a
	// signal.
	//
	// Measured 2026-09-13 on the file that drove this: it backgrounds jobs
	// of one, two and four seconds and ends once the two-second one is
	// waited out, so the orphan outlives the shell by two seconds. At two
	// this bound landed on the same edge for the reference as for us and
	// the file came back *unstable* — the oracle disagreeing with itself
	// between runs — which costs the file entirely. At fifteen both shells
	// finish and the file is evidence again.
	cmd.WaitDelay = 15 * time.Second
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return Outcome{Output: err.Error(), Status: -1, Failed: true}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	case err := <-done:
		return finish(err, buf.String())
	}

	// Kill the group rather than the process. A suite file's shell is not the
	// only thing running: it starts background jobs, coprocesses and nested
	// shells, and every one of them holds the output pipe open. Killing the
	// leader alone leaves the run un-reaped and the harness waiting on a
	// descendant nobody named.
	killGroup(cmd.Process)
	<-done
	return Outcome{Output: buf.String(), TimedOut: true}
}

// argv is what the shell under test is asked to run.
//
// A suite file is a script in its own dialect and the shell runs it
// directly — that is bash's column and it is our own. A suite with a
// [Suite.Driver] is one whose files are data the suite's own harness reads,
// so the shell runs *the harness* and the file is the harness's argument.
// The shell under test is still the only shell in the picture either way,
// which is what makes the second shape a table entry rather than a second
// instrument.
//
// Every path is relative to the run directory, which is the copy of the
// suite this run was given. The driver especially: `ztst.zsh` recomputes its
// own source directory from how it was invoked and ignores the environment
// variable the suite's Makefile sets, so a driver named by an absolute path
// would send it looking outside the copy.
func argv(s Suite, name string) []string {
	if s.Driver == "" {
		return []string{"./" + name}
	}
	out := make([]string, 0, len(s.DriverArgs)+2)
	out = append(out, s.DriverArgs...)
	return append(out, "./"+s.Driver, "./"+name)
}

func finish(err error, out string) Outcome {
	if err == nil {
		return Outcome{Output: out}
	}
	var ee *exec.ExitError
	if !asExitError(err, &ee) {
		return Outcome{Output: out, Status: -1}
	}
	return Outcome{Output: out, Status: ee.ExitCode()}
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// environ is what a run is given, and it is the same for both shells but for
// the one variable naming the shell itself.
//
// Cut down to what a shell needs to find its tools, so the answer does not
// depend on what happens to be exported on the machine today. The suite's
// directory leads PATH because the helpers were compiled into it and the
// files call them by name.
func environ(s Suite, dir, shell string) []string {
	tmp := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + dir + ":/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"TMPDIR=" + tmp,
		"PWD=" + dir,
		// C rather than the machine's locale: collation order and the
		// wording of a libc error are the machine's, and a run whose numbers
		// moved with LANG would be measuring the machine.
		"LC_ALL=C",
		"LANG=C",
		"TERM=dumb",
		"TZ=UTC",
	}
	if s.ShellVar != "" {
		// Pointed at the shell of *this* run, not at one shell for both. A
		// suite re-enters its own shell constantly, and a variable frozen to
		// the reference would have our column grading the reference against
		// itself — the whole run green, and nothing measured.
		env = append(env, s.ShellVar+"="+shell)
	}
	return env
}

// Believable applies [Suite.MustReport]: the path exists and answered, but is
// it this shell?
//
// The same question [oracle.Shell] asks of a panel member, and for the same
// recorded reason: /bin/sh is BusyBox on Alpine and dash on Debian, so a
// column that trusted a path recorded the wrong shell and nothing looked
// wrong. A column reached inside an image is where this matters most, since
// nobody is going to notice by eye what that path resolved to.
func (s Suite) Believable(b Build) error {
	if s.MustReport == "" {
		return nil
	}
	if !b.Known {
		// A shell that would not identify itself is not a shell this check
		// can clear, and before #3135 it could be: the probe handed back
		// whatever the refusal printed, and a refusal that happened to carry
		// the name — `Usage: ksh [ options ]` does — passed as an
		// identification.
		return fmt.Errorf("found, but it answers no version probe, so it cannot be confirmed "+
			"as the %q this column names", s.MustReport)
	}
	if strings.Contains(strings.ToLower(b.Version), s.MustReport) {
		return nil
	}
	return fmt.Errorf("found, but it reports %q rather than %q, so it is not the shell this column names",
		b.Version, s.MustReport)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// staticParse asks a shell to read a file without running any of it — the
// POSIX `-n` — and reports only whether it accepted.
//
// The verdict and never the diagnostic. A shell's complaint about a suite
// file quotes the file back, and this instrument may not print that; a
// verdict is a fact about a binary, which is what the whole oracle rests on.
//
// It is what tells a construct this parser cannot read from a construct no
// static read can reach. An option set at run time decides what a later line
// means — `shopt -s extglob` is the worked example — and a static read has no
// run time, so the reference refuses its own suite's file while running that
// same file to completion. No parser change moves such a file into the parsed
// column, and counting it as a gap in this parser would be counting a gap
// nobody can close.
func staticParse(ctx context.Context, shell, path string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-n", path)
	// None of the run's environment: `-n` executes nothing, so the only
	// thing an environment could decide here is where the shell looks for
	// what it is not going to run.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "LANG=C"}
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}

// staticTimeout bounds that read. Reading a file without running it is
// bounded by the file's size and nothing else, so this is short where the
// per-file run timeout is generous.
const staticTimeout = 15 * time.Second
