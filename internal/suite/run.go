// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
		return Outcome{Output: err.Error(), Status: -1}
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
func runVersion(ctx context.Context, shell string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "--version")
	out, err := cmd.Output()
	return string(out), err
}
