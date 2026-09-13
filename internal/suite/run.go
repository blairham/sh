// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"bytes"
	"context"
	"errors"
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
func runFile(ctx context.Context, shell, dir, name string, env []string, timeout time.Duration) Outcome {
	cmd := exec.Command(shell, "./"+name)
	cmd.Dir = dir
	cmd.Env = env
	// Left nil on purpose: nil is the empty input, so a file that reads gets
	// an immediate end rather than waiting for a person who is not there.
	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	// A backstop only. The group kill below is what actually ends a run; this
	// bounds the wait if a process escaped the group.
	cmd.WaitDelay = 2 * time.Second
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

// runVersion asks a shell for its build string, bounded like everything else
// here: a shell that will not answer this is not one to run a suite under.
//
// Two spellings, in order, because one shell on the panel answers neither of
// the usual ones. `--version` covers bash and zsh; BusyBox refuses it and
// prints its build on the first line of `--help`, which is the only place the
// string "BusyBox" appears at all. ksh and dash answer nothing here and stay
// unknown, which is what their rows have always said.
//
// The output is combined for the second probe: BusyBox writes its usage to
// standard error, and a version taken from an empty stream would make
// [Suite.Believable] refuse a shell that had in fact identified itself.
func runVersion(ctx context.Context, shell string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, shell, "--version").Output(); err == nil {
		return string(out), nil
	}
	out, err := exec.CommandContext(ctx, shell, "--help").CombinedOutput()
	if len(bytes.TrimSpace(out)) == 0 {
		if err == nil {
			err = errors.New("the shell answered neither --version nor --help")
		}
		return "", err
	}
	return string(out), nil
}

// Believable applies [Suite.MustReport]: the path exists and answered, but is
// it this shell?
//
// The same question [oracle.Shell] asks of a panel member, and for the same
// recorded reason: /bin/sh is BusyBox on Alpine and dash on Debian, so a
// column that trusted a path recorded the wrong shell and nothing looked
// wrong. A column reached inside an image is where this matters most, since
// nobody is going to notice by eye what that path resolved to.
func (s Suite) Believable(version string) error {
	if s.MustReport == "" || strings.Contains(strings.ToLower(version), s.MustReport) {
		return nil
	}
	return fmt.Errorf("found, but it reports %q rather than %q, so it is not the shell this column names",
		firstLine(version), s.MustReport)
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
