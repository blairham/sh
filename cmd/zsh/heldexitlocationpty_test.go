// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// TestAnExitInsideEvalOrAFunctionOnACommandString is #6078: on the `-c`
// route with the monitor on, an `exit` inside `eval` text is held — the
// sentence is located in the text and the `eval` returns 1 — and the hangup
// warning behind an `exit` that wrote no sentence is located at the
// outermost place: the call's line for a function, however deep, and the
// text's own for `eval` at the top. A one-line function's sentence names the
// function with no line.
//
// Measured 2026-10-05 through a pseudo-terminal on zsh 5.9.2
// (/opt/homebrew/bin/zsh). The helper's EXIT trap sits on the string's first
// line, which moves every line number one down from the measurement.
func TestAnExitInsideEvalOrAFunctionOnACommandString(t *testing.T) {
	for _, c := range []struct {
		name, body string
		not        []string
		last       []string
	}{
		{
			name: "an unheld exit in a function, located at the call",
			body: "set -m\n/bin/sleep 1 &\nsetopt nocheckjobs\nf() {\n:\nexit\n}\nf",
			last: []string{"zsh:9: warning: 1 jobs SIGHUPed", monitorExitEnd},
		},
		{
			name: "in a function called from a function",
			body: "set -m\n/bin/sleep 1 &\nsetopt nocheckjobs\nf() {\nexit\n}\ng() {\nf\n}\ng",
			last: []string{"zsh:11: warning: 1 jobs SIGHUPed", monitorExitEnd},
		},
		{
			name: "an unheld exit in eval, located in the text",
			body: "set -m\n/bin/sleep 1 &\nsetopt nocheckjobs\neval exit",
			last: []string{"(eval):1: warning: 1 jobs SIGHUPed", monitorExitEnd},
		},
		{
			name: "a held exit in eval returns 1",
			body: "set -m\n/bin/sleep 1 &\neval exit; print -r -- st=$?",
			last: []string{"(eval):1: you have running jobs.", "st=1", monitorExitEnd, "zsh:1: warning: 1 jobs SIGHUPed"},
		},
		{
			name: "and a later exit is located on its own line",
			body: "set -m\n/bin/sleep 1 &\neval exit\nexit",
			last: []string{"(eval):1: you have running jobs.", "zsh:5: warning: 1 jobs SIGHUPed", monitorExitEnd},
		},
		{
			name: "a one-line function names no line",
			body: "set -m\n/bin/sleep 1 &\nf() { exit; }\nf",
			last: []string{"f: you have running jobs.", "zsh:1: warning: 1 jobs SIGHUPed"},
			not:  []string{"f:0:"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			screen := monitorExitCommandString(t, c.body, c.last...)
			at := 0
			for _, w := range c.last {
				i := strings.Index(screen[at:], w)
				if i < 0 {
					t.Fatalf("want %q after what came before it; the screen was\n%s", w, smoke.Readable(smoke.LastLines(screen, 10)))
				}
				at += i + len(w)
			}
			for _, n := range c.not {
				if strings.Contains(screen, n) {
					t.Errorf("%q should not be there; the screen was\n%s", n, smoke.Readable(smoke.LastLines(screen, 10)))
				}
			}
		})
	}
}

// And the same hold in a script: measured 2026-10-05, `set -m⏎sleep 1
// &⏎eval exit; echo st=$?` as a script under zsh 5.9.2 writes `(eval):1: you
// have running jobs.` and `st=1`, where this shell located the sentence at
// the script's own line and left.
func TestAHeldExitInEvalInAScript(t *testing.T) {
	screen := monitorExitScript(t, []string{"-f"}, "set -m\n/bin/sleep 1 &\neval exit; print -r -- st=$?\nprint -r -- "+monitorExitFence+"\n")
	for _, w := range []string{"(eval):1: you have running jobs.", "st=1", monitorExitFence} {
		if !strings.Contains(screen, w) {
			t.Errorf("want %q; the screen was\n%s", w, smoke.Readable(smoke.LastLines(screen, 10)))
		}
	}
}
