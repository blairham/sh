// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// The notice arrives while the shell is **busy**, and not only while it is
// idle (#4531).
//
// #4524 served the rows a session can observe between keystrokes, which is the
// only place a front end has a descriptor in its hand. These are the two on
// the other side of that sentence: the shell is inside a wait of its own — for
// a foreground command, or for `wait` — and the reference still writes the
// notice in front of what that command was about to write.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — on a pseudo-terminal with `TERM=dumb`, `PS1`/`PS2` exported
// empty and the shell started `-fiV +Z`, and re-measured on 2026-09-26 beside
// this binary, where the two now agree byte for byte:
//
//	sleep 0.3 & sleep 1.5; print FGDONE  →  [1] 3692
//	                                        [1]  + done       sleep 0.3
//	                                        FGDONE            ← a second later
//	sleep 1.2 & / wait; print WAITED     →  [1]  + done       sleep 1.2
//	                                        WAITED
//
// **One line for the first row**, which is what makes it the row it claims to
// be: typed as two lines the background job finishes at an idle prompt and
// #4524's path serves it, and the case would pass for a shell with no seam in
// the foreground wait at all.
//
// The mark is found with LastIndex because the terminal echoes the line as it
// is typed, so the word appears once as input before anything has run.
func TestAFinishedJobIsReportedWhileAForegroundCommandRuns(t *testing.T) {
	control, screen := jobNoticeSession(t)
	before := screen.Text()
	jobNoticeType(t, control, screen, "sleep 0.3 & sleep 1.5; print FGDONE")

	got := screen.Text()[len(before):]
	notice := strings.Index(got, "[1]  + done       sleep 0.3")
	mark := strings.LastIndex(got, "FGDONE")
	if notice < 0 {
		t.Fatalf("no finished-job notice at all:\n%s", smoke.Readable(smoke.LastLines(got, 8)))
	}
	if notice > mark {
		t.Errorf("the notice came behind the foreground command's output:\n%s",
			smoke.Readable(smoke.LastLines(got, 8)))
	}
}

// And before a `wait` returns, which is the other row.
func TestAFinishedJobIsReportedBeforeAWaitReturns(t *testing.T) {
	control, screen := jobNoticeSession(t)
	jobNoticeType(t, control, screen, "sleep 1.2 &")
	before := screen.Text()
	jobNoticeType(t, control, screen, "wait; print WAITED")

	got := screen.Text()[len(before):]
	notice := strings.Index(got, "[1]  + done       sleep 1.2")
	mark := strings.LastIndex(got, "WAITED")
	if notice < 0 {
		t.Fatalf("no finished-job notice at all:\n%s", smoke.Readable(smoke.LastLines(got, 8)))
	}
	if notice > mark {
		t.Errorf("the notice came behind the `wait`:\n%s",
			smoke.Readable(smoke.LastLines(got, 8)))
	}
}
