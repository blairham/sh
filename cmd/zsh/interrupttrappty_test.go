// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// **A ^C at the prompt runs the INT trap, and the trap decides the line**
// (#5888).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`: the trap set, `false`, a half-typed line, ^C,
// and `$?` with pipestatus on the line after.
//
//	trap                                          handler saw   then
//	TRAPINT() { print …; return 0 }               1             line kept
//	TRAPINT() { print …; return 130 }             1             130-1
//	trap 'print …; return 3' INT                  1             3
//	trap '' INT                                   —             line kept
//
// This shell ran no trap at the prompt and gave the line up with 130 in every
// row. The handler prints `tr-$?-42`, so the wait after the ^C is on the
// handler having run — and on the status it saw — before anything else is
// typed. The `return 3` row asserts only `$?`: zsh's record there reads `3`,
// which this shell does not reproduce, and the other rows pin the record.
//
// Whether the line was kept is read off the line itself, because the status
// cannot say: an ignore leaves `false`'s 1 either way. Where the line is
// kept, what is typed before the ^C is the front of a command and what is
// typed after it is the rest, so `kept-42` is printed only by a kept line —
// a given-up one would run `$((6*7))` alone. Where it is given up, `abc` is
// typed and ^U clears nothing.
func TestControlCAtThePromptRunsTheTrap(t *testing.T) {
	for _, tc := range []struct {
		name, rc string
		// saw is what the handler printed, or empty for an ignore, which
		// runs nothing.
		saw string
		// givesUp is whether a fresh prompt follows the ^C.
		givesUp bool
		probe   string
		want    string
	}{
		{"TRAPINT returning 0 keeps the line", "TRAPINT() { print -r -- tr-$?-$((6*7)); return 0 }\n", "tr-1-42", false, interruptRecordProbe, "0-0"},
		{"TRAPINT returning 130 gives it up", "TRAPINT() { print -r -- tr-$?-$((6*7)); return 130 }\n", "tr-1-42", true, interruptRecordProbe, "130-1"},
		{"a return in the action gives it up", "trap 'print -r -- tr-$?-$((6*7)); return 3' INT\n", "tr-1-42", true, "print -r -- st-$?-", "3-"},
		{"trap \"\" INT keeps it", "trap '' INT\n", "", false, interruptRecordProbe, "0-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionRC(t, tc.rc, "zsh", "-i")
			interruptType(t, control, screen, "false")
			before := "abc"
			if !tc.givesUp {
				before = "print -r -- kept-"
			}
			if _, err := control.WriteString(before + "\x03"); err != nil {
				t.Fatalf("typing %q and ^C: %v", before, err)
			}
			if tc.saw != "" {
				if err := screen.Await(tc.saw, jobNoticeBudget); err != nil {
					t.Fatalf("the trap did not run, or saw another status: %v", err)
				}
			}
			if tc.givesUp {
				for _, mark := range []string{"JNROW", jobNoticeMark} {
					if err := screen.Await(mark, jobNoticeBudget); err != nil {
						t.Fatalf("no prompt after ^C: %v", err)
					}
				}
			} else if got := interruptAnswer(t, control, screen, "$((6*7))", interruptKeptLine); got != "42" {
				t.Fatalf("the line was not kept: it printed %q", got)
			}
			if got := interruptAnswer(t, control, screen, "\x15"+tc.probe, interruptStatusLine); got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
		})
	}
}

// interruptKeptLine picks out what a kept line printed, and cannot match its
// echo, which carries `$((6*7))` where this wants digits.
var interruptKeptLine = regexp.MustCompile(`kept-([0-9]+)\r?\n`)

// interruptRecordProbe prints `$?` and pipestatus the way interruptStatusLine
// reads them.
const interruptRecordProbe = "print -r -- st-$?-${pipestatus[*]}"
