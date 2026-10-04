// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
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

// And on the read path the suite actually uses, which is the one the hang was
// on.
//
// `+Z` turns the line editor off: the terminal gathers the line itself and
// echoes it, so the shell is blocked in a plain read with nothing of its own
// on the screen. That is a different place from the editor's key loop and it
// needs the wake looked at separately — see repl/jobnotify.go — and it is the
// place zsh's own `W02jobs.ztst` drives the shell from, where a notice that
// waits for the next command means `zpty -r` waits for ever.
//
// Nothing is redrawn here, which is measured rather than assumed: on zsh 5.9.2
// under `-fiV +Z` with `PS1=$'PR1\nzz> '`, the row is written straight after
// the prompt, on the prompt's own row, and no new prompt follows it.
func TestAnUnpromptedNoticeReachesASessionWithNoLineEditor(t *testing.T) {
	control, screen := jobNoticeSessionArgs(t, "zsh", "-i", "+Z")
	jobNoticeType(t, control, screen, "sleep 0.4 &")
	if err := screen.Await("[1]  + done       sleep 0.4", jobNoticeBudget); err != nil {
		t.Errorf("no unprompted notice with the editor off: %v", err)
	}
	// And the session still reads the next line, which a wait that swallowed
	// the keystroke would not.
	before := screen.Text()
	jobNoticeType(t, control, screen, "print AFTER")
	if got := screen.Text()[len(before):]; !strings.Contains(got, "AFTER") {
		t.Errorf("the line after the notice did not run:\n%s",
			smoke.Readable(smoke.LastLines(got, 8)))
	}
}

// A job that ends with nothing to report leaves the cursor where the prompt
// left it, right after the prompt's last row.
//
// A disowned job is never in the job table, so there is never a notice for
// it, but its ending still wakes the editor. The editor used to write the
// newline a notice starts with before asking whether there was a notice, so
// each such job moved the cursor down a row to column 0 with nothing drawn
// there. That is how a real `.zshrc` under powerlevel10k came to start with
// the cursor two rows below `❯`: a plugin's deferred load refreshes a
// completion cache with `{ … } &|`, after the first prompt is up (#5862).
//
// Measured 2026-10-04 through a pseudo-terminal against `/opt/homebrew/bin/zsh`
// (zsh 5.9.2, aarch64-apple-darwin25.4.0) with `PS1=$'upper row\n> '`:
// `precmd() { sleep 1 &! }` leaves the cursor at row 1, column 2, and zsh
// writes nothing when the job ends. This shell left it at row 2, column 0,
// and at row 3 with two jobs.
//
// The jobs are started from `precmd` and not from the startup file. A job the
// startup file starts ends before the editor is listening, so it wakes
// nothing, and a test written that way passes against the bug. The position
// is read through a terminal model rather than off the text: the prompt
// itself is drawn correctly either way, and only the cursor is wrong.
func TestAJobEndingWithNothingToReportLeavesTheCursorOnThePrompt(t *testing.T) {
	const rc = `precmd() {
  (( ${+jn_started} )) && return
  jn_started=1
  { sleep 0.2; : > ended1 } &!
  { sleep 0.4; : > ended2 } &!
}
`
	control, screen, home := jobNoticeSessionRC(t, rc, "zsh", "-i")

	// Both jobs have ended once both files are there. A job that is still
	// running cannot have moved anything yet, so asking before then would
	// pass against the bug.
	for _, name := range []string{"ended1", "ended2"} {
		deadline := time.Now().Add(jobNoticeBudget)
		for {
			if _, err := os.Stat(filepath.Join(home, name)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the disowned job writing %s never finished:\n%s",
					name, smoke.Readable(smoke.LastLines(screen.Text(), 8)))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Then a quiet period. The editor answers a job's ending within
	// milliseconds, so a stray newline that is coming arrives well inside
	// it, and the cursor has to stay put for the whole of it.
	cols := 100
	where := func() (row, col, promptRow int, grid *cellgrid.Grid) {
		grid = cellgrid.New(cols)
		_, _ = grid.Write([]byte(screen.Text()))
		promptRow = -1
		for r := range grid.Rows() {
			if strings.HasPrefix(grid.Text(r), strings.TrimRight(jobNoticeMark, " ")) {
				promptRow = r
			}
		}
		row, col = grid.Cursor()
		return row, col, promptRow, grid
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		row, col, promptRow, grid := where()
		if row != promptRow || col != len(jobNoticeMark) {
			t.Fatalf("after two disowned jobs ended the cursor is at row %d, column %d; "+
				"want row %d, column %d, right after the prompt:\n%s\nraw: %q",
				row, col, promptRow, len(jobNoticeMark), grid, screen.Text())
		}
	}

	// And the model can tell a cursor that moved from one that did not: a
	// typed character moves it one column along the prompt's row. Without
	// this, a model that never moved the cursor at all would pass the check
	// above.
	if _, err := control.WriteString("x"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	deadline := time.Now().Add(jobNoticeBudget)
	for {
		row, col, promptRow, grid := where()
		if row == promptRow && col == len(jobNoticeMark)+1 && strings.HasSuffix(grid.Text(row), "x") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a typed character did not move the cursor along the prompt's row: "+
				"cursor at row %d, column %d:\n%s", row, col, grid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
