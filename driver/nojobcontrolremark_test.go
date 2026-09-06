// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// remarking is the diagnostics of a dialect that says something when an
// interactive shell cannot have job control, worded as bash words it.
//
// Measured 2026-09-05 with no terminal on any of the three standard streams,
// scratch HOME and scratch HISTFILE: `bash: no job control in this shell` in
// bash 5.3.15 and 3.2.57, and `sh: no job control in this shell` in bash 3.2
// run as `sh` — the shell's own name, and its own name on `-i script.sh` as
// well, never the script's.
func remarking() interp.Diagnostics {
	return interp.Diagnostics{NoJobControlAtStartup: "no job control in this shell"}
}

// noTerminalScript runs `-i script` with nothing but files on the three
// standard streams and returns what reached the error stream.
//
// The whole point of the case is that there is no terminal, so the streams are
// real files rather than a pseudo-terminal — which is also what makes the
// assertion safe: nothing the shell writes can go anywhere this test cannot
// read, which is the trap #793 fell into from the other direction.
func noTerminalScript(
	t *testing.T, dg interp.Diagnostics, sem interp.Semantics, argv ...string,
) (errs string, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.sh")
	if err := os.WriteFile(path, []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := shell()
	sh.Semantics, sh.Diagnostics = sem, dg
	sh.Stdin = openFile(t, os.DevNull)
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	args := []string{"testsh"}
	for _, a := range argv {
		args = append(args, strings.ReplaceAll(a, "@SCRIPT@", path))
	}
	if code := driver.MainArgs(sh, args); code != 0 {
		t.Fatalf("status %d, want 0 (stderr %q)", code, e.String())
	}
	if o.String() != "ran\n" && len(argv) > 0 && argv[len(argv)-1] == "@SCRIPT@" {
		t.Fatalf("output %q, want %q — the script did not run", o.String(), "ran\n")
	}
	return e.String(), path
}

// An interactive shell that wanted the monitor and has no terminal to run one
// on says so, in the dialects that say anything.
//
// The whole line, name and all. Measured on `-i script.sh` with every stream
// redirected: bash writes `bash: no job control in this shell` and dash writes
// `<script>: 0: can't access tty; job control turned off`.
func TestAnInteractiveShellWithNoTerminalSaysItHasNoJobControl(t *testing.T) {
	errs, _ := noTerminalScript(t, remarking(), interp.PosixSemantics(), "-i", "@SCRIPT@")
	if want := "testsh: no job control in this shell\n"; errs != want {
		t.Errorf("wrote %q, want %q", errs, want)
	}
}

// And it names itself rather than the script it was handed — unless the
// dialect is the one that names the script.
//
// dash's shape, and dash alone: `-i script.sh` writes the script's path and
// `-i -c` writes dash's own, which is `$0` on both. bash writes its own name
// on both.
func TestWhoTheRemarkNames(t *testing.T) {
	t.Run("its own name, which is bash's", func(t *testing.T) {
		errs, _ := noTerminalScript(t, remarking(), interp.PosixSemantics(), "-i", "@SCRIPT@")
		if want := "testsh: no job control in this shell\n"; errs != want {
			t.Errorf("wrote %q, want %q", errs, want)
		}
	})
	t.Run("or the script, which is dash's", func(t *testing.T) {
		dg := remarking()
		dg.NoJobControlAtStartupNamesTheScript = true
		errs, path := noTerminalScript(t, dg, interp.PosixSemantics(), "-i", "@SCRIPT@")
		if want := path + ": no job control in this shell\n"; errs != want {
			t.Errorf("wrote %q, want %q", errs, want)
		}
	})
	t.Run("and off the script route the two are the same name", func(t *testing.T) {
		dg := remarking()
		dg.NoJobControlAtStartupNamesTheScript = true
		errs, _ := noTerminalScript(t, dg, interp.PosixSemantics(), "-i", "-c", "echo ran")
		if want := "testsh: no job control in this shell\n"; errs != want {
			t.Errorf("wrote %q, want %q", errs, want)
		}
	})
}

// It is said on every interactive route and on none of the others, which is
// measured: the same line comes out of `-i script.sh`, `-i -c` and `-i -s`,
// and out of a plain script and a plain `-c` never.
func TestWhichRoutesRemark(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"a script operand", []string{"-i", "@SCRIPT@"}, "testsh: no job control in this shell\n"},
		{"a command string", []string{"-i", "-c", "echo ran"}, "testsh: no job control in this shell\n"},
		{"and a script that is not interactive says nothing", []string{"@SCRIPT@"}, ""},
		{"nor does a plain command string", []string{"-c", "echo ran"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs, _ := noTerminalScript(t, remarking(), interp.PosixSemantics(), tc.argv...)
			if errs != tc.want {
				t.Errorf("wrote %q, want %q", errs, tc.want)
			}
		})
	}
}

// A prompt reached with no terminal remarks too, which is the route `-i` with
// nothing to run takes. Measured: bash and dash both write the same line
// before the first prompt, ahead of anything the startup files print.
func TestAPromptWithNoTerminalRemarksToo(t *testing.T) {
	sh := shell()
	sh.Diagnostics = remarking()
	sh.Semantics = interp.PosixSemantics()
	_, errs, _ := runPipedShell(t, sh, "echo ran\n", "testsh", "-i")
	// The first line whole, because everything after it is the prompts this
	// route draws — and *that* it is the first line is the assertion: the
	// panel writes this ahead of the first prompt and ahead of anything the
	// startup files print.
	first, rest, _ := strings.Cut(errs, "\n")
	if want := "testsh: no job control in this shell"; first != want {
		t.Errorf("the first line was %q, want %q (the rest %q)", first, want, rest)
	}
}

// With a terminal there is nothing to say, and the dialect that needs no
// terminal has nothing to say without one either.
func TestTheRemarkIsOnlyForAShellThatCouldNotHaveTheMonitor(t *testing.T) {
	t.Run("a terminal, so the monitor runs", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "probe.sh")
		if err := os.WriteFile(path, []byte("echo ran\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sh := shell()
		sh.Diagnostics = remarking()
		sh.Semantics = interp.PosixSemantics()
		_, tty := terminal(t)
		sh.Stdin = tty
		var o, e bytes.Buffer
		sh.Stdout, sh.Stderr = &o, &e
		if code := driver.MainArgs(sh, []string{"testsh", "-i", path}); code != 0 {
			t.Fatalf("status %d, want 0 (stderr %q)", code, e.String())
		}
		if e.String() != "" {
			t.Errorf("wrote %q, want nothing — this shell has its monitor", e.String())
		}
	})

	t.Run("no terminal, but the dialect needs none", func(t *testing.T) {
		sem := interp.PosixSemantics()
		sem.InteractiveMonitorNeedsATerminal = interp.No
		errs, _ := noTerminalScript(t, remarking(), sem, "-i", "@SCRIPT@")
		if errs != "" {
			t.Errorf("wrote %q, want nothing — ksh93's shape, the monitor runs anyway", errs)
		}
	})

	t.Run("no terminal and no monitor, but the dialect says nothing", func(t *testing.T) {
		errs, _ := noTerminalScript(t, interp.Diagnostics{}, interp.PosixSemantics(), "-i", "@SCRIPT@")
		if errs != "" {
			t.Errorf("wrote %q, want nothing — zsh's and ksh93's shape", errs)
		}
	})
}
