// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `jobs` inside a subshell of a session, which is the surface zsh's own
// `W02jobs.ztst` asks about in its `` `jobs -r` and `jobs -s` with running
// job `` chunk: it wants the running job's row written **twice**, once from
// `(jobs -r)` and once from the `jobs -r` beside it.
//
// The listing itself is measured where a script can see it, in interp and in
// dialect/zsh. What is asked here is the state a script cannot get into: the
// monitor is on, which zsh grants only where there is a terminal, and the
// difference this test is about does not exist without it. `zsh -f -c
// '/bin/sleep 1 & jobs; (jobs)'` writes the row once in zsh 5.9.2 exactly as
// it does here (#4538).
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — on a pseudo-terminal with `TERM=dumb`,
// `PS1` and `PS2` exported empty and the shell started `-fiV +Z`:
//
//	sleep 3 &   →  [1] 2859
//	(jobs -r)   →  [1]  + running sleep 3
//	jobs -r     →  [1]  + running sleep 3
//
// **And the monitor is the noun rather than the prompt**, which is the pair
// the second test below holds. They are the same in a default session and
// they are not the same thing, and a rule keyed on the wrong one of two
// nouns that agree nearly everywhere is the failure this repository keeps
// meeting.

// subshellJobsFence is text no typed line here contains and only an expansion
// can produce: the shell is told to print a parameter, and what comes back is
// its value. A probe that matched the terminal's echo of its own typed line
// is a failure this repository has had (#4491), and every wait below is on
// this.
const subshellJobsFence = "SJ-FENCE-4538"

// subshellJobsListing types a line that lists and then writes the fence, and
// answers with what was drawn between the two.
//
// The fence is what makes an *empty* listing readable at all: a subshell that
// listed nothing and a subshell whose listing this test failed to capture
// look exactly alike, so the poll is for something that always arrives and
// the listing is read out of what came before it.
func subshellJobsListing(t *testing.T, control *os.File, screen *smoke.Screen, line string) string {
	t.Helper()
	drawn := jobNoticeAfter(t, control, screen, line+`; print "<${fence}>"`, "<"+subshellJobsFence+">")
	listing, _, _ := strings.Cut(drawn, "<"+subshellJobsFence+">")
	return listing
}

// A running job's row is written by `jobs -r` inside `( … )` as well as
// beside it. This is the chunk's own shape, in the order it matches.
func TestASubshellListsTheSessionsJobs(t *testing.T) {
	control, screen := jobNoticeSession(t)
	jobNoticeType(t, control, screen, "fence="+subshellJobsFence)
	jobNoticeType(t, control, screen, "/bin/sleep 3 &")

	inside := subshellJobsListing(t, control, screen, "(jobs -r)")
	beside := subshellJobsListing(t, control, screen, "jobs -r")

	const row = "[1]  + running"
	if !strings.Contains(inside, row) {
		t.Errorf("the subshell's listing is\n%s\nwant a row starting %q",
			smoke.Readable(smoke.LastLines(inside, 8)), row)
	}
	if !strings.Contains(beside, row) {
		t.Errorf("the session's own listing is\n%s\nwant a row starting %q",
			smoke.Readable(smoke.LastLines(beside, 8)), row)
	}
	// The job is still the session's afterwards. A subshell here is a cloned
	// runner holding the very same job, not a fork's copy of it, so "the
	// listing works" and "the listing cost the session its job" are
	// distinguishable only by asking again.
	again := subshellJobsListing(t, control, screen, "jobs -r")
	if !strings.Contains(again, row) {
		t.Errorf("the job is gone from the session after the subshell looked at it:\n%s",
			smoke.Readable(smoke.LastLines(again, 8)))
	}
}

// The monitor and not the prompt. One session, one job, the same `( jobs -r
// )` asked twice with `unsetopt monitor` between — so interactivity is held
// fixed and the answer still moves.
//
// The negative half is the one that needs the fence: "the subshell listed
// nothing" is exactly what a probe that captured nothing also reports.
func TestASubshellStopsListingThemWhenTheMonitorGoesOff(t *testing.T) {
	control, screen := jobNoticeSession(t)
	jobNoticeType(t, control, screen, "fence="+subshellJobsFence)
	jobNoticeType(t, control, screen, "/bin/sleep 3 &")

	const row = "[1]  + running"
	on := subshellJobsListing(t, control, screen, "(jobs -r)")
	if !strings.Contains(on, row) {
		t.Fatalf("with the monitor on the subshell's listing is\n%s\nwant a row starting %q",
			smoke.Readable(smoke.LastLines(on, 8)), row)
	}

	jobNoticeType(t, control, screen, "unsetopt monitor")
	off := subshellJobsListing(t, control, screen, "(jobs -r)")
	if strings.Contains(off, row) {
		t.Errorf("with the monitor off the subshell still lists them:\n%s",
			smoke.Readable(smoke.LastLines(off, 8)))
	}
	// And the session itself still has the job, which is what says the
	// monitor moved the *subshell's* view rather than emptying the table.
	beside := subshellJobsListing(t, control, screen, "jobs -r")
	if !strings.Contains(beside, row) {
		t.Errorf("the session's own listing is\n%s\nwant the job still there",
			smoke.Readable(smoke.LastLines(beside, 8)))
	}
}

// A subshell that ends does not hold its exit for the jobs it can only look
// at, and does not warn about them.
//
// The table a subshell inherits holds nothing it would abandon, so there is
// nothing to stay for. Measured at a `-fiV +Z` session of zsh 5.9.2 with a
// running job: `(exit 42)` answers 42 and says nothing at all, where an
// `exit` typed on the line outside says `you have running jobs.` first.
//
// It is here because the inheritance made it reachable. `HoldsExitForJobs`
// reads the table, and a subshell holding the session's jobs is a shell with
// jobs as far as that reading goes — so an `(exit N)`, which is how
// `internal/promptfidelity` pins a status, wrote the session's warning from
// inside the parentheses.
func TestASubshellDoesNotHoldItsExitForTheSessionsJobs(t *testing.T) {
	control, screen := jobNoticeSession(t)
	jobNoticeType(t, control, screen, "fence="+subshellJobsFence)
	jobNoticeType(t, control, screen, "/bin/sleep 3 &")

	drawn := jobNoticeAfter(t, control, screen,
		`(exit 42); print "<${fence}:$?>"`, "<"+subshellJobsFence+":")
	if !strings.Contains(drawn, "<"+subshellJobsFence+":42>") {
		t.Errorf("the subshell's status did not come back as 42:\n%s",
			smoke.Readable(smoke.LastLines(drawn, 8)))
	}
	if strings.Contains(drawn, "running jobs") {
		t.Errorf("the subshell warned about the session's jobs:\n%s",
			smoke.Readable(smoke.LastLines(drawn, 8)))
	}
}
