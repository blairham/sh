// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// How deep a start's stack goes, which is a cost of its own.
//
// A goroutine's stack starts small and doubles when a call needs more, and
// every doubling is a fresh allocation the kernel has to fault in page by
// page before the old stack is copied into it. On macOS a fault is tens of
// thousands of instructions, so a start that reaches a megabyte of stack pays
// for every page of every size on the way there.
//
// The front end passed its Shell — 17 KB, most of it the dialect's
// diagnostics table — by value through every step of a start, beside a
// source carrying another 10 KB copy of that table. Each frame held its own copies,
// and some held several: `startup` held four, one in each method value of its
// loop over the startup files. A zsh script file's start reached 1 MB of stack
// that way and bash's 512 KB, which was 1.7M instructions and 95 page faults
// of the 40M a one-line zsh script cost (#6491).
//
// These run one start in a copy of this test binary whose stacks may not grow
// past stackCeiling; a start that needs more dies with the runtime's "stack
// overflow" rather than passing. Measured on darwin/arm64 by lowering the
// ceiling a power of two at a time until a row failed, since a stack only
// grows by doubling: every row below needs 32 KB today and fails at 16 KB;
// before #6610 every one needed 128 KB, and before #6491 512 KB. The ceiling
// is one doubling above today's, so that another architecture's frames or the
// race detector's do not fail it, and two below what the Diagnostics table's
// value-receiver methods cost. Those held a 10 KB copy of the table on the
// frame of every function that called one, on an error path or not — 153
// frames of 20 KB or more in interp and driver — and a start's deepest path
// ran through several of them. The methods take the table by pointer now, and
// TestADiagnosticsMethodWritesOnlyACopy in interp keeps them from writing it.
const stackCeiling = 64 << 10

// stackHelperShell names the shell a re-executed copy of this binary starts,
// and stackHelperCeiling the ceiling it starts it under.
const (
	stackHelperShell   = "STARTUPCOST_STACK_SHELL"
	stackHelperCeiling = "STARTUPCOST_STACK_CEILING"
)

// stackHelperDone is what the helper prints after the start returns, so that
// a helper that exited 0 without running anything does not read as a pass.
const stackHelperDone = "STACK-HELPER-DONE"

func TestAStartStaysOnASmallStack(t *testing.T) {
	if name := os.Getenv(stackHelperShell); name != "" {
		runStackHelper(t, name)
		return
	}
	dir := t.TempDir()
	scripts := map[string]string{"zsh": "print -r -- ONE\n", "bash": "echo ONE\n"}
	for shell, text := range scripts {
		if err := os.WriteFile(filepath.Join(dir, shell+".script"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The positive control: a program that recurses two hundred functions deep
	// needs far more than the ceiling, so the helper must die of it. A ceiling
	// the helper never applied, or applied to some goroutine other than the
	// one the shell runs on, would pass every row below.
	deep := "f() { [ \"$1\" -gt 0 ] && f $(($1 - 1)); }; f 200; echo ONE"
	if out, err := startUnderCeiling(
		t,
		dir,
		"zsh",
		stackCeiling,
		"-c",
		deep,
	); err == nil ||
		!strings.Contains(out, "stack overflow") {
		t.Fatalf(
			"a recursion 200 deep fit under the %d KB ceiling (err %v), so the rows below would check nothing:\n%s",
			stackCeiling>>10,
			err,
			out,
		)
	}
	for _, c := range []struct {
		shell string
		args  []string
	}{
		// zsh reads its unconditional startup file even for a script, which
		// is the route that went deepest.
		{"zsh", []string{filepath.Join(dir, "zsh.script")}},
		{"zsh", []string{"-c", "print -r -- ONE"}},
		{"bash", []string{filepath.Join(dir, "bash.script")}},
		{"bash", []string{"-c", "echo ONE"}},
	} {
		out, err := startUnderCeiling(t, dir, c.shell, stackCeiling, c.args...)
		if err != nil {
			t.Errorf("%s %q under a %d KB stack ceiling: %v\n%s", c.shell, c.args, stackCeiling>>10, err, out)
			continue
		}
		if !strings.Contains(out, "ONE\n") || !strings.Contains(out, stackHelperDone) {
			t.Errorf("%s %q: the start did not run its program:\n%s", c.shell, c.args, out)
		}
	}
}

// startUnderCeiling re-executes this test binary to start one shell with its
// stacks capped at ceiling, and returns what it wrote.
func startUnderCeiling(t *testing.T, dir, shell string, ceiling int, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAStartStaysOnASmallStack$")
	cmd.Args = append(cmd.Args, append([]string{"--"}, args...)...)
	cmd.Env = testenv.WithoutRaceExitPause(append(os.Environ(),
		stackHelperShell+"="+shell,
		stackHelperCeiling+"="+strconv.Itoa(ceiling),
		"HOME="+dir, "ZDOTDIR="+dir, "ENV=", "BASH_ENV=",
	))
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// runStackHelper is the re-executed side: one start, by the entry point a
// binary's main uses, with the ceiling in force.
//
// The system startup directory is the test's scratch directory, which holds
// no startup files: a machine's own `/etc/zsh/zshenv` would make the depth
// depend on what that machine's administrator wrote.
func runStackHelper(t *testing.T, name string) {
	ceiling, err := strconv.Atoi(os.Getenv(stackHelperCeiling))
	if err != nil {
		t.Fatal(err)
	}
	var sh driver.Shell
	switch name {
	case "zsh":
		sh = gateZshShell()
	case "bash":
		sh = gateBashShell()
	default:
		t.Fatalf("no shell %q", name)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sh.SystemStartupDirectory = wd
	sh.Stdout, sh.Stderr = os.Stdout, os.Stderr
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	debug.SetMaxStack(ceiling)
	// On a goroutine of its own, so the start begins at the runtime's
	// starting stack size rather than on whatever the test's goroutine has
	// already grown to: a stack only grows, and a start that fit in one that
	// was already large would never be asked to grow past the ceiling.
	done := make(chan int)
	go func() { done <- driver.MainArgs(sh, append([]string{name}, args...)) }()
	status := <-done
	fmt.Println(stackHelperDone)
	os.Exit(status)
}
