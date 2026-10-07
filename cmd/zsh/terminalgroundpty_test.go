// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
	"github.com/blairham/sh/internal/tty"
)

// What a fresh prompt writes is read from the terminal's description: the
// unfinished-output mark in whatever attributes the terminal has, padded one
// column short of the edge without `xn`; the resets and the erase before the
// prompt and the erase after it where it has them, each delay written as NULs
// at the terminal's speed; and bracketed paste asked for after the prompt.
// A terminal called `emacs` has no line editor, and its prompt is the mark
// and the prompt alone.
//
// Measured 2026-10-07 on zsh 5.9.2 through a pseudo-terminal at 80 columns,
// `zsh -f -i`, `PS1='P> '`, the bytes before the first key (#6315):
//
//	dumb   %, 78 spaces, \r \r\r, P> , \e[?2004h
//	emacs  %, 78 spaces, \r \r, P>
//	vt100  \e[1m … \e[0m, 79 spaces, \r \r\r\e[0m\e[m\e[m\e[J, P> , \e[K,
//	       \e[?2004h — every delay as NULs: 2 for `$<2>`, 53 for `$<50>`
//	       at 9600 bits per second
//
// Here every terminal got xterm's bytes, and `emacs` the line editor.
func TestTheGroundUnderAPromptIsTheTerminals(t *testing.T) {
	nul := func(ms, speed int) string { return strings.Repeat("\x00", ms*speed/9000) }
	pad := func(n int) string { return strings.Repeat(" ", n) }
	for _, c := range []struct {
		term string
		want func(speed int) string
	}{
		{"dumb", func(int) string { return "%" + pad(78) + "\r \r\rP> \x1b[?2004h" }},
		{"emacs", func(int) string { return "%" + pad(78) + "\r \rP> " }},
		{"vt100", func(s int) string {
			return "\x1b[1m" + nul(2, s) + "\x1b[7m" + nul(2, s) + "%\x1b[m" + nul(2, s) +
				"\x1b[1m" + nul(2, s) + "\x1b[0m" + pad(79) + "\r \r\r" +
				"\x1b[0m\x1b[m" + nul(2, s) + "\x1b[m" + nul(2, s) + "\x1b[J" + nul(50, s) +
				"P> \x1b[K" + nul(3, s) + "\x1b[?2004h"
		}},
	} {
		t.Run(c.term, func(t *testing.T) {
			screen, _, speed := firstPrompt(t, c.term)
			want := c.want(speed)
			// Until what was written is what is wanted, or longer: a prompt
			// is several writes, and the last of them may still be coming.
			deadline := time.Now().Add(20 * time.Second)
			for drawn := screen.Text(); drawn != want; drawn = screen.Text() {
				if len(drawn) > len(want) || time.Now().After(deadline) {
					t.Fatalf("at %d bits per second the first prompt wrote\n%q\nwant\n%q", speed, drawn, want)
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

// firstPrompt starts `zsh -f -i` on a pseudo-terminal of 80 columns under
// term and answers what it is writing, and the terminal's speed.
func firstPrompt(t *testing.T, term string) (*smoke.Screen, *os.File, int) {
	t.Helper()
	home := scratchHome(t)
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 80); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	sh := scratchShell(t)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=" + term, "PS1=P> ",
		"HISTFILE=" + filepath.Join(home, "hist"),
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"zsh", "-f", "-i"}) }()
	t.Cleanup(func() {
		_, _ = control.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	return screen, control, tty.OutputSpeed(terminal)
}

// And an accepted line takes the bracketed-paste request back before the
// newline that ends it. Measured 2026-10-07, zsh 5.9.2 under xterm and dumb
// alike: `true` then Return writes `\e[?2004l\r\r\n` (#6315).
func TestAnAcceptedLineTakesThePasteRequestBackFirst(t *testing.T) {
	screen, control, _ := firstPrompt(t, "dumb")
	if err := screen.Await("P> \x1b[?2004h", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := control.WriteString("true\r"); err != nil {
		t.Fatal(err)
	}
	if err := screen.Await("true\x1b[?2004l\r\r\n", 20*time.Second); err != nil {
		t.Error(err)
	}
}

// And it is any assignment of `emacs` to `$TERM` that takes the editor away,
// not only the one the shell started with; assigning something else does not
// give it back. Measured 2026-10-07 on zsh 5.9.2 through a pseudo-terminal:
// `TERM=emacs`, `TERM=emacs true` and a function's `local TERM=emacs` each
// leave `[[ -o zle ]]` false, and `TERM=xterm` after them leaves it so.
func TestAssigningEmacsToTERMTurnsTheEditorOff(t *testing.T) {
	for _, set := range []string{"TERM=emacs", "TERM=emacs true", "f() { local TERM=emacs; }; f"} {
		t.Run(set, func(t *testing.T) {
			out, _, _ := prompt(t, "[[ -o zle ]] && echo before-on\n"+set+"\nTERM=xterm\n[[ -o zle ]] || echo after-off\n", "zsh", "-f", "-i")
			if !strings.Contains(out, "before-on") || !strings.Contains(out, "after-off") {
				t.Errorf("stdout %q, want the editor on before and off after", out)
			}
		})
	}
}
