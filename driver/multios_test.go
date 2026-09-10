// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// multiosScript names the script for the half of a test that has to *be* the
// shell, and carries the one axis this file is about.
const multiosScript = "SH_TEST_MULTIOS_SCRIPT"

// beTheMultiosShell is beTheShell with the dialect that writes to every target
// of a repeated redirection, which is the only place the question arises.
func beTheMultiosShell() {
	if path := os.Getenv(multiosScript); path != "" {
		sh := shell()
		sh.Semantics.RedirectsUseEveryTarget = interp.Yes
		os.Exit(driver.MainArgs(sh, []string{"testsh", path}))
	}
}

// runMultios re-executes this test binary as such a shell, in a directory of
// its own, and hands back that directory so the *files* can be read.
//
// Reading the files is the point. A replacement leaves no shell behind to
// print anything, and the bytes are the whole of what a stream with two
// targets is about, so an assertion on output or status would pass with the
// text in one file, in the other, or in neither.
func runMultios(t *testing.T, name, src string) (dir, out string) {
	t.Helper()
	dir = t.TempDir()
	script := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), multiosScript+"="+script)
	cmd.Dir = dir
	got, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("the shell half could not be started: %v\n%s", err, got)
	}
	return dir, string(got)
}

// read returns a file's contents, and says so rather than failing when the
// file is not there: "no file" and "an empty file" are different answers here
// and a test that could not tell them apart would be worth less.
func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

// A stream with two targets reaches a process replacement.
//
// It could not: the replacement is handed descriptor *numbers*, a writer over
// two files is no number, and reading that as "not a file, therefore nil,
// therefore closed" left the command with standard output closed —
// `echo: fflush: Bad file descriptor`, status 1, and neither file written.
// The shell declines the replacement for such a stream now and stands in for
// it with the child route, which copies the bytes to both.
func TestAStreamWithTwoTargetsReachesAReplacement(t *testing.T) {
	beTheMultiosShell()
	dir, out := runMultios(t, "TestAStreamWithTwoTargetsReachesAReplacement",
		"exec >a >b\nexec /bin/echo hi\n")
	if strings.Contains(out, "Bad file descriptor") {
		t.Errorf("the replacement found its stream closed: %q", out)
	}
	for _, name := range []string{"a", "b"} {
		if got := read(t, dir, name); !strings.Contains(got, "hi") {
			t.Errorf("%s = %q, want the replacement's line", name, got)
		}
	}
}

// The rule it must not disturb: a stream closed *on purpose* is still a
// closed number, which is what every shell in the panel gives the command.
// Both are "standard output is not a file"; only one of them is a stream.
func TestAClosedStreamStillCrossesAsClosed(t *testing.T) {
	beTheMultiosShell()
	_, out := runMultios(t, "TestAClosedStreamStillCrossesAsClosed",
		"exec >&-\nexec /bin/echo hi\n")
	if !strings.Contains(out, "Bad file descriptor") {
		t.Errorf("out = %q, want the command to have found its stream closed", out)
	}
}

// And an ordinary redirection is still a real replacement rather than a child
// standing in for one, which is the thing the decline must not spread to. The
// pid is how they are told apart: a replacement keeps the shell's and a child
// gets its own.
func TestAnOrdinaryRedirectionStillReplacesTheProcess(t *testing.T) {
	beTheMultiosShell()
	dir, _ := runMultios(t, "TestAnOrdinaryRedirectionStillReplacesTheProcess",
		"echo \"shell=$$\" > p\nexec >a\nexec /bin/sh -c 'echo child=$$'\n")
	shellPid := strings.TrimSpace(strings.TrimPrefix(read(t, dir, "p"), "shell="))
	childPid := strings.TrimSpace(strings.TrimPrefix(read(t, dir, "a"), "child="))
	if shellPid == "" || shellPid == "<missing>" || childPid == "" {
		t.Fatalf("nothing to compare: shell %q child %q", shellPid, childPid)
	}
	if shellPid != childPid {
		t.Errorf("shell %s child %s, want one process — the replacement was declined",
			shellPid, childPid)
	}
}
