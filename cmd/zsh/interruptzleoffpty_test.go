// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// **With the line editor off, a ^C at the prompt gives up the line it was
// typed on** (#5890), and does not wait to kill the next command.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`, `unsetopt zle`: `true`, `abc`, ^C, an empty line,
// then `$?` twice.
//
//	          first `$?` line   second
//	zsh       130               0
//	this      (nothing)         130     — before the fix
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
// The signal is sent to this process by the test. The shell is running in it,
// on a terminal that is not this process's controlling one, so the ^C byte
// alone reaches the line discipline — which throws away `abc` and echoes `^C`
// exactly as it would for a person — and the kernel has no foreground group to
// send the signal to. What the kernel would send, the test sends.
//
// Only the first row listens for it. `trap ""` is SIG_IGN, and a listener of
// the test's own would take the ignore away again — os/signal's Notify is a
// sigaction — so that row would be measuring a shell whose ignore the test had
// undone. There the signal is sent into the ignore, which is what a person's
// ^C does.
func TestControlCWithTheEditorOffGivesUpTheLine(t *testing.T) {
	for _, tc := range []struct {
		name, rc string
		// heard is whether the signal reaches this process at all.
		heard bool
		// want is `$?` and pipestatus read by the line after the one typed
		// after the ^C, and then by the line after that.
		want, then string
	}{
		{"no trap", "", true, "130-0 1", "0-0"},
		{"trap \"\" INT", "trap '' INT\n", false, "1-0 1", "0-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionRC(t, tc.rc, "zsh", "-i", "+Z")
			interruptType(t, control, screen, "true | false")
			var heard chan os.Signal
			if tc.heard {
				heard = make(chan os.Signal, 1)
				signal.Notify(heard, syscall.SIGINT)
				defer signal.Stop(heard)
			}
			from := len(screen.Text())
			if _, err := control.WriteString("abc\x03"); err != nil {
				t.Fatalf("typing abc and ^C: %v", err)
			}
			if err := screen.Await("^C", jobNoticeBudget); err != nil {
				t.Fatalf("the terminal did not echo the ^C: %v", err)
			}
			if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
				t.Fatalf("sending the interrupt: %v", err)
			}
			if tc.heard {
				select {
				case <-heard:
				case <-time.After(jobNoticeBudget):
					t.Fatal("the interrupt never arrived")
				}
			}
			// The line the read ends on. In the first row it is given up, so
			// what it would print is not printed either way it is spelled; the
			// assertion is on the lines after it.
			interruptType(t, control, screen, "")
			if got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine); got != tc.want {
				t.Errorf("after ^C the line after next read %q, want %q; drawn:\n%q", got, tc.want, screen.Text()[from:])
			}
			if got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine); got != tc.then {
				t.Errorf("and the line after it read %q, want %q", got, tc.then)
			}
		})
	}
}
