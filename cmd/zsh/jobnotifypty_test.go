// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// jobNotifyQuiet is how long "the notice did not come" is given before it is
// believed.
//
// It is not a race being waited out. With `NOTIFY` off there is no code path
// that can write the row until another command has run, so the wait is slow
// rather than delicate — and the half after it, which types one command and
// watches the same row appear, is what says the instrument can fire at all.
const jobNotifyQuiet = 3 * time.Second

// `NOTIFY` decides **when** a finished job's notice is written: the moment the
// job ends, or held back until the shell is about to draw a prompt.
//
// The option is the noun and the two halves below are one session apiece so
// that it is the only thing that differs. Until #4524 the name was accepted,
// remembered and acted on by nothing, and this shell wrote the row a command
// late in both states — which is byte-for-byte the `off` half, so a test that
// only asked the `off` question would have passed for the shell that had the
// bug.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — on a pseudo-terminal with `TERM=dumb`, `PS1` and `PS2` exported
// empty and the shell started `-fiV +Z`:
//
//	setopt notify / sleep 0.4 &    →  [1] 44968
//	                                  [1]  + done       sleep 0.4     ← unprompted
//	unsetopt notify / sleep 0.4 &  →  [1] 45033
//	  then `print AFTER`              AFTER
//	                                  [1]  + done       sleep 0.4
//
// This is what zsh's own `W02jobs.ztst` needs to reach its last chunk at all.
// That chunk drives the shell through `zpty`, which echoes nothing, so a
// `kill`ed job's notice is the only line there is to read: with the notice a
// command late, `zpty -r` blocks and the file **hangs** rather than failing.
//
// The assertion is on the **raw** bytes the terminal was given, for the reason
// TestLongListJobsNamesThePIDInACompletionNotice gives: a view with the
// escapes taken out is a view that could have normalized away the thing under
// test. Nothing is cleared between waits — Seek moves a cursor — so a failure
// prints everything that was drawn.
func TestNotifyDecidesWhenAFinishedJobIsReported(t *testing.T) {
	// The whole row, spaces and all: the state column is nine wide with two
	// after it (#4508), and a prefix match would pass for a row this change
	// had reformatted.
	const notice = "[1]  + done       sleep 0.4"

	t.Run("on, and nothing is typed after the job", func(t *testing.T) {
		control, screen := jobNoticeSession(t)
		// The prompt that follows `&` is the marker that the job has
		// *started*. Everything after it is unprompted: the wait below is on
		// the notice itself, with nothing else sent down the terminal.
		jobNoticeType(t, control, screen, "sleep 0.4 &")
		if err := screen.Await(notice, jobNoticeBudget); err != nil {
			t.Errorf("no unprompted notice: %v", err)
		}
	})

	t.Run("off, and it waits for the next command", func(t *testing.T) {
		control, screen := jobNoticeSession(t)
		jobNoticeType(t, control, screen, "unsetopt notify")
		jobNoticeType(t, control, screen, "sleep 0.4 &")
		if !screen.Quiet(notice, jobNotifyQuiet) {
			t.Errorf("with the option unset the notice arrived anyway:\n%s",
				smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
		// And the same session says it once a command has run, which is what
		// makes the silence above a measurement rather than a broken wait.
		//
		// Read out of everything drawn since, rather than waited for: the
		// notice is written *before* the prompt jobNoticeType synchronizes
		// on, so a second wait would have the cursor past it already.
		before := screen.Text()
		jobNoticeType(t, control, screen, ":")
		if got := screen.Text()[len(before):]; !strings.Contains(got, notice) {
			t.Errorf("the notice never came at all, so the wait above said nothing:\n%s",
				smoke.Readable(smoke.LastLines(got, 8)))
		}
	})
}

// And the notice does not eat the line being typed: it goes on a row of its
// own and the prompt and the half-typed line are drawn again underneath it.
//
// This is the part a shell gets wrong by doing nothing. Measured on zsh 5.9.2
// the same day with `print MID` typed and not submitted when the job ended: a
// newline, then the row, then both rows of the prompt with `print MID` back on
// the second one.
//
// The two-row prompt is load-bearing, for the reason jobNoticeSession gives —
// a one-row `PS1` cannot tell a prompt that was redrawn whole from one whose
// last row was rewritten.
func TestAnUnpromptedNoticeRedrawsTheLineBeingTyped(t *testing.T) {
	control, screen := jobNoticeSession(t)
	jobNoticeType(t, control, screen, "sleep 0.4 &")
	// Typed and **not** submitted: no newline, so the shell is inside the
	// read with a line in hand when the job ends.
	if _, err := control.WriteString("print MID"); err != nil {
		t.Fatalf("typing the half-line: %v", err)
	}
	if err := screen.Await("[1]  + done       sleep 0.4", jobNoticeBudget); err != nil {
		t.Fatalf("no unprompted notice: %v", err)
	}
	// The prompt's own last row after the notice, and the line back on it.
	// Sought in order and from where the notice was found, so this is
	// "afterwards" rather than "somewhere on the screen".
	if err := screen.Await(jobNoticeMark, jobNoticeBudget); err != nil {
		t.Errorf("the prompt was not drawn again under the notice: %v", err)
	}
	if err := screen.Await("print MID", jobNoticeBudget); err != nil {
		t.Errorf("the line being typed was not drawn again under the notice: %v\n%s",
			err, smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	// And it is still the line the shell will run, which a redraw that only
	// looked right would not prove. Out of what was drawn since rather than
	// waited for, for the reason the other case gives: the output comes
	// before the prompt this synchronizes on.
	before := screen.Text()
	jobNoticeType(t, control, screen, "")
	if got := screen.Text()[len(before):]; !strings.Contains(got, "MID") {
		t.Errorf("the redrawn line did not run:\n%s", smoke.Readable(smoke.LastLines(got, 8)))
	}
}
