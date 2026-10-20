// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The fifteen rows BusyBox v1.37.0 prints on Linux, byte for byte, with every
// limit set to the value this test's hooks hold. Measured 2026-09-14 via
// `docker run --rm alpine:3 /bin/ash -c 'ulimit -a'` and confirmed identical
// from a glibc build on Debian 12.
//
// The values are this file's and not the measurement's — a real `ulimit -a`
// reports the machine, which no test can pin. What is pinned is the *layout*:
// the label, its 32-column field, the letter in its own parenthesis, and the
// scale each row divides by.
var wantLinuxTable = strings.Join([]string{
	"core file size (blocks)         (-c) 4",
	"data seg size (kb)              (-d) 2",
	"scheduling priority             (-e) 2048",
	"file size (blocks)              (-f) 4",
	"pending signals                 (-i) 2048",
	"max locked memory (kb)          (-l) 2",
	"max memory size (kb)            (-m) 2",
	"open files                      (-n) 2048",
	"POSIX message queues (bytes)    (-q) 2048",
	"real-time priority              (-r) 2048",
	"stack size (kb)                 (-s) 2",
	"cpu time (seconds)              (-t) 2048",
	"max user processes              (-u) 2048",
	"virtual memory (kb)             (-v) 2",
	"file locks                      (-x) 2048",
	"",
}, "\n")

// The five limits Linux has and the BSDs do not, in the order the table lists
// them.
var linuxOnly = []interp.Resource{
	interp.ResourceSchedulingPriority,
	interp.ResourcePendingSignals,
	interp.ResourceMessageQueues,
	interp.ResourceRealtimePriority,
	interp.ResourceFileLocks,
}

// ulimitRun runs a snippet with every limit at 2048 bytes, and with hasRlimit
// deciding which limits this pretend kernel has at all.
func ulimitRun(t *testing.T, has func(interp.Resource) bool, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, dg := ash.Semantics(), ash.Diagnostics()
	r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "ash", Dialect: presetDialect()}
	r.GetRlimit = func(interp.Resource) (int64, int64, error) { return 2048, 2048, nil }
	r.SetRlimit = func(interp.Resource, int64, int64) error { return nil }
	r.HasRlimit = has
	ash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return buf.String(), st
}

// TestUlimitListingIsTheMeasuredTable: `ulimit -a` renders BusyBox's own
// layout rather than refusing the letter, which is #2278.
//
// The refusal it replaces was not a gap in the measurement — the fifteen rows
// were recorded in full when the issue was filed — but a gap in what could be
// said about a *second* platform, since five of them name limits only Linux
// has and there is no BusyBox on a BSD to ask. See the table's own comment for
// how that was closed without one.
func TestUlimitListingIsTheMeasuredTable(t *testing.T) {
	out, st := ulimitRun(t, nil, "ulimit -a")
	if st != 0 {
		t.Fatalf("status %d, want 0 — `ulimit -a` refused (#2278): %q", st, out)
	}
	if out != wantLinuxTable {
		t.Errorf("`ulimit -a` wrote:\n%s\nwant:\n%s", out, wantLinuxTable)
	}
}

// TestAKernelWithoutALimitDropsItsRowAndMovesNothingElse is the half of #2278
// that could not be read off the Linux run, and the half a wrong fix would
// get wrong in a way no Linux test would see.
//
// A macOS build has no RLIMIT_NICE, RLIMIT_SIGPENDING, RLIMIT_MSGQUEUE,
// RLIMIT_RTPRIO or RLIMIT_LOCKS. The measured rule — bash 5.3, zsh and dash
// each print their Linux table without the rows for limits the kernel lacks,
// every other row byte-identical, because the field width is a literal and
// not one computed from the rows present — says the ten that remain keep the
// column this table found them in.
//
// So the assertion is deliberately strict about the *padding* of the
// survivors: a fix that rebuilt the table for the shorter set, or that
// printed a zero for a limit that does not exist, fails here.
func TestAKernelWithoutALimitDropsItsRowAndMovesNothingElse(t *testing.T) {
	absent := map[interp.Resource]bool{}
	for _, res := range linuxOnly {
		absent[res] = true
	}
	out, st := ulimitRun(t, func(res interp.Resource) bool { return !absent[res] }, "ulimit -a")
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	var want []string
	for _, line := range strings.Split(strings.TrimSuffix(wantLinuxTable, "\n"), "\n") {
		switch {
		case strings.Contains(line, "(-e)"), strings.Contains(line, "(-i)"),
			strings.Contains(line, "(-q)"), strings.Contains(line, "(-r)"),
			strings.Contains(line, "(-x)"):
		default:
			want = append(want, line)
		}
	}
	if len(want) != 10 {
		t.Fatalf("the table has %d rows a BSD kernel can answer, want 10", len(want))
	}
	if got := strings.Join(want, "\n") + "\n"; out != got {
		t.Errorf("without the five Linux limits `ulimit -a` wrote:\n%s\nwant the same "+
			"ten rows in the same columns:\n%s", out, got)
	}
}

// TestEveryRowsLetterSitsInTheSameColumn pins the one thing the second
// measurement would have settled and no run of ours can: that the label field
// is a fixed 32 columns rather than one sized to the rows that happen to be
// present. The widest label in the table — `POSIX message queues (bytes)` — is
// one of the five that a BSD kernel drops, so a computed width would narrow
// every remaining row by five columns and this test is what notices.
func TestEveryRowsLetterSitsInTheSameColumn(t *testing.T) {
	for _, tc := range []struct {
		name string
		has  func(interp.Resource) bool
	}{
		{"every limit", nil},
		{"without the Linux-only five", func(res interp.Resource) bool {
			for _, l := range linuxOnly {
				if res == l {
					return false
				}
			}
			return true
		}},
	} {
		out, _ := ulimitRun(t, tc.has, "ulimit -a")
		for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
			if i := strings.Index(line, "(-"); i != 32 {
				t.Errorf("%s: %q puts its letter at column %d, want 32", tc.name, line, i)
			}
		}
	}
}
