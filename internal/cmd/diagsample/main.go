// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command diagsample runs one bounded sample of error situations through a real
// shell and through ours, and reports how many came back byte-identical.
//
// It exists because the README used to open its list of gaps with "diagnostic
// wording is the biggest remaining gap", and the only probe anybody had for
// that claim was a full conformance run — which is an hour of the machine and
// so was never run to check it. A claim whose only instrument is too expensive
// to use is a claim that cannot go stale *visibly*, which is the worst kind.
//
// So this is deliberately a **sample and not a census**: twenty error
// situations a person actually hits, each run once, the whole panel in a few
// seconds. It cannot replace the corpus and is not meant to. What it can do is
// notice that a headline caveat has stopped being true.
//
// Two details are the measurement rather than the plumbing:
//
//   - **Both sides are started under the same `argv[0]`.** A shell puts its own
//     `$0` on the front of most diagnostics, so comparing `/bin/ksh` against
//     `build/probe-ksh` reports twenty differences that are all the same
//     difference and none of them about wording.
//
//   - **Standard error and the exit status are compared together.** Half of
//     what looks like a wording difference is a status difference wearing one:
//     in the sample below `ulimit -Z` and `kill -99` each differ in the words
//     *and* in the status, which makes them behavior.
//
// Usage:
//
//	go run ./internal/cmd/diagsample -real /bin/ksh -ours build/probe-ksh -name ksh
//
// The `ash` column's reference is BusyBox, which is not installed on a Mac.
// -image runs the reference inside a container the same way `internal/oracle`
// reaches that column, and our own binary still runs here:
//
//	go run ./internal/cmd/diagsample -image alpine@sha256:… -ours build/probe-ash -name ash
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The sample. Ordinary mistakes rather than interesting ones: the point is what
// a person meets in an afternoon, not what a fuzzer finds.
var cases = []string{
	`nosuchcommand_xyz`,
	`cd /nosuchdir_xyz`,
	`set -u; echo $nosuchvar_xyz`,
	`echo hi < /nosuchfile_xyz`,
	`echo $((1/0))`,
	`echo ${nosuchvar_xyz?is unset}`,
	`unset -Z foo`,
	`eval "echo )"`,
	`read -Z x < /dev/null`,
	`kill -99 $$`,
	`exec /nosuchprog_xyz`,
	`ulimit -Z`,
	`shift 99`,
	`trap -Z INT`,
	`$WORK/adir`,
	`printf "%z" x`,
	`. /nosuchfile_xyz`,
	`exit foo`,
	`echo hi > /nosuchdir_xyz/out`,
	`cd $WORK/plain`,
}

// A process id is in one of the answers and changes every run, so it is not a
// difference. Four digits and up, deliberately: an exit status is at most three,
// and a looser pattern normalizes `127` into the same token and hides a status
// difference inside a thing that claims to be hiding noise.
var pids = regexp.MustCompile(`\b[0-9]{4,}\b`)

const work = "/tmp/diagsample"

func main() {
	real := flag.String("real", "", "the reference shell on this machine")
	image := flag.String("image", "", "run the reference inside this container image instead, at /bin/<name>")
	ours := flag.String("ours", "", "our dialect binary")
	name := flag.String("name", "", "the dialect, and the argv[0] both sides are given")
	flag.Parse()
	if *ours == "" || *name == "" || (*real == "" && *image == "") {
		fmt.Fprintln(os.Stderr, "diagsample: -ours, -name and one of -real/-image are required")
		os.Exit(2)
	}
	if err := sample(*real, *image, *ours, *name); err != nil {
		fmt.Fprintln(os.Stderr, "diagsample:", err)
		os.Exit(1)
	}
}

func sample(real, image, ours, name string) error {
	path, err := filepath.Abs(ours)
	if err != nil {
		return err
	}
	if err := scratch(); err != nil {
		return err
	}
	var same int
	for i, c := range cases {
		want, err := reference(real, image, name, c)
		if err != nil {
			return err
		}
		got := normalize(runHere(path, name, c), path, name)
		if want == got {
			same++
			continue
		}
		fmt.Printf("  differs %2d  %s\n    reference: %s\n    ours:      %s\n",
			i+1, strings.ReplaceAll(c, "$WORK", work), oneLine(want), oneLine(got))
	}
	fmt.Printf("%s: %d/%d byte-identical on standard error and exit status\n", name, same, len(cases))
	return nil
}

// scratch makes the two files two of the cases need: a directory to try to run,
// and a plain file to try to cd into.
func scratch() error {
	if err := os.MkdirAll(filepath.Join(work, "adir"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(work, "plain"), nil, 0o644)
}

// reference runs one case through the real shell, here or in a container.
func reference(real, image, name, c string) (string, error) {
	if image == "" {
		return normalize(runAs(real, name, c), real, name), nil
	}
	script := "mkdir -p " + work + "/adir; : > " + work + "/plain; " +
		"/bin/" + name + " -c " + single(strings.ReplaceAll(c, "$WORK", work)) + " 2>&1; echo \"[status $?]\""
	cmd := exec.Command("docker", "run", "--rm", image, "/bin/sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil && !errors.As(err, new(*exec.ExitError)) {
		return "", fmt.Errorf("reaching %s: %w", image, err)
	}
	return normalize(string(out), "/bin/"+name, name), nil
}

func runHere(path, name, c string) string { return runAs(path, name, c) }

// runAs runs one case, giving the shell the argv[0] both sides share.
func runAs(bin, name, c string) string {
	cmd := exec.Command(bin, "-c", strings.ReplaceAll(c, "$WORK", work))
	cmd.Args = []string{name, "-c", strings.ReplaceAll(c, "$WORK", work)}
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, _ := cmd.CombinedOutput()
	return string(out) + fmt.Sprintf("[status %d]", cmd.ProcessState.ExitCode())
}

// normalize removes the two things that are not the shell's answer: where its
// binary happened to be found, and this run's process id.
func normalize(s, bin, name string) string {
	// The container path ends its answer with a newline and the local one does
	// not, and a trailing byte nobody wrote is not a wording difference.
	s = strings.TrimRight(s, "\n")
	s = strings.ReplaceAll(s, bin, name)
	s = strings.ReplaceAll(s, filepath.Base(bin), name)
	return pids.ReplaceAllString(s, "PID")
}

func oneLine(s string) string { return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", " | ") }

// single quotes a case for a /bin/sh -c inside a container.
func single(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
