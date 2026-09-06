// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// UnderTest names the shell being swept and how it is invoked.
//
// It is a struct rather than three parameters because the three go together:
// a shell that needs a flag to be the dialect it is being graded as, and a
// shell asked to contain itself, are both statements about *this* side of the
// comparison. The reference is a path and stays one — a real shell needs no
// flags to be itself and has no policy to be given.
type UnderTest struct {
	Path string
	// Args are the flags the binary needs before the script, so a substrate
	// binary can be swept as the dialect it is being compared against.
	Args []string
	// Contained asks for the shell to be handed a sandbox policy naming the
	// directory this run is given, and nothing else to write to.
	//
	// This is what turns the sweep's containment from a statement about the
	// *arrangement* into a statement about the *shell*. Today's containment
	// is that each script runs in a directory of its own with no standard
	// input and a timeout — all of it outside the shell, and none of it a
	// claim the shell could fail. With a policy on, a script that writes
	// outside its directory is refused by the thing under test, and every
	// script on the machine becomes a test of the boundary, in bulk, written
	// by people who did not know this implementation exists.
	//
	// Only the shell under test is contained. Handing the reference a flag it
	// does not have would end the sweep, and containing a real shell is not
	// possible in any case — which is the asymmetry the comparison has to
	// live with, and the reason a difference here is read as "the policy
	// refused something" rather than as "the shells disagree".
	Contained bool
}

// containmentPolicy is what a contained run is handed: write where you were
// put, and read and run as you like.
//
// The same posture the corpus runs under, and confined to writes for the same
// reason. Reads and execs cannot honestly be confined here — an allowed
// program is outside the boundary the instant it starts, and a `--help` that
// could not read its own message would tell us nothing about the shell.
func containmentPolicy(dir string) string {
	return "version 1\ndefault allow\ndefault deny write\nallow write " + dir + "/**\nallow write /dev/**\n"
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
	Ran      int
	Agreed   int
	Timedout int
	// Unstable counts scripts that do not produce the same thing twice, and
	// so are not evidence about either shell.
	Unstable   int
	Mismatches []RunResult
}

// RunSweep runs every script that parses, under both shells, and reports where
// they disagree.
//
// ours and reference are shell binaries. Only scripts this parser accepts are
// run: one that cannot be read has already been reported by the parse sweep,
// and running it would only say the same thing again.
func RunSweep(ctx context.Context, paths []string, ours UnderTest, reference string, timeout time.Duration) RunReport {
	var rep RunReport
	theirs := UnderTest{Path: reference}
	for _, path := range paths {
		for _, args := range Probes {
			a, aTimeout := runOnce(ctx, ours, path, args, timeout)
			b, bTimeout := runOnce(ctx, theirs, path, args, timeout)
			if aTimeout || bTimeout {
				// A script that waits for something is not evidence about
				// either shell, and which of them hung is not the question.
				rep.Timedout++
				continue
			}
			rep.Ran++
			res := RunResult{
				Path: path, Args: args,
				Ours: normalise(a.out, ours.Path, path), Theirs: normalise(b.out, reference, path),
				OurStatus: a.status, TheirStatus: b.status,
			}
			if res.Match() {
				rep.Agreed++
				continue
			}
			if !repeats(ctx, reference, path, args, timeout, res) {
				// The script is not the same twice, so the two shells were
				// never going to agree and this says nothing about either.
				//
				// A third of what this sweep reported was this: eleven
				// scripts whose output is a log line carrying a timestamp
				// and a process id, and six that are killed by the timeout
				// and named the process that died. Every one of them
				// disagreed with the reference for the same reason the
				// reference disagrees with itself.
				//
				// Counted rather than dropped, because "we cannot tell"
				// deserves its own number: a sweep that quietly ignored
				// these would be hiding the same thing in the other
				// direction.
				rep.Unstable++
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

// repeats reports whether the reference shell produces the same run twice.
//
// Asked only where the two shells differed, which is the only place the
// answer changes anything and keeps the cost to one extra run per difference
// rather than one per script.
func repeats(ctx context.Context, reference, path string, args []string, timeout time.Duration, res RunResult) bool {
	again, timedOut := runOnce(ctx, UnderTest{Path: reference}, path, args, timeout)
	if timedOut {
		return false
	}
	return normalise(again.out, reference, path) == res.Theirs && again.status == res.TheirStatus
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
func runOnce(ctx context.Context, shell UnderTest, script string, args []string, timeout time.Duration) (outcome, bool) {
	dir, err := os.MkdirTemp("", "wild")
	if err != nil {
		return outcome{}, false
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	argv := append([]string(nil), shell.Args...)
	if shell.Contained {
		// Written into the run's own directory, which the policy therefore
		// allows: a policy file somewhere else would be one more thing on the
		// machine for the sweep to leave behind, and one more path the rule
		// would have to name.
		p := filepath.Join(dir, "contained.policy")
		if err := os.WriteFile(p, []byte(containmentPolicy(dir)), 0o600); err != nil {
			return outcome{}, false
		}
		argv = append(argv, "-policy", p)
	}
	argv = append(argv, script)
	argv = append(argv, args...)

	cmd := exec.CommandContext(ctx, shell.Path, argv...)
	cmd.Dir = dir
	// Stdin is left nil on purpose and not assigned: nil *is* the empty
	// input, so a script that reads gets an immediate end rather than
	// waiting for a person. Assigning it would say the same thing twice.
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
	//
	// And "the full path only" has to be enforced rather than assumed. The
	// reference shell is named on the command line and defaults to `bash`, a
	// bare word — which put the base name straight back in, on one side of
	// the comparison only, because ours is always a path.
	out = replacePath(out, shell, "<shell>")
	out = replacePath(out, script, "<script>")
	// A temporary directory's name is different every run.
	return tempDirPattern.ReplaceAllString(out, "<tmp>")
}

// replacePath substitutes a file's path, and only a path.
//
// A name with no separator in it is a word that could turn up in any output
// for any reason, so it is left alone. Nothing is lost by that: a shell names
// itself in a diagnostic by the path it was invoked with, and the sweep now
// resolves the reference to one before it starts.
func replacePath(out, path, with string) string {
	if !strings.ContainsRune(path, filepath.Separator) {
		return out
	}
	return strings.ReplaceAll(out, path, with)
}
