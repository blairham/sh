// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// **With the line editor off, a ^C at the prompt gives up the line it was
// typed on** (#5890), and does not wait to kill the next command.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`, `unsetopt zle`: `true | false`, `abc`, ^C, an
// empty line, then `$?` and pipestatus twice.
//
//	          first line   second
//	zsh       130-0 1      0-0
//	this      (nothing)    130-130   — before the fix
//
// The terminal's own discipline turns ^C into SIGINT and goes on gathering, so
// the read ends on the next newline, and zsh gives that whole read up. This
// shell ran it, and the interrupt it had heard was still waiting for the
// command after it, which it killed without a word.
//
// Under `trap "" INT` the line after the ^C runs and `$?` is left alone. That
// row failed too before the fix, and only from a startup file: the session
// installs its own handler after the rc has run, and a handler is a sigaction,
// so it took the ignore away again and the ^C it then heard killed the next
// command. Measured against the same zsh with `trap "" INT` in `.zshrc`:
// `$?` reads 1 and pipestatus `0 1`, as before the ^C.
//
// And a ^D after the ^C ends the read that is being given up, not the
// session: zsh draws a fresh prompt and `$?` is 130.
//
// **The shell is a process of its own, in a session of its own with the
// terminal as its controlling one**, which is what makes the ^C byte a signal
// at all: the kernel sends SIGINT to the controlling terminal's foreground
// process group, and a shell running inside this test binary on a terminal
// that is not its controlling one is in no such group. Sending the signal to
// this process by hand instead is not the same thing — every shell this binary
// has run shares its dispositions, and one that ran an asynchronous list
// leaves the driver's fatal-signal watch subscribed to SIGINT, which then ends
// the whole test binary.
func TestControlCWithTheEditorOffGivesUpTheLine(t *testing.T) {
	for _, tc := range []struct {
		name, rc string
		// after is typed after the ^C, and ends the read it interrupted.
		after string
		// want is `$?` and pipestatus read by the line after the one typed
		// after the ^C, and then by the line after that.
		want, then string
	}{
		{"no trap", "", "\n", "130-0 1", "0-0"},
		{"no trap, then ^D", "", "\x04", "130-0 1", "0-0"},
		{"trap \"\" INT", "trap '' INT\n", "\n", "1-0 1", "0-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := zleOffChildSession(t, tc.rc)
			interruptType(t, control, screen, "true | false")
			from := len(screen.Text())
			if _, err := control.WriteString("abc\x03"); err != nil {
				t.Fatalf("typing abc and ^C: %v", err)
			}
			if err := screen.Await("^C", jobNoticeBudget); err != nil {
				t.Fatalf("the terminal did not echo the ^C: %v", err)
			}
			// What the read ends on. Where it is given up, what it would
			// print is not printed either way it is spelled; the assertion is
			// on the lines after it.
			if _, err := control.WriteString(tc.after); err != nil {
				t.Fatalf("typing %q: %v", tc.after, err)
			}
			for _, mark := range []string{"JNROW", jobNoticeMark} {
				if err := screen.Await(mark, jobNoticeBudget); err != nil {
					t.Fatalf("no prompt after %q: %v", tc.after, err)
				}
			}
			if got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine); got != tc.want {
				t.Errorf("after ^C the line after next read %q, want %q; drawn:\n%q", got, tc.want, screen.Text()[from:])
			}
			if got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine); got != tc.then {
				t.Errorf("and the line after it read %q, want %q", got, tc.then)
			}
		})
	}
}

// zleOffChildMode is the variable that turns this test binary into the shell
// TestZleOffChildShell is, and holds the startup directory it is to use in
// place of the machine's.
const zleOffChildMode = "SH_TEST_ZLE_OFF_CHILD"

// TestZleOffChildShell is **not a test**. It is the shell
// zleOffChildSession starts: this binary re-entered with the variable above
// set, so that it can be given a terminal as its controlling one. Run any
// other way it skips.
func TestZleOffChildShell(t *testing.T) {
	dir := os.Getenv(zleOffChildMode)
	if dir == "" {
		t.Skip("the child half of TestControlCWithTheEditorOffGivesUpTheLine")
	}
	sh := shell()
	sh.SystemStartupDirectory = dir
	os.Exit(driver.MainArgs(sh, []string{"zsh", "-i", "+Z"}))
}

// zleOffChildSession starts `zsh -i +Z` as a process of its own on a fresh
// pseudo-terminal, with rc after a prompt the interrupt helpers wait on, and
// waits for its first prompt.
func zleOffChildSession(t *testing.T, rc string) (*os.File, *smoke.Screen) {
	t.Helper()
	home := t.TempDir()
	writeHomeFile(t, home, ".zshrc", "PS1=$'JNROW\\n"+jobNoticeMark+"'\n"+rc)
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("finding this test binary: %v", err)
	}
	cmd := exec.Command(self, "-test.run=^TestZleOffChildShell$")
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
		// The child is a copy of a test binary, and this is what tells its
		// TestMain the environment was already built — see internal/testenv.
		"SH_TEST_HOME=" + home,
		zleOffChildMode + "=" + t.TempDir(),
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the shell: %v", err)
	}
	_ = terminal.Close()
	screen := smoke.Watch(control)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_, _ = control.WriteString("exit\nexit\n")
		select {
		case <-done:
		case <-time.After(jobNoticeBudget):
			_ = cmd.Process.Kill()
			<-done
			t.Error("the shell did not exit")
		}
		_ = control.Close()
	})
	if err := screen.Await(jobNoticeMark, jobNoticeBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	return control, screen
}
