// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `jobs -d` on a terminal, which is the surface zsh's own `W02jobs.ztst`
// asks about: it drives an interactive shell through `zpty` and wants a
// `(pwd : …)` line under the running job's row.
//
// The listing itself is measured where a script can see it, in dialect/zsh.
// What is asked here is that the letter reaches a session — the shell a
// person is typing at, with the monitor on and a job of its own — and that
// the line names the directory the **job** started in rather than the one
// the shell is in when the listing is asked. The `cd` between the two is the
// whole of the case: without it, a reading of either noun writes the same
// line.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb`,
// `PS1` and `PS2` exported empty and the shell started `-fiV +Z`:
//
//	mkdir sub; cd sub; sleep 5 &; cd ..; jobs -d
//	   [1] 48212
//	   [1]  + running    sleep 5
//	   (pwd : ~/sub)
//
// The assertion is on the **raw** bytes the terminal was given, and the
// value it looks for is one no typed line contains: `~/sub` is written by
// the shell's own abbreviation of a path it resolved, where every line this
// test types is echoed back verbatim. A probe matching text it had itself
// typed is a failure this repository has had (#4491).
func TestJobsDashDNamesTheJobsDirectoryOnATerminal(t *testing.T) {
	control, screen := jobNoticeSession(t)

	// The home directory is the session's scratch one, so the directory the
	// job starts in abbreviates to `~/sub` and the one it is listed from to
	// `~`. Two spellings the shell produces and neither of them typed.
	jobNoticeType(t, control, screen, "mkdir sub")
	jobNoticeType(t, control, screen, "cd sub")
	jobNoticeType(t, control, screen, "sleep 5 &")
	jobNoticeType(t, control, screen, "cd ..")
	before := screen.Text()
	jobNoticeType(t, control, screen, "jobs -d")
	// The prompt jobNoticeType waited for is the marker that the listing has
	// been written; nothing is cleared between waits, so a failure prints
	// everything the session drew.
	listing := screen.Text()[len(before):]

	if !strings.Contains(listing, "(pwd : ~/sub)") {
		t.Errorf("the listing is\n%s\nwant a line naming the directory the job started in",
			smoke.Readable(smoke.LastLines(listing, 8)))
	}
	if strings.Contains(listing, "(pwd : ~)") {
		t.Errorf("the listing followed the `cd` to the shell's own directory:\n%s",
			smoke.Readable(smoke.LastLines(listing, 8)))
	}
	if strings.Contains(listing, "not implemented") {
		t.Errorf("the letter is still refused on a terminal:\n%s",
			smoke.Readable(smoke.LastLines(listing, 8)))
	}
	// And the row above it is the ordinary state row, unchanged — this is
	// the pair `W02jobs.ztst` matches, in the order it matches them.
	row := strings.Index(listing, "[1]  + running")
	line := strings.Index(listing, "(pwd : ~/sub)")
	if row < 0 || line < row {
		t.Errorf("the listing is\n%s\nwant the state row and then the directory under it",
			smoke.Readable(smoke.LastLines(listing, 8)))
	}
}
