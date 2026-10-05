// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"testing"
)

// zcalc at a terminal: the first results from the command line, a line
// naming two of them, a line leaving a ( open carried on behind a `...`
// prompt, and a blank line back to the shell. Measured 2026-10-04 through a
// pseudo-terminal against /opt/homebrew/bin/zsh (zsh 5.9.2) with its own
// zcalc and ZCALCPROMPT set to `zc%1v> `: the same prompts, `...zc5> ` for
// the carried line, and the results 3, 15 and 9 on rows of their own.
//
// The prompts are the marks waited on, and none of them is text that is
// typed; the results are read off the screen once the session is back at
// the shell's prompt.
func TestZcalcAtATerminal(t *testing.T) {
	control, screen, _ := contribSession(t, "autoload -Uz zcalc\nZCALCPROMPT='zc%1v> '\n")
	for _, step := range []struct{ keys, mark string }{
		{"zcalc 5 6\r", "zc3> "},
		{"1+2\r", "zc4> "},
		{"$1*$3\r", "zc5> "},
		{"(1+\r", "...zc5> "},
		{"2)*3\r", "zc6> "},
		{"\r", jobNoticeMark},
	} {
		contribSend(t, control, step.keys)
		if err := screen.Await(step.mark, jobNoticeBudget); err != nil {
			t.Fatalf("after %q: %v\n%s", step.keys, err, grid(screen))
		}
	}
	g := grid(screen)
	var rows []string
	for r := range g.Rows() {
		rows = append(rows, g.Text(r))
	}
	for _, want := range [][]string{{"1> 5", "2> 6"}, {"3"}, {"15"}, {"9"}} {
		i := slices.Index(rows, want[0])
		if i < 0 || (len(want) > 1 && (i+1 >= len(rows) || rows[i+1] != want[1])) {
			t.Errorf("no rows %q on the screen:\n%s", want, g)
		}
	}
}
