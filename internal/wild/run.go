// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// tempDirPattern matches the directory each run is given, whose name differs
// every time and would otherwise be a difference in itself.
var tempDirPattern = regexp.MustCompile(`(/private)?/(tmp|var/folders)/[^\s:"']*`)

// Running a script is the half the parse sweep cannot do, and it is where the
// last two gaps came from: `set -f` was invisible to a reader, and only turned
// up because /usr/bin/man parsed and then failed on it. Positional parameters
// were the same — the text was fine and the shell was not.
//
// It is opt-in, and the reason is worth stating plainly: this executes third
// party code. A `--help` is conventionally read-only and is not guaranteed to
// be, so the sweep runs each script under *both* shells with the same
// arguments, in a directory of its own, with no standard input and a timeout.
// Whatever a script does, it does the same twice, and the comparison is
// between two runs rather than between a run and an expectation.

// Probes are the arguments a script is tried with.
//
// Conventional and read-only by habit: a program asked for its version or its
// usage is not expected to do anything else. That is a convention rather than
// a guarantee, which is why this list is short and why the mode is opt-in.
var Probes = [][]string{
	{"--version"},
	{"--help"},
}

// RunResult is one script run one way under both shells.
type RunResult struct {
	Path string
	Args []string
	// Ours and Theirs are what each shell produced, already normalised.
	Ours, Theirs string
	// Status is each shell's exit status.
	OurStatus, TheirStatus int
}

// Match reports whether the two shells agreed.
func (r RunResult) Match() bool {
	return r.Ours == r.Theirs && r.OurStatus == r.TheirStatus
}

// RunReport is a whole run sweep.
type RunReport struct {
	Ran        int
	Agreed     int
	Timedout   int
	Mismatches []RunResult
}

// RunSweep runs every script that parses, under both shells, and reports where
// they disagree.
//
// ours and reference are shell binaries. Only scripts this parser accepts are
// run: one that cannot be read has already been reported by the parse sweep,
// and running it would only say the same thing again.
func RunSweep(ctx context.Context, paths []string, ours, reference string, timeout time.Duration) RunReport {
	var rep RunReport
	for _, path := range paths {
		for _, args := range Probes {
			a, aTimeout := runOnce(ctx, ours, path, args, timeout)
			b, bTimeout := runOnce(ctx, reference, path, args, timeout)
			if aTimeout || bTimeout {
				// A script that waits for something is not evidence about
				// either shell, and which of them hung is not the question.
				rep.Timedout++
				continue
			}
			rep.Ran++
			res := RunResult{
				Path: path, Args: args,
				Ours: normalise(a.out, ours, path), Theirs: normalise(b.out, reference, path),
				OurStatus: a.status, TheirStatus: b.status,
			}
			if res.Match() {
				rep.Agreed++
				continue
			}
			rep.Mismatches = append(rep.Mismatches, res)
		}
	}
	sort.Slice(rep.Mismatches, func(i, j int) bool {
		return rep.Mismatches[i].Path < rep.Mismatches[j].Path
	})
	return rep
}

type outcome struct {
	out    string
	status int
}

// runOnce runs one script under one shell, contained.
//
// A directory of its own, so a script that writes a file writes it somewhere
// that goes away. No standard input, so one that reads gets an immediate end
// rather than waiting for a person. A timeout, because some will wait anyway.
// And an environment cut down to what a shell needs to find its tools, so the
// answer does not depend on what happens to be exported today.
func runOnce(ctx context.Context, shell, script string, args []string, timeout time.Duration) (outcome, bool) {
	dir, err := os.MkdirTemp("", "wild")
	if err != nil {
		return outcome{}, false
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"LC_ALL=C",
		"TERM=dumb",
	}
	// Killing the shell is not enough to make Run return: it waits for the
	// output pipes to close, and a grandchild the script started still holds
	// them open. WaitDelay closes them shortly after the kill, which is what
	// makes the timeout a bound on the sweep rather than a suggestion — a
	// script that ran `sleep 30` otherwise took the full thirty seconds no
	// matter what the timeout said.
	cmd.WaitDelay = time.Second

	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	runErr := cmd.Run()

	if ctx.Err() != nil {
		return outcome{}, true
	}
	status := 0
	var ee *exec.ExitError
	if runErr != nil {
		if !asExitError(runErr, &ee) {
			return outcome{}, true
		}
		status = ee.ExitCode()
	}
	return outcome{out: buf.String(), status: status}, false
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

// normalise removes what is true of the machine rather than of the shell.
//
// The shell's own path appears in its diagnostics, and the two shells have
// different ones — comparing those would report a difference on every script
// that fails, which is most of the interesting ones.
func normalise(out, shell, script string) string {
	// The full path only. Replacing the base name as well was too eager: it
	// turned `GNU bashbug` into `GNU <shell>bug` and reported a difference
	// between two identical outputs.
	out = strings.ReplaceAll(out, shell, "<shell>")
	out = strings.ReplaceAll(out, script, "<script>")
	// A temporary directory's name is different every run.
	return tempDirPattern.ReplaceAllString(out, "<tmp>")
}
