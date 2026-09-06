// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// bgScript names the script a re-executed copy of this test binary should run
// as a shell, the way dyingScript does for the signal tests. Set, it means
// "you are the shell"; unset, "you are the test".
const bgScript = "SH_TEST_BACKGROUND_SCRIPT"

// bgRunningAsAShell turns this process into a shell when it was re-executed as
// one, and does nothing otherwise. Called from TestMain, because os.Exit
// inside a running test is a panic in Go 1.26.
//
// A second process is not caution here, it is the only way to ask the
// question. What is under test is what survives *this process ending*, and a
// test that ran the shell in-process would still be running when it looked.
func bgRunningAsAShell() {
	src := os.Getenv(bgScript)
	if src == "" {
		return
	}
	os.Exit(driver.MainArgs(shell(), []string{"testsh", "-c", src}))
}

// What a `&` job leaves behind after the shell has gone, pinned as it is.
//
// **This test records a known difference, and a change that fixes it will make
// it fail.** That is deliberate and it is the point: #519 decided this rather
// than fixing it, and a decision nothing checks is one that gets reversed by
// accident in either direction. Whoever makes a job a real process should come
// here, see the assertion turn over, and change the words.
// docs/spec/semantics.md carries the reasoning and the panel measurement.
//
// Measured 2026-09-06: every shell in the panel — bash 5.3.15, bash 3.2.57,
// dash, ksh93u+, zsh 5.9.2 — keeps every one of these, because a real shell
// forks and the job is a process that outlives its parent by construction.
// Here a job is a goroutine on a cloned Runner in this process, so what is left
// of it when the process ends is what ends with it.
//
// # Why the job waits for a file rather than sleeping
//
// The first version gave each job a `sleep 0.3` and looked for its output
// afterwards, and it was wrong: under `-race` the shell binary took longer than
// that to start and exit, so the job *finished before the shell did* and the
// difference this is about never arose. It passed on its own and failed under
// `make check`, which is the good way round to find it.
//
// So the job waits for a file the test creates only **after** the shell has
// exited. It cannot finish early however slow the machine is, because what it
// is waiting for does not exist yet. What is left is a bounded wait for the
// file the job would write, and the two directions cost differently: where the
// job survives the wait ends the moment the file appears, and where it does not
// the wait is the whole budget and is the price of a definite negative.
//
// A named pipe would remove even that, and cannot be used: a job blocking on
// one never lets `&` return in this shell at all (#1003), so the test would
// hang before reaching the question it is asking. The polling loop is written
// with `sleep`, which is an external command, and that is what keeps this
// clear of #1003 — it is also why the loop cannot be a bare `read`.
func TestWhatABackgroundJobLeavesBehind(t *testing.T) {
	for _, tc := range []struct {
		name     string
		script   string
		survives bool
		why      string
	}{
		{
			name:     "one external command",
			script:   `sh -c 'until [ -f GATE ]; do sleep 0.05; done; echo LATE > OUT' &`,
			survives: true,
			why:      "the whole job is a child process, so outliving the shell is what it already does",
		},
		{
			name:     "a subshell",
			script:   `(until [ -f GATE ]; do sleep 0.05; done; echo LATE > OUT) &`,
			survives: false,
			why:      "the loop and the echo after it are this process's own work",
		},
		{
			name:     "an and-list",
			script:   `sh -c 'until [ -f GATE ]; do sleep 0.05; done' && echo LATE > OUT &`,
			survives: false,
			why:      "the waiting half is a child and survives; the echo after it is this shell's and does not",
		},
		{
			name:     "a function",
			script:   `f() { until [ -f GATE ]; do sleep 0.05; done; echo LATE > OUT; }; f &`,
			survives: false,
			why:      "a function body is the shell's own work throughout",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			gate, out := filepath.Join(dir, "gate"), filepath.Join(dir, "late")
			script := strings.NewReplacer("GATE", gate, "OUT", out).Replace(tc.script)
			runShellForBackground(t, script)

			// Only now, with the shell gone, is the job let go. Anything that
			// arrives after this arrived from a job that outlived it.
			if err := os.WriteFile(gate, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			// Generous where arrival is expected, because the wait ends the
			// moment the file appears; short where loss is, because there is
			// nothing to return early for and a machine slow enough to need
			// more would have failed the arriving cases first.
			budget := 2 * time.Second
			if tc.survives {
				budget = 20 * time.Second
			}
			if arrived := waitForFile(out, budget); arrived != tc.survives {
				t.Errorf("the job's later half %s, want it to have %s — %s",
					arrivedWord(arrived), survivesWord(tc.survives), tc.why)
			}
		})
	}
}

// runShellForBackground re-executes this test binary as a shell running src,
// and returns once that shell has exited.
func runShellForBackground(t *testing.T, src string) {
	t.Helper()
	// No -test.run: the shell half is taken in TestMain before any test is
	// selected, so naming one would only decide what the process would have
	// done had it not become a shell.
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), bgScript+"="+src)
	// A file and not a pipe: os/exec waits for a pipe to close, and a
	// surviving job holding the shell's streams keeps one open — so the wait
	// would be for the job rather than for the shell, which is the one thing
	// this must not do. A file is never held open against the reader.
	log, err := os.CreateTemp(t.TempDir(), "shell-half-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		said, _ := os.ReadFile(log.Name())
		t.Fatalf("the shell half: %v; it said:\n%s", err, said)
	}
}

// waitForFile reports whether the path appears within the budget.
func waitForFile(path string, budget time.Duration) bool {
	for deadline := time.Now().Add(budget); time.Now().Before(deadline); {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func arrivedWord(b bool) string {
	if b {
		return "arrived"
	}
	return "was lost"
}

func survivesWord(b bool) string {
	if b {
		return "arrived"
	}
	return "been lost"
}
