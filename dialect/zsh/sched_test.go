// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The `sched` builtin, measured against zsh 5.9.2 on 2026-09-07 with a scratch
// HOME and no startup files, and through a pseudo-terminal for when an elapsed
// entry runs. Every want below is what that binary wrote, with the clock pinned
// here to the day and time the run was made so the listings are the same
// strings.

// schedNow is when the measurements were taken: Monday 7 September 2026 at
// 17:47:39. Pinned in UTC, as runZshAt pins one, so the listing's day name and
// clock are the machine's answer nowhere.
var schedNow = time.Date(2026, time.September, 7, 17, 47, 39, 0, time.UTC)

// runSched runs src with the clock pinned to the hour the oracle ran.
func runSched(t *testing.T, src string) (string, int) {
	t.Helper()
	return runZshAt(t, schedNow, src)
}

// schedRunner is runSched keeping the Runner and a clock that can be moved,
// for the tests about what happens at a prompt rather than what a script sees.
// The clock has to move: an entry is scheduled for the future and the question
// is what happens once the future arrives.
func schedRunner(t *testing.T, src string) (*interp.Runner, *bytes.Buffer, func(time.Duration)) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	now := schedNow
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Dialect: presetDialect(),
		Clock: func() time.Time { return now },
	}
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return r, &out, func(d time.Duration) { now = now.Add(d) }
}

// TestASchedEntryIsStoredAndSaidBack is the honest minimum: an rc file that
// schedules a command runs to the end, and the entry is there afterwards in
// the exact line the real shell writes.
func TestASchedEntryIsStoredAndSaidBack(t *testing.T) {
	out, st := runSched(t, "sched +5 echo hi; echo st=$?\nsched\n")
	if want := "st=0\n  1 Mon Sep  7 17:47:44 echo hi\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
	// An empty table prints nothing at all, at 0.
	out, st = runSched(t, "sched\n")
	if out != "" || st != 0 {
		t.Errorf("output = %q status %d, want nothing at 0", out, st)
	}
}

// TestTheTimeSpecifierIsThreeGrammars is the fact worth stating plainly,
// because the manual's `hh:mm` invites reading a bare `+5` as five minutes and
// it is five seconds. A scheduler that fires a hundred times later than it was
// asked to is one nobody can see is broken.
func TestTheTimeSpecifierIsThreeGrammars(t *testing.T) {
	for _, c := range []struct{ name, spec, want string }{
		{"a bare number is seconds", "+5", "Mon Sep  7 17:47:44"},
		{"and a big one is still seconds", "+100", "Mon Sep  7 17:49:19"},
		{"hours and minutes, relative", "+0:05", "Mon Sep  7 17:52:39"},
		{"an hour, relative", "+1:00", "Mon Sep  7 18:47:39"},
		// Measured at 17:58:57, where the answer was 18:59:27 — the same
		// hour and thirty seconds, moved to the clock the rest of this table
		// is pinned to rather than copied across.
		{"with seconds too, relative", "+1:00:30", "Mon Sep  7 18:48:09"},
		// Absolute times are arithmetic from today's midnight rather than a
		// clock face, which is what lets 99:99 mean four days out.
		{"a time still to come today", "23:59", "Mon Sep  7 23:59:00"},
		{"one already past rolls forward a day", "17:00", "Tue Sep  8 17:00:00"},
		{"and midnight is always tomorrow's", "0:00", "Tue Sep  8  0:00:00"},
		{"ninety-nine hours and ninety-nine minutes", "99:99", "Fri Sep 11  4:39:00"},
		{"seconds and all", "12:30:45", "Tue Sep  8 12:30:45"},
		// A bare number with no colon and no plus is seconds since the epoch.
		// Recorded rather than admired.
		{"a bare absolute number is the epoch", "5", "Thu Jan  1  0:00:05"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runSched(t, "sched "+c.spec+" x\nsched\n")
			if want := "  1 " + c.want + " x\n"; out != want || st != 0 {
				t.Errorf("output = %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// The table is sorted by time and numbered from 1 in that order, so the number
// is a position in the listing and not an identity — which is what makes
// `sched -1` mean "the next one".
func TestTheListingIsSortedByTimeAndNumberedFromOne(t *testing.T) {
	out, _ := runSched(t, "sched +5 echo hi; sched +3 echo b\nsched\n")
	want := "  1 Mon Sep  7 17:47:42 echo b\n  2 Mon Sep  7 17:47:44 echo hi\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// And the number is space-padded to three columns, which only shows past
	// nine.
	out, _ = runSched(t, "for i in 1 2 3 4 5 6 7 8 9 10; do sched +$i t$i; done\nsched\n")
	if first, tenth := "  1 Mon Sep  7 17:47:40 t1\n", " 10 Mon Sep  7 17:47:49 t10\n"; !contains(out, first) || !contains(out, tenth) {
		t.Errorf("output = %q, want %q and %q in it", out, first, tenth)
	}
}

// The command is the words joined by one space, and the quoting that produced
// them is gone by the time this builtin sees them.
func TestTheCommandIsTheWordsJoined(t *testing.T) {
	out, _ := runSched(t, "sched +1 echo one   three\nsched +1 \"echo   a\"\nsched\n")
	want := "  1 Mon Sep  7 17:47:40 echo one three\n  2 Mon Sep  7 17:47:40 echo   a\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// And a first word starting with `-` is written after a `--`, so the line
	// reads back as what it was.
	out, _ = runSched(t, "sched +1 -x\nsched\n")
	if want := "  1 Mon Sep  7 17:47:40 -- -x\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// `sched -N` deletes entry N, and takes the rest of the line with it — the
// deletion wins over anything that looks like a time.
func TestDeletingAnEntry(t *testing.T) {
	out, _ := runSched(t, "sched +5 a; sched +3 b\nsched -1; echo st=$?\nsched\n")
	want := "st=0\n  1 Mon Sep  7 17:47:44 a\n"
	if out != want {
		t.Errorf("output = %q, want %q — the earlier entry was number 1", out, want)
	}
	out, _ = runSched(t, "sched -1 +5 echo hi; echo st=$?\nsched\n")
	if want := "zsh:sched:1: not that many entries\nst=1\n"; out != want {
		t.Errorf("output = %q, want %q — the deletion won and nothing was scheduled", out, want)
	}
}

// TestSchedRefusesEachMistakeItsOwnWay pins the wordings, which are this
// builtin's and not shared: `bad option` is bindkey's and zle's rather than
// zstyle's `invalid option`, and `-0` has a usage sentence of its own rather
// than being a bad option.
func TestSchedRefusesEachMistakeItsOwnWay(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"nothing that many", "sched -1", "zsh:sched:1: not that many entries\n"},
		{"past the end", "sched +5 a; sched -2", "zsh:sched:1: not that many entries\n"},
		{"item zero", "sched -0", "zsh:sched:1: usage for delete: sched -<item#>.\n"},
		{"no command", "sched +5", "zsh:sched:1: not enough arguments\n"},
		{"not a time", "sched bogus x", "zsh:sched:1: bad time specifier\n"},
		{"nearly a time", "sched +5x y", "zsh:sched:1: bad time specifier\n"},
		{"not a word this builtin has", "sched -x", "zsh:sched:1: bad option: -x\n"},
		// Three colon-separated fields at most: `sched 1:02:03:04` is refused
		// rather than read as a day and change, measured.
		{"a fourth field", "sched 1:02:03:04 x", "zsh:sched:1: bad time specifier\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runSched(t, c.src+"\n")
			if out != c.want || st != 1 {
				t.Errorf("output = %q status %d, want %q at 1", out, st, c.want)
			}
		})
	}
}

// `$zsh_scheduled_events` is the same table in the same order, one element per
// entry as `<epoch>:<options>:<command>`. Measured; the middle field is empty
// for every spelling this builtin has.
func TestTheScheduledEventsParameter(t *testing.T) {
	out, _ := runSched(t, "sched +5 echo hi; sched +3 x y\nprint -rl -- \"${zsh_scheduled_events[@]}\"\necho n=${#zsh_scheduled_events[@]}\n")
	want := fmt.Sprintf("%d::x y\n%d::echo hi\nn=2\n",
		schedNow.Add(3*time.Second).Unix(), schedNow.Add(5*time.Second).Unix())
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The module loads because both of its features are here, which makes it the
// second in the table that can say so.
func TestTheSchedModuleLoads(t *testing.T) {
	out, st := runSched(t, "zmodload zsh/sched; echo st=$?\nzmodload -lF zsh/sched\n")
	want := "st=0\n+b:sched\n+p:zsh_scheduled_events\n"
	if out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// TestAnElapsedEntryRunsAndLeavesTheTable is what the builtin is for. Taken
// out of the table before it runs, so an entry that schedules another one —
// which is exactly what a plugin manager's scheduler does, since that is how
// it keeps a poll going — does not have its successor removed by the pass that
// ran it.
func TestAnElapsedEntryRunsAndLeavesTheTable(t *testing.T) {
	r, out, advance := schedRunner(t, "sched +5 echo fired\n")
	out.Reset()
	// Nothing is due yet, so a prompt at this moment runs nothing.
	zsh.RunScheduled(r, context.Background())
	if got := out.String(); got != "" {
		t.Errorf("before the time, the session wrote %q, want nothing", got)
	}
	if list, _ := runSchedList(t, r); list == "" {
		t.Error("table = empty before the time, want the entry still in it")
	}
	// And once the time has passed it runs, and is gone.
	advance(6 * time.Second)
	out.Reset()
	zsh.RunScheduled(r, context.Background())
	if got, want := out.String(), "fired\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if list, _ := runSchedList(t, r); list != "" {
		t.Errorf("table after = %q, want the elapsed entry gone", list)
	}
	// And a second prompt runs nothing a second time, which is what taking it
	// out of the table means.
	out.Reset()
	zsh.RunScheduled(r, context.Background())
	if got := out.String(); got != "" {
		t.Errorf("at the next prompt the session wrote %q, want nothing", got)
	}
}

// Everything already due runs in one pass, in time order.
func TestEverythingDueRunsInOnePassInOrder(t *testing.T) {
	r, out, advance := schedRunner(t, "sched +5 echo second\nsched +2 echo first\n")
	advance(10 * time.Second)
	out.Reset()
	zsh.RunScheduled(r, context.Background())
	if got, want := out.String(), "first\nsecond\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// An entry that schedules another is the shape a plugin manager's scheduler
// has, and the second one must not run in the same pass — its time has not
// come, and the pass that ran the first must not have removed it either.
func TestAnEntryThatSchedulesAnotherKeepsIt(t *testing.T) {
	r, out, advance := schedRunner(t,
		"tick() { echo tick; sched +60 tick; }\nsched +1 tick\n")
	advance(2 * time.Second)
	out.Reset()
	zsh.RunScheduled(r, context.Background())
	if got, want := out.String(), "tick\n"; got != want {
		t.Errorf("output = %q, want %q — the new entry must not run in the same pass", got, want)
	}
	if list, _ := runSchedList(t, r); list == "" {
		t.Error("table after = empty, want the entry the run scheduled still in it")
	}
}

// runSchedList asks a Runner for its table the way a script would.
func runSchedList(t *testing.T, r *interp.Runner) (string, int) {
	t.Helper()
	f, err := syntax.Parse("sched\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	saved := r.Stdout
	r.Stdout = &out
	st, rerr := r.Run(context.Background(), f)
	r.Stdout = saved
	if rerr != nil {
		t.Fatal(rerr)
	}
	return out.String(), st
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
