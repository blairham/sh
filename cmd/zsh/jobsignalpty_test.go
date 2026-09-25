// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A job a signal killed is named for the signal in the notice it gets — on a
// terminal, which is the only place that notice exists.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable — on a pseudo-terminal with `TERM=dumb`, `PS1` and `PS2` exported
// empty and the shell started `-fiV +Z`:
//
//	kill %1        [1]  + terminated  sleep 30
//	kill -HUP %1   [1]  + hangup     sleep 30
//	kill -INT %1   [1]  + interrupt  sleep 30
//	kill -KILL %1  [1]  + killed     sleep 30
//
// This shell wrote `done` for all four (#4508), which is what zsh's own
// `W02jobs.ztst` asks about in its last chunk: its patterns are `terminate*`,
// `hangup*`, `interrupt*` and `kill*`.
//
// The rows are asserted **whole**, spaces and all, because the padding is part
// of what was wrong: `terminated` is followed by two spaces where `hangup` is
// followed by five, which is a column nine wide with two after it rather than
// the flat eleven this shell had. And they are asserted on the **raw** bytes
// the terminal was given, for the reason
// TestLongListJobsNamesThePIDInACompletionNotice gives: a view with the
// escapes taken out is a view that could have normalized away the thing under
// test.
//
// One job at a time, each of them `%1`, so that the row under test is not also
// a claim about how job numbers are reused.
func TestAKilledJobsNoticeNamesTheSignal(t *testing.T) {
	control, screen := jobNoticeSession(t)
	for _, c := range []struct{ kill, want string }{
		{"kill %1", "[1]  + terminated  sleep 30"},
		{"kill -HUP %1", "[1]  + hangup     sleep 30"},
		{"kill -INT %1", "[1]  + interrupt  sleep 30"},
		{"kill -KILL %1", "[1]  + killed     sleep 30"},
	} {
		t.Run(c.kill, func(t *testing.T) {
			jobNoticeType(t, control, screen, "sleep 30 &")
			// Everything drawn since the kill, waited out until the notice
			// is in it. `[1]  + ` is the notice and nothing else: the `&`
			// announcement is `[1] <pid>`, with no marker. Since #4524 the
			// notice arrives on its own rather than at the prompt after the
			// next command, so no second line is typed to fetch it —
			// jobNoticeAfter says why the reading is a tail and not a seek.
			got := jobNoticeAfter(t, control, screen, c.kill, "[1]  + ")
			if !strings.Contains(got, c.want) {
				t.Errorf("after %q the terminal was given\n%s\nwant a row %q",
					c.kill, smoke.Readable(smoke.LastLines(got, 10)), c.want)
			}
			if strings.Contains(got, "+ done") {
				t.Errorf("after %q the job was reported done:\n%s",
					c.kill, smoke.Readable(smoke.LastLines(got, 10)))
			}
		})
	}
}
