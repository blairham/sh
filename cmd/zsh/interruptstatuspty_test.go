// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// **^C at the prompt sets `$?` to 130** (#5867), whatever the line held and
// whatever the status was before.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — through a pseudo-terminal, `-f -i`, the
// command before the ^C failing with 1 (and with 127, and succeeding):
//
//	keys before ^C          then `$?`   pipestatus
//	`abc`, half typed       130         the pipeline before, untouched
//	nothing                 130
//	`echo "abc⏎` (at PS2)   130
//	`for i in 1; do⏎`       130
//
// This shell abandoned the line and left the previous status in place, so the
// prompt's failure indicator — powerlevel10k's `✘ INT` — drew nothing for a
// ^C. The status before each row is 1 rather than 0, so a shell that kept it
// and a shell that reset it fail differently and both fail.
//
// pipestatus is asserted beside it because it is the half that differs from
// bash: zsh leaves the record alone, bash writes 130 into it. See
// interp.Semantics.PromptStatusWritesThePipelineRecord, and the bash twin in
// cmd/bash/interruptstatuspty_test.go.
func TestControlCAtThePromptSetsTheStatus(t *testing.T) {
	control, screen, _ := jobNoticeSessionRC(t, "PS2='"+interruptPS2+"'\n", "zsh", "-i")
	for _, tc := range []struct {
		name string
		// keys is typed before the ^C; a newline in it is a line the
		// prompt took and is waiting for the rest of.
		keys string
	}{
		{"a half-typed line", "abc"},
		{"an empty line", ""},
		{"a continuation prompt", "echo \"abc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			interruptType(t, control, screen, "true | false")
			interruptAt(t, control, screen, tc.keys)
			got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine)
			if want := "130-0 1"; got != want {
				t.Errorf("after ^C on %s the next line read %q, want %q", tc.name, got, want)
			}
		})
	}
}

// A refused line goes through the same door, and zsh leaves the record alone
// there too: after `true | false` and a `)` the parser will not read, zsh
// 5.9.2 reads `$?` as 1 and pipestatus as `0 1`. Measured beside the rows
// above; the bash twin is the row that moves.
func TestARefusedLineLeavesThePipelineRecord(t *testing.T) {
	control, screen, _ := jobNoticeSessionRC(t, "", "zsh", "-i")
	interruptType(t, control, screen, "true | false")
	interruptType(t, control, screen, ")")
	got := interruptAnswer(t, control, screen, "print -r -- st-$?-${pipestatus[*]}", interruptStatusLine)
	if want := "1-0 1"; got != want {
		t.Errorf("after a refused line the next line read %q, want %q", got, want)
	}
}

// And precmd runs after the ^C and is handed 130 — which is the route the
// status takes to a prompt theme, and so the symptom the issue was filed for.
// Measured the same way: `precmd() { pc=$? }`, a failing command, `abc` and
// ^C, and `$pc` read 130 at the next line.
func TestPrecmdSeesTheStatusAControlCLeft(t *testing.T) {
	control, screen, _ := jobNoticeSessionRC(t, "precmd() { print -r -- \"pc[$?]\" }\n", "zsh", "-i")
	interruptType(t, control, screen, "false")
	from := len(screen.Text())
	interruptAt(t, control, screen, "abc")
	if got := interruptDrawnSince(t, screen, from, interruptHookLine); got != "130" {
		t.Errorf("precmd after ^C saw $? = %s, want 130", got)
	}
}

// interruptPS2 is the continuation prompt, waited on before the ^C so that
// the ^C lands at PS2 and not on a line still being read.
const interruptPS2 = "jn2> "

// interruptStatusLine and interruptHookLine pick out what the typed line and
// the hook print. Neither can match the echo of what was typed: the echo
// carries `$?`, and these want digits where it stands.
var (
	interruptStatusLine = regexp.MustCompile(`st-([0-9]+-[0-9 ]*)\r?\n`)
	interruptHookLine   = regexp.MustCompile(`pc\[([0-9]+)\]`)
)

// interruptAt types keys, waits for the continuation prompt if they left one,
// presses ^C, and waits for the fresh prompt the ^C leaves.
//
// The wait is on the prompt's **upper row** and then its last: the editor
// redraws the row being typed on, so a wait on `jn> ` alone could be answered
// by the redraw of a keystroke before the ^C was read. The upper row is drawn
// once per prompt.
func interruptAt(t *testing.T, control *os.File, screen *smoke.Screen, keys string) {
	t.Helper()
	if _, err := control.WriteString(keys); err != nil {
		t.Fatalf("typing %q: %v", keys, err)
	}
	if len(keys) > 0 && keys[len(keys)-1] == '\n' {
		if err := screen.Await(interruptPS2, jobNoticeBudget); err != nil {
			t.Fatalf("no continuation prompt after %q: %v", keys, err)
		}
	}
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatalf("typing ^C: %v", err)
	}
	for _, mark := range []string{"JNROW", jobNoticeMark} {
		if err := screen.Await(mark, jobNoticeBudget); err != nil {
			t.Fatalf("no prompt after ^C: %v", err)
		}
	}
}

// interruptType is jobNoticeType with the wait interruptAt has: both rows of
// the next prompt, so a redraw of the line being typed cannot answer it.
func interruptType(t *testing.T, control *os.File, screen *smoke.Screen, line string) {
	t.Helper()
	interruptAnswer(t, control, screen, line, nil)
}

// interruptAnswer types line, answers what rx captured from its output — or
// nothing, for a nil rx — and waits for both rows of the prompt after it.
func interruptAnswer(t *testing.T, control *os.File, screen *smoke.Screen, line string, rx *regexp.Regexp) string {
	t.Helper()
	from := len(screen.Text())
	if _, err := control.WriteString(line + "\n"); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}
	got := ""
	if rx != nil {
		got = interruptDrawnSince(t, screen, from, rx)
	}
	for _, mark := range []string{"JNROW", jobNoticeMark} {
		if err := screen.Await(mark, jobNoticeBudget); err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
	}
	return got
}

// interruptDrawnSince polls what was drawn after from for rx and answers its
// first group, so a wrong status fails at once rather than at a deadline.
func interruptDrawnSince(t *testing.T, screen *smoke.Screen, from int, rx *regexp.Regexp) string {
	t.Helper()
	deadline := time.Now().Add(jobNoticeBudget)
	for {
		if m := rx.FindStringSubmatch(screen.Text()[from:]); m != nil {
			return m[1]
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("waited %v for %s; drawn since:\n%s",
				jobNoticeBudget, rx, smoke.Readable(smoke.LastLines(screen.Text()[from:], 10)))
		}
		time.Sleep(2 * time.Millisecond)
	}
}
