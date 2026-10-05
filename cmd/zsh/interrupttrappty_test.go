// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// **A ^C at the prompt runs the INT trap, and the trap decides the line**
// (#5888).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`: the trap set, `false`, `abc`, ^C, then ^U and
// `$?` with pipestatus on the next line.
//
//	trap                                          handler saw   next line
//	TRAPINT() { print …; return 0 }               1             1-1, line kept
//	TRAPINT() { print …; return 130 }             1             130-1
//	trap 'print …; return 3' INT                  1             3
//	trap '' INT                                   —             1-1, line kept
//
// This shell ran no trap at the prompt and gave the line up with 130 in every
// row. The handler prints `tr-$?-42`, so the wait after the ^C is on the
// handler having run — and on the status it saw — before anything else is
// typed. The `return 3` row asserts only `$?`: zsh's record there reads `3`,
// which this shell does not reproduce, and the other rows pin the record.
//
// Whether the line was kept is read off the status: a line given up sets the
// status the trap returned, and a kept one leaves `false`'s 1 — with ^U
// clearing the kept `abc` so it does not join the line typed next.
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
		{"TRAPINT returning 0 keeps the line", "TRAPINT() { print -r -- tr-$?-$((6*7)); return 0 }\n", "tr-1-42", false, interruptRecordProbe, "1-1"},
		{"TRAPINT returning 130 gives it up", "TRAPINT() { print -r -- tr-$?-$((6*7)); return 130 }\n", "tr-1-42", true, interruptRecordProbe, "130-1"},
		{"a return in the action gives it up", "trap 'print -r -- tr-$?-$((6*7)); return 3' INT\n", "tr-1-42", true, "print -r -- st-$?-", "3-"},
		{"trap \"\" INT keeps it", "trap '' INT\n", "", false, interruptRecordProbe, "1-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionRC(t, tc.rc, "zsh", "-i")
			interruptType(t, control, screen, "false")
			if _, err := control.WriteString("abc\x03"); err != nil {
				t.Fatalf("typing abc and ^C: %v", err)
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
			}
			if got := interruptAnswer(t, control, screen, "\x15"+tc.probe, interruptStatusLine); got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
		})
	}
}

// interruptRecordProbe prints `$?` and pipestatus the way interruptStatusLine
// reads them.
const interruptRecordProbe = "print -r -- st-$?-${pipestatus[*]}"
