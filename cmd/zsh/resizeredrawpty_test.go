// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package main

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// A window resize draws the prompt and the line again at once, with the
// right prompt against the new edge.
//
// #5908: nothing was written until the next key. Measured 2026-10-04 against
// zsh 5.9.2 through a pseudo-terminal with `PS1=$'up\n> '`, `RPS1=TIME` and
// `ab` typed, resizing from 40 columns to 50:
//
//	\r\r\e[A…\e[Jup\r\n> ab\e[K\e[41CTIME\e[45D
//
// The session runs in this process, so the signal is sent here: the kernel
// sends SIGWINCH to the terminal's foreground group, and this process is not
// in one.
func TestAResizeDrawsTheLineAgainAtOnce(t *testing.T) {
	control, screen, _ := jobNoticeSessionRC(t, "RPS1=TIME\n", "zsh", "-i")
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	if _, err := control.WriteString("ab"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		if strings.HasPrefix(g.Text(g.Rows()-1), jobNoticeMark+"ab") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ab was not drawn:\n%s", g)
		}
	}
	// The line at 100 columns, then the window made 60 wide: `TIME` ends one
	// short of the new edge, so it starts at column 55 and the move to it
	// from the end of `jn> ab` is 49.
	settled := screen.Text()
	if err := pty.SetSize(control, 24, 60); err != nil {
		t.Skipf("resizing the terminal: %v", err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	want := "JNROW\r\n\r\x1b[J" + jobNoticeMark + "ab\x1b[49CTIME"
	for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
		if after := strings.TrimPrefix(screen.Text(), settled); strings.Contains(after, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after the resize the shell wrote %q, want it to contain %q",
				smoke.Readable(strings.TrimPrefix(screen.Text(), settled)), smoke.Readable(want))
		}
	}
}
