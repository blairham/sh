// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"strings"
	"testing"
)

// The hang half of #4026, asked of the strings rather than of a terminal.
//
// Deliberately here and not in ourspty_test.go. What made this failure both
// rare and expensive was that only a real shell on a real terminal could ask
// the question, and only on Linux — macOS revokes the terminal end when the
// control end closes, so the orphan that costs a Linux leg ten minutes costs
// this machine nothing and shows nothing. A property that can be asserted
// about the line itself is asserted on every machine on every run, which is
// the only way this one gets watched from here at all.

// A background job holds none of the terminal's three streams.
//
// A job is in a process group of its own, so one the harness failed to learn
// the pid of outlives close's kills — and a surviving holder of the terminal
// end keeps the control end's read blocked, because a read there ends only
// when the last holder lets go. That is the ten-minute hang: the package's
// whole test budget spent waiting out somebody else's `sleep 600`. A job
// holding nothing cannot cause it.
func TestABackgroundJobHoldsNoneOfTheTerminal(t *testing.T) {
	line := jobLine("0")
	for _, redirect := range []string{"</dev/null", ">/dev/null", "2>&1"} {
		if !strings.Contains(line, redirect) {
			t.Errorf("a background job keeps %s of the terminal: %q", redirect, line)
		}
	}
	// Before the `&`, or they belong to the printf rather than to the job
	// that outlives it — which is the only one that can hold anything.
	amp := strings.Index(line, "&")
	if amp < 0 {
		t.Fatalf("the job is not backgrounded at all: %q", line)
	}
	for _, redirect := range []string{"</dev/null", ">/dev/null", "2>&1"} {
		if at := strings.Index(line, redirect); at < 0 || at > amp {
			t.Errorf("%s is not part of the backgrounded command: %q", redirect, line)
		}
	}
}

// And the line still says what background waits for, so the test above
// cannot be satisfied by a line that redirects everything and starts
// nothing this harness can find again.
func TestTheJobLineStillCarriesBothMarks(t *testing.T) {
	line := jobLine("7")
	for _, want := range []string{"job-%s-is[%s]", "job-%s-said", " 7 $! 7"} {
		if !strings.Contains(line, want) {
			t.Errorf("the job line lost %q: %q", want, line)
		}
	}
}

// close kills a job whose pid background never managed to read, because the
// pid is on the screen even when nothing read it.
//
// This is the orphan a failed render leaves: a `sleep 600` at ppid 1, in a
// process group the shell's group kill cannot reach.
func TestAJobIsFoundOnTheScreenWhenItsPidWasNeverRead(t *testing.T) {
	screen := []byte("job-0-is[31337]\r\njob-0-said\r\njob-1-is[31338]\r\njob-1-said\r\n")
	got := jobPids(screen)
	want := []int{31337, 31338}
	if len(got) != len(want) {
		t.Fatalf("read %v off the screen, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("read %v off the screen, want %v", got, want)
		}
	}
}

// A half-drawn field names no pid, because what is found here is killed.
//
// The pid's own line having been drawn is not proof it was drawn *whole* —
// a terminal hands over as many bytes as it has — so the mark that follows
// it is what vouches for it. Without that rule this reads `3` out of a
// `31337` that was still arriving and kills whatever process 3 is.
func TestAHalfDrawnJobNamesNoPid(t *testing.T) {
	for _, screen := range []string{
		"job-0-is[3133",              // still arriving, no bracket yet
		"job-0-is[31337]",            // bracketed, but nothing vouches for it
		"job-0-is[31337]\r\njob-0-s", // and the mark itself half-drawn
	} {
		if got := jobPids([]byte(screen)); len(got) != 0 {
			t.Errorf("a half-drawn screen named %v as started: %q", got, screen)
		}
	}
	// The echo of the line that starts the job names no pid either: it
	// carries `job-%s-is[` and `job-%s-said`, neither of which is a mark.
	if got := jobPids([]byte(jobLine("0"))); len(got) != 0 {
		t.Errorf("the echo of the typed line named %v as started", got)
	}
}
